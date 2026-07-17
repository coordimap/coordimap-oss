package postgres

import (
	"database/sql"
	"time"

	"github.com/coordimap/agent/pkg/domain/agent"
)

type postgresCrawler struct {
	dataSource        *agent.DataSource
	outputChannel     chan *agent.CloudCrawlData
	dbConn            *sql.DB
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
