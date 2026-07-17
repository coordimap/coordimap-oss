package mongodb

import (
	"time"

	"github.com/coordimap/agent/pkg/domain/agent"
	"go.mongodb.org/mongo-driver/mongo"
)

type mongoCrawler struct {
	Host          string
	User          string
	Pass          string
	DBName        []string
	dbConn        *mongo.Client
	outputChannel chan *agent.CloudCrawlData
	crawlInterval time.Duration
	dataSource    *agent.DataSource
	scopeID       string
}

type Crawler interface {
	Crawl()
}
