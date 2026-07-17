package mariadb

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/coordimap/agent/pkg/utils"

	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/coordimap/agent/pkg/domain/database"
	"github.com/coordimap/agent/pkg/domain/mariadb"
	"github.com/rs/zerolog/log"
)

func NewMariadbCrawler(dataSource *agent.DataSource, outChannel chan *agent.CloudCrawlData) (Crawler, error) {
	crawler := mariadbCrawler{
		dbConn:            nil,
		outputChannel:     outChannel,
		crawlInterval:     30 * time.Second,
		Host:              "localhost",
		User:              "postgres",
		Pass:              "",
		DBName:            "postgres",
		dataSource:        dataSource,
		externalMappingID: "",
	}
	var mappingDSID, mappingInternalID string

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

		case "scope_id":
			crawler.scopeID = dsConfig.Value

		case "db_host":
			crawler.Host = dsConfig.Value

		case "mapping_data_source_id":
			mappingDSID = dsConfig.Value

		case "mapping_internal_id":
			mappingInternalID = dsConfig.Value

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
		return nil, fmt.Errorf("MariaDB crawler config error: scope_id must be provided for data source %s", crawler.dataSource.DataSourceID)
	}

	if mappingDSID != "" && mappingInternalID != "" {
		crawler.externalMappingID = fmt.Sprintf("%s-%s", mappingDSID, mappingInternalID)
	}

	// 3. connect to the DB
	db, errDBConn := connectToDB(crawler.User, crawler.Pass, crawler.Host, "3306", crawler.DBName)
	if errDBConn != nil {
		log.Error().Msgf("Cannot connect to the MariaDB of the config %s", dataSource.DataSourceID)
		return &crawler, errDBConn
	}
	crawler.dbConn = db

	return &crawler, nil
}

func (mariaCrawler *mariadbCrawler) Crawl() {
	crawlTicker := time.NewTicker(mariaCrawler.crawlInterval)

	log.Info().Msgf("Starting ticker for: %s", mariaCrawler.scopeID)
	for range crawlTicker.C {
		_, errCrawl := mariaCrawler.crawl()
		log.Info().Msgf("Crawling MariaDB for %s", mariaCrawler.scopeID)
		if errCrawl != nil {
			// do not ship any data
			log.Info().Msg(errCrawl.Error())
			continue
		}
		// ship the crawledData to the backend
	}
}

func (mariaCrawler *mariadbCrawler) crawl() (*agent.CloudCrawlData, error) {
	crawlTime := time.Now().UTC()
	allCrawledElements := []*agent.Element{}

	postDB := database.Database{
		Name:    mariaCrawler.DBName,
		Host:    mariaCrawler.Host,
		Schemas: []string{},
	}

	schemaName := mariaCrawler.DBName
	dbInternalName := generateInternalName(mariaCrawler.scopeID, mariaCrawler.DBName, "")
	dbElem, errDBElem := utils.CreateElement(postDB, postDB.Name, dbInternalName, mariadb.MARIADB_TYPE_DB, agent.StatusNoStatus, "", crawlTime)
	if errDBElem != nil {
		log.Error().Msgf("Cannot create schema db element for db name: %s because %s", mariaCrawler.DBName, errDBElem.Error())
		return nil, errDBElem
	}

	allCrawledElements = append(allCrawledElements, dbElem)
	extIDDBNameRel, errExtIDDBNameRel := utils.CreateRelationship(mariaCrawler.externalMappingID, dbInternalName, agent.RelationshipExternalSourceSideType, agent.ParentChildTypeRelation, crawlTime)
	if errExtIDDBNameRel == nil {
		allCrawledElements = append(allCrawledElements, extIDDBNameRel)
	}
	rel, errRel := utils.CreateRelationship(mariaCrawler.Host, mariaCrawler.DBName, agent.RelationshipExternalSourceSideType, agent.ErTypeRelation, crawlTime)
	if errRel == nil {
		allCrawledElements = append(allCrawledElements, rel)
	}

	// get table names in schema
	tableNames, _ := mariaCrawler.GetTableNames(mariaCrawler.DBName)

	for _, tableName := range tableNames {
		internalTableName := generateInternalName(mariaCrawler.scopeID, schemaName, tableName)
		// get table data
		tableData, errTableData := mariaCrawler.GetTableData(schemaName, tableName)
		if errTableData != nil {
			// we need to move on because we cannot add either indexes or relationships to this specific table
			continue
		}

		// add constraints relationships
		for _, constraint := range tableData.Constraints {
			if constraint.Type != mariadb.MARIADB_CONSTRAINT_FK {
				continue
			}

			for _, destination := range constraint.Destinations {
				// add the referenced tableName in the current elem's relations
				rel, errRel := utils.CreateRelationship(internalTableName, destination.Table, agent.RelationshipType, agent.ErTypeRelation, crawlTime)
				if errRel == nil {
					allCrawledElements = append(allCrawledElements, rel)
				}
			}
		}

		// get table indexes
		tableIndexes, _ := mariaCrawler.getTableIndexes(schemaName, tableName)
		for _, tableIndex := range tableIndexes {
			indexInternalName := generateInternalName(mariaCrawler.scopeID, schemaName, tableIndex.Name)
			indexElem, errIndexElem := utils.CreateElement(tableIndex, tableIndex.Name, indexInternalName, mariadb.MARIADB_TYPE_INDEX, agent.StatusNoStatus, "", crawlTime)
			if errIndexElem != nil {
				log.Info().Msgf("Cannot create table index element for index: %s because %s", tableIndex.Name, errIndexElem.Error())
				continue
			}
			allCrawledElements = append(allCrawledElements, indexElem)
			tableData.Indexes = append(tableData.Indexes, indexInternalName)

			relTableIndex, errRelTableIndex := utils.CreateRelationship(internalTableName, indexInternalName, agent.RelationshipType, agent.ErTypeRelation, crawlTime)
			if errRelTableIndex == nil {
				allCrawledElements = append(allCrawledElements, relTableIndex)
			}

			relDBNameIndex, errRelDBNameIndex := utils.CreateRelationship(mariaCrawler.DBName, indexInternalName, agent.RelationshipType, agent.ErTypeRelation, crawlTime)
			if errRelDBNameIndex == nil {
				allCrawledElements = append(allCrawledElements, relDBNameIndex)
			}
		}

		tableElem, errTableElem := utils.CreateElement(tableData, tableData.Name, internalTableName, mariadb.MARIADB_TYPE_TABLE, agent.StatusNoStatus, "", crawlTime)
		if errTableElem != nil {
			continue
		}
		allCrawledElements = append(allCrawledElements, tableElem)
	}

	crawledData := agent.CrawledData{
		Data: allCrawledElements,
	}

	mariaCrawler.outputChannel <- &agent.CloudCrawlData{
		Timestamp:       time.Now().UTC(),
		DataSource:      *mariaCrawler.dataSource,
		CrawledData:     crawledData,
		CrawlInternalID: schemaName,
	}

	return nil, nil
}
