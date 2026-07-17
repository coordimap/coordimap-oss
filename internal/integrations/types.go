package integrations

import (
	"time"
)

const (
	INTEGRATION_POSTGRES      = "postgres"
	INTEGRATION_AWS           = "aws"
	INTEGRATION_KUBERNETES    = "kubernetes"
	INTEGRATION_AWS_FLOW_LOGS = "aws_flow_logs"
	INTEGRATION_MONGODB       = "mongodb"
	INTEGRATION_MARIADB       = "mariadb"
	INTEGRATION_MYSQL         = "mysql"
	INTEGRATION_GCP           = "gcp"
)

type BaseConfig struct {
	Output        chan []string
	Name          string
	Desc          string
	CrawlInterval time.Duration
}

type Crawler interface {
	Crawl()
}
