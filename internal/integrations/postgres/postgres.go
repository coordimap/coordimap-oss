package postgres

import (
	"database/sql"
	"fmt"
	"slices"
	"strconv"
	"time"

	cloudutils "github.com/coordimap/agent/internal/cloud/utils"
	"github.com/coordimap/agent/pkg/utils"

	_ "github.com/lib/pq"

	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/coordimap/agent/pkg/domain/database"
	"github.com/coordimap/agent/pkg/domain/postgres"
	"github.com/rs/zerolog/log"
)

// NewPostgresCrawler creates a new Postgresql crawler based on the input DataSource provided.
func NewPostgresCrawler(dataSource *agent.DataSource, outChannel chan *agent.CloudCrawlData) (Crawler, error) {
	// 1. initialize postgresCrawler with default values
	crawler := postgresCrawler{
		dbConn:            nil,
		outputChannel:     outChannel,
		crawlInterval:     30 * time.Second,
		Host:              "localhost",
		User:              "postgres",
		Pass:              "",
		DBName:            "postgres",
		SSLMode:           "disable",
		dataSource:        dataSource,
		externalMappingID: "",
	}

	// 2. populate postgresCrawler with the provided configuration
	for _, dsConfig := range dataSource.Config.ValuePairs {
		switch dsConfig.Key {
		case "db_name":
			crawler.DBName = dsConfig.Value

		case "db_user":
			crawler.User = dsConfig.Value

		case "db_pass":
			var err error
			crawler.Pass, err = utils.LoadValueFromEnvConfig(dsConfig.Value)
			if err != nil {
				log.Info().Msgf("Error loading value of db_pass for value: %s. The error returned was: %s", dsConfig.Value, err.Error())
				return &crawler, err
			}

		case "db_host":
			crawler.Host = dsConfig.Value

		case "scope_id":
			crawler.scopeID = dsConfig.Value

		case "mapping_internal_id":
			crawler.externalMappingID = dsConfig.Value

		case "ssl_mode":
			allowedValues := []string{"require", "disable"}

			if slices.Index(allowedValues, dsConfig.Value) == -1 {
				return &crawler, fmt.Errorf("postgres config error: Value %s of config option %s is not allowed", dsConfig.Value, dsConfig.Key)
			}

			crawler.SSLMode = dsConfig.Value

		case "crawl_interval":
			const DEFAULT_CRAWL_TIME = 30 * time.Second
			amountStr := string(dsConfig.Value[:len(dsConfig.Value)-1])
			durationStr := string(dsConfig.Value[len(dsConfig.Value)-1])

			amount, errConv := strconv.ParseInt(amountStr, 10, 32)
			if errConv != nil {
				return &crawler, errConv
			}

			switch durationStr {
			case "s":
				crawler.crawlInterval = time.Duration(amount) * time.Second

			case "m":
				crawler.crawlInterval = time.Duration(amount) * time.Minute

			default:
				crawler.crawlInterval = DEFAULT_CRAWL_TIME
			}
		}
	}

	if crawler.scopeID == "" {
		return nil, fmt.Errorf("PostgreSQL crawler config error: scope_id must be provided for data source %s", crawler.dataSource.DataSourceID)
	}

	// 3. connect to the DB
	db, errDBConn := connectToDB(crawler.Host, crawler.User, crawler.Pass, crawler.DBName, crawler.SSLMode)
	if errDBConn != nil {
		log.Error().Msgf("Cannot connect to the Postgres db of the config %s", crawler.scopeID)
		return &crawler, errDBConn
	}
	crawler.dbConn = db

	return &crawler, nil
}

func connectToDB(dbHost, dbUser, dbPass, dbName, sslMode string) (*sql.DB, error) {
	psqlConnString := fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=%s", dbHost, dbUser, dbPass, dbName, sslMode)

	db, err := sql.Open("postgres", psqlConnString)
	if err != nil {
		return nil, err
	}

	if errPing := db.Ping(); errPing != nil {
		return nil, errPing
	}

	db.SetMaxIdleConns(10)
	db.SetConnMaxIdleTime(1 * time.Hour)
	db.SetMaxOpenConns(20)
	db.SetConnMaxLifetime(20 * time.Minute)

	return db, nil
}

// Crawl Crawls the specified Postgresql database and retrieves all the Tables/MaterializedViews
// Things that are crawled
// 1. Schemas
// 2. Tables
// 3. MaterializedViews
// 4. Indexes
// 5. Relationships (foreign keys)
// 6. Sizes of Tables/Indexes/MaterializedViews
func (postCrawler *postgresCrawler) Crawl() {
	crawlTicker := time.NewTicker(postCrawler.crawlInterval)

	log.Info().Msgf("Starting ticker for: %s", postCrawler.scopeID)
	for range crawlTicker.C {
		_, errCrawl := postCrawler.crawl()
		log.Info().Msgf("Crawling Postgres DB for %s", postCrawler.scopeID)
		if errCrawl != nil {
			// do not ship any data
			log.Info().Msg(errCrawl.Error())
			continue
		}
		// ship the crawledData to the backend
	}
}

func (postCrawler *postgresCrawler) crawl() (*agent.CloudCrawlData, error) {
	crawlTime := time.Now().UTC()
	allCrawledElements := []*agent.Element{}

	postDB := database.Database{
		Name:    postCrawler.DBName,
		Host:    postCrawler.Host,
		Schemas: []string{},
	}

	log.Debug().Msgf("Starting retrieval of Postgres DB schemas for %s", postCrawler.scopeID)

	schemaNames, errGetSchemaNames := postCrawler.getSchemaNames()
	if errGetSchemaNames != nil {
		log.Error().Msgf("Could not retrieve the schema names because: %s", errGetSchemaNames.Error())
	}

	postDB.Schemas = schemaNames
	dbInternalName := generateInternalName(postCrawler.scopeID, postCrawler.DBName, "", "")
	dbElem, errDBElem := utils.CreateElement(postDB, postDB.Name, dbInternalName, postgres.POSTGRES_TYPE_DB, agent.StatusNoStatus, "", crawlTime)
	if errDBElem != nil {
		log.Error().Msgf("Cannot create schema db element for db name: %s because %s", postCrawler.DBName, errDBElem.Error())
		return nil, errDBElem
	}

	externalSqlName, errExternalSqlName := cloudutils.CreateSQLInternalName(postCrawler.externalMappingID)
	if errExternalSqlName == nil {
		extIDDBNameRel, errExtIDDBNameRel := utils.CreateRelationship(externalSqlName, dbInternalName, agent.RelationshipExternalSourceSideType, agent.ParentChildTypeRelation, crawlTime)
		if errExtIDDBNameRel == nil {
			allCrawledElements = append(allCrawledElements, extIDDBNameRel)
		}
	} else {
		rel, errRel := utils.CreateRelationship(postCrawler.Host, dbInternalName, agent.RelationshipExternalSourceSideType, agent.ErTypeRelation, crawlTime)
		if errRel == nil {
			allCrawledElements = append(allCrawledElements, rel)
		}
	}

	allCrawledElements = append(allCrawledElements, dbElem)

	for _, schemaName := range schemaNames {
		schemaInternalName := generateInternalName(postCrawler.scopeID, postCrawler.DBName, schemaName, "")
		log.Debug().Msgf("Starting retrieval of Postgres DB schema tables for %s-%s %s", postCrawler.dataSource.Info.Type, postCrawler.scopeID, schemaName)

		tableNames, errGetTableNames := postCrawler.getSchemaTables(schemaName)
		if errGetTableNames != nil {
			log.Error().Msgf("Could not get the table names for the schema %s because %s", schemaName, errGetTableNames.Error())
			continue
		}

		rel, errRel := utils.CreateRelationship(dbInternalName, schemaInternalName, agent.RelationshipType, agent.ErTypeRelation, crawlTime)
		if errRel == nil {
			allCrawledElements = append(allCrawledElements, rel)
		}

		for _, tableName := range tableNames {
			tableInternalName := generateInternalName(postCrawler.scopeID, postCrawler.DBName, schemaName, tableName)
			log.Debug().Msgf("Starting retrieval of Postgres DB table columns & constraints for %s-%s %s.%s", postCrawler.dataSource.Info.Type, postCrawler.scopeID, schemaName, tableName)
			table, errTable := postCrawler.getTableData(schemaName, tableName)
			if errTable != nil {
				log.Error().Msgf("Error while getting table data for table: %s.%s due to: %s", schemaName, tableName, errTable.Error())
			}
			rel, errRel := utils.CreateRelationship(schemaInternalName, tableInternalName, agent.RelationshipType, agent.ErTypeRelation, crawlTime)
			if errRel == nil {
				allCrawledElements = append(allCrawledElements, rel)
			}

			for _, constraint := range table.Constraints {
				if constraint.Type != postgres.POSTGRES_CONSTRAINT_FK {
					continue
				}

				for _, destination := range constraint.Destinations {

					// add the referenced tableName in the current elem's relations
					rel, errRel := utils.CreateRelationship(tableInternalName, destination.Table, agent.RelationshipType, agent.ErTypeRelation, crawlTime)
					if errRel == nil {
						allCrawledElements = append(allCrawledElements, rel)
					}
				}
			}

			log.Debug().Msgf("Starting retrieval of Postgres DB table indexes for %s-%s %s.%s", postCrawler.dataSource.Info.Type, postCrawler.scopeID, schemaName, tableName)
			tableIndexes, errTableIndexes := postCrawler.getTableIndexes(schemaName, tableName)
			if errTableIndexes != nil {
				log.Info().Msgf("Cannot get the table index names for table: %s.%s because %s", schemaName, tableName, errTableIndexes.Error())
				continue
			}

			for _, tableIndex := range tableIndexes {
				indexInternalName := generateInternalName(postCrawler.scopeID, postCrawler.DBName, schemaName, tableIndex.Name)
				indexElem, errIndexElem := utils.CreateElement(tableIndex, tableIndex.Name, indexInternalName, postgres.POSTGRES_TYPE_INDEX, agent.StatusNoStatus, "", crawlTime)
				if errIndexElem != nil {
					log.Info().Msgf("Cannot create table index element for index: %s because %s", tableIndex.Name, errIndexElem.Error())
					continue
				}
				allCrawledElements = append(allCrawledElements, indexElem)
				table.Indexes = append(table.Indexes, tableIndex.Name)

				relTableIndex, errRelTableIndex := utils.CreateRelationship(tableInternalName, indexInternalName, agent.RelationshipType, agent.ErTypeRelation, crawlTime)
				if errRelTableIndex == nil {
					allCrawledElements = append(allCrawledElements, relTableIndex)
				}

				relDBNameIndex, errRelDBNameIndex := utils.CreateRelationship(dbInternalName, indexInternalName, agent.RelationshipType, agent.ErTypeRelation, crawlTime)
				if errRelDBNameIndex == nil {
					allCrawledElements = append(allCrawledElements, relDBNameIndex)
				}

				relSchemaIndex, errRelSchemaIndex := utils.CreateRelationship(schemaInternalName, indexInternalName, agent.RelationshipType, agent.ErTypeRelation, crawlTime)
				if errRelSchemaIndex == nil {
					allCrawledElements = append(allCrawledElements, relSchemaIndex)
				}
			}

			tableElem, errTableElem := utils.CreateElement(table, tableName, tableInternalName, postgres.POSTGRES_TYPE_TABLE, agent.StatusNoStatus, "", crawlTime)
			if errTableElem != nil {
				log.Info().Msgf("Cannot create table element for table: %s because %s", tableName, errTableElem.Error())
				continue
			}
			allCrawledElements = append(allCrawledElements, tableElem)
		}

		materializedViewNames, errMaterializedViewNames := postCrawler.getSchemaMaterializedViewNames(schemaName)
		if errMaterializedViewNames != nil {
			log.Info().Msgf("Cannot get materialized view names for schema: %s because %s", schemaName, errMaterializedViewNames.Error())
			continue
		}

		for _, materializedViewName := range materializedViewNames {
			view, errView := postCrawler.getMaterializedView(schemaName, materializedViewName)
			if errView != nil {
				log.Info().Msgf("Cannot get view data for materialized view: %s because %s", materializedViewName, errView.Error())
				continue
			}

			materializedViewInternalName := generateInternalName(postCrawler.scopeID, postCrawler.DBName, schemaName, materializedViewName)

			rel, errRel := utils.CreateRelationship(dbInternalName, materializedViewInternalName, agent.RelationshipType, agent.ErTypeRelation, crawlTime)
			if errRel == nil {
				allCrawledElements = append(allCrawledElements, rel)
			}

			relSchema, errRelSchema := utils.CreateRelationship(schemaInternalName, materializedViewInternalName, agent.RelationshipType, agent.ErTypeRelation, crawlTime)
			if errRelSchema == nil {
				allCrawledElements = append(allCrawledElements, relSchema)
			}

			viewElem, errViewElem := utils.CreateElement(view, view.Name, materializedViewInternalName, postgres.POSTGRES_TYPE_MATERIALIZED_VIEW, agent.StatusNoStatus, "", crawlTime)
			if errViewElem != nil {
				log.Info().Msgf("Cannot create materialized view element for view: %s because %s", materializedViewName, errViewElem.Error())
				continue
			}
			allCrawledElements = append(allCrawledElements, viewElem)
		}

		schema := database.Schema{
			Name:     schemaName,
			Tables:   tableNames,
			Views:    materializedViewNames,
			Database: postDB.Name,
		}
		schemaElem, errSchemaElem := utils.CreateElement(schema, schemaName, schemaInternalName, postgres.POSTGRES_TYPE_SCHEMA, agent.StatusNoStatus, "", crawlTime)
		if errSchemaElem != nil {
			// We cannot process anymore if there is no schema
			continue
		}
		allCrawledElements = append(allCrawledElements, schemaElem)

		crawledData := agent.CrawledData{
			Data: allCrawledElements,
		}

		log.Info().Msgf("Crawled %d PostgreSQL elements for connection %s and schema %s", len(allCrawledElements), postCrawler.scopeID, schemaName)

		postCrawler.outputChannel <- &agent.CloudCrawlData{
			Timestamp:       crawlTime,
			DataSource:      *postCrawler.dataSource,
			CrawledData:     crawledData,
			CrawlInternalID: schemaName,
		}
	}

	return nil, nil
}
