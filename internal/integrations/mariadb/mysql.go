package mariadb

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/coordimap/agent/pkg/utils"

	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/rs/zerolog/log"
)

func NewMysqlCrawler(dataSource *agent.DataSource, outChannel chan *agent.CloudCrawlData) (Crawler, error) {
	crawler := mariadbCrawler{
		dbConn:        nil,
		outputChannel: outChannel,
		crawlInterval: 30 * time.Second,
		Host:          "localhost",
		User:          "postgres",
		Pass:          "",
		DBName:        "postgres",
		dataSource:    dataSource,
		scopeID:       "",
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

		case "scope_id":
			crawler.scopeID = dsConfig.Value

		case "db_host":
			crawler.Host = dsConfig.Value

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
		return nil, fmt.Errorf("MySQL crawler config error: scope_id must be provided for data source %s", crawler.dataSource.DataSourceID)
	}

	// 3. connect to the DB
	db, errDBConn := connectToDB(crawler.User, crawler.Pass, crawler.Host, "3306", crawler.DBName)
	if errDBConn != nil {
		log.Error().Msgf("Cannot connect to the MySQL of the config %s", crawler.scopeID)
		return &crawler, errDBConn
	}
	crawler.dbConn = db

	return &crawler, nil
}
