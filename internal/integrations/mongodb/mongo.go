package mongodb

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/coordimap/agent/pkg/utils"

	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/coordimap/agent/pkg/domain/mongodb"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func NewMongoDBCrawler(dataSource *agent.DataSource, outChannel chan *agent.CloudCrawlData) (Crawler, error) {
	// 1. initialize postgresCrawler with default values
	crawler := mongoCrawler{
		outputChannel: outChannel,
		crawlInterval: 30 * time.Second,
		Host:          "localhost",
		User:          "mongo",
		Pass:          "",
		DBName:        []string{},
		dataSource:    dataSource,
	}

	// 2. populate postgresCrawler with the provided configuration
	dbNameStar := ""
	for _, dsConfig := range dataSource.Config.ValuePairs {
		switch dsConfig.Key {
		case "db_name":
			if dsConfig.Value != "*" {
				crawler.DBName = strings.Split(dsConfig.Value, ",")
			} else {
				dbNameStar = dsConfig.Value
			}

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

	// 3. connect to the DB
	db, errDBConn := connectToDB(crawler.Host, crawler.User, crawler.Pass)
	if errDBConn != nil {
		log.Error().Msgf("Cannot connect to the MongoDB of the config %s", crawler.scopeID)
		return &crawler, errDBConn
	}

	// 4. in case of '*' get the names of all the databases
	if dbNameStar == "*" {
		dbNames, errListDBNames := db.ListDatabaseNames(context.Background(), bson.D{})
		if errListDBNames != nil {
			return nil, fmt.Errorf("cannot retrieve the database names because %w", errListDBNames)
		}
		crawler.DBName = dbNames
	}

	crawler.dbConn = db
	if crawler.scopeID == "" {
		return nil, fmt.Errorf("MongoDB crawler config error: scope_id must be provided for data source %s", crawler.dataSource.DataSourceID)
	}

	return &crawler, nil
}

func connectToDB(host, user, pass string) (*mongo.Client, error) {
	connectURI := fmt.Sprintf("mongodb+srv://%s:%s@%s/?retryWrites=true&w=majority", user, pass, host)

	serverAPIOptions := options.ServerAPI(options.ServerAPIVersion1)
	clientOptions := options.Client().ApplyURI(connectURI).SetServerAPIOptions(serverAPIOptions)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func (mongoCrawler *mongoCrawler) Crawl() {
	crawlTicker := time.NewTicker(mongoCrawler.crawlInterval)

	log.Info().Msgf("Starting ticker for: %s", mongoCrawler.scopeID)
	for range crawlTicker.C {
		_, errCrawl := mongoCrawler.crawl()
		log.Info().Msgf("Crawling MongoDB for %s", mongoCrawler.scopeID)
		if errCrawl != nil {
			// do not ship any data
			log.Info().Msgf("%s", errCrawl.Error())
			continue
		}
	}
}

func (mongoCrawler *mongoCrawler) crawl() (*agent.CloudCrawlData, error) {
	crawlTime := time.Now().UTC()
	for _, dbName := range mongoCrawler.DBName {

		allCrawledElements := []*agent.Element{}
		dbHandle := mongoCrawler.dbConn.Database(dbName)

		// get the mongo database
		mongoDB := mongoCrawler.getMongodbDatabase(dbName)
		dbInternalName := fmt.Sprintf("%s/%s", mongoCrawler.scopeID, mongoDB.Name)
		dbElem, errDBElem := utils.CreateElement(mongoDB, mongoDB.Name, dbInternalName, mongodb.MONGODB_TYPE_DATABASE, agent.StatusNoStatus, "", crawlTime)
		if errDBElem != nil {
			return nil, errDBElem
		}
		allCrawledElements = append(allCrawledElements, dbElem)

		// get collections
		collections, errCollections := dbHandle.ListCollectionSpecifications(context.Background(), bson.D{})
		if errCollections != nil {
			return nil, errCollections
		}

		for _, collection := range collections {
			collectionHandle := dbHandle.Collection(collection.Name)
			mongoCollection, errMongoCollection := mongoCrawler.getMongodbDatabaseCollection(dbHandle, collection.Name)
			if errMongoCollection != nil {
				log.Error().Msgf("could not get collection: %s and data source: %s", collection.Name, mongoCrawler.scopeID)
				continue
			}
			collectionInternalName := fmt.Sprintf("%s/%s", dbInternalName, collection.Name)
			collectionElem, errCollectionElem := utils.CreateElement(mongoCollection, mongoCollection.Name, collectionInternalName, mongodb.MONGODB_TYPE_COLLECTION, agent.StatusNoStatus, "", crawlTime)
			if errCollectionElem != nil {
				log.Error().Msgf("could not create collection element for collection: %s and data source: %s", collection.Name, mongoCrawler.scopeID)
				continue
			}
			allCrawledElements = append(allCrawledElements, collectionElem)
			relDbColl, errRelDbColl := utils.CreateRelationship(dbInternalName, collectionInternalName, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRelDbColl == nil {
				allCrawledElements = append(allCrawledElements, relDbColl)
			}

			// get indexes
			collectionIndexes, errCollectionIndexes := mongoCrawler.listCollectionIndexes(collectionHandle)
			if errCollectionIndexes != nil {
				log.Error().Msgf("could not get collection indexes for collection: %s and data source: %s", collection.Name, mongoCrawler.scopeID)
			}

			for _, foundIndex := range collectionIndexes {
				indexInternalName := fmt.Sprintf("%s/%s", collectionInternalName, foundIndex.Name)
				indexElem, errIndexElem := utils.CreateElement(foundIndex, foundIndex.Name, indexInternalName, mongodb.MONGODB_TYPE_INDEX, agent.StatusNoStatus, "", crawlTime)
				if errIndexElem != nil {
					log.Error().Msgf("could not create index element for index: %s, collection: %s and data source: %s", foundIndex.Name, collection.Name, mongoCrawler.scopeID)
					continue
				}
				allCrawledElements = append(allCrawledElements, indexElem)
				relCollIndex, errRelCollIndex := utils.CreateRelationship(collectionInternalName, indexInternalName, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRelCollIndex == nil {
					allCrawledElements = append(allCrawledElements, relCollIndex)
				}
			}
		}

		crawledData := agent.CrawledData{
			Data: allCrawledElements,
		}

		log.Info().Msgf("Crawled %d MongoDB elements for connection %s and database %s", len(allCrawledElements), mongoCrawler.scopeID, dbName)

		mongoCrawler.outputChannel <- &agent.CloudCrawlData{
			Timestamp:       crawlTime,
			DataSource:      *mongoCrawler.dataSource,
			CrawledData:     crawledData,
			CrawlInternalID: dbName,
		}
	}
	return nil, nil
}
