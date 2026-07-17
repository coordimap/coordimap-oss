package mariadb

import (
	"database/sql"
	"time"

	"github.com/coordimap/agent/pkg/domain/agent"
)

type mariadbCrawler struct {
	dbConn            *sql.DB
	dataSource        *agent.DataSource
	outputChannel     chan *agent.CloudCrawlData
	Host              string
	User              string
	Pass              string
	DBName            string
	SSLMode           string
	externalMappingID string
	crawlInterval     time.Duration
	scopeID           string
}

type Crawler interface {
	Crawl()
}
