package integrations

import (
	"fmt"

	"github.com/coordimap/agent/internal/cloud/flows"
	"github.com/coordimap/agent/internal/cloud/gcp"
	"github.com/coordimap/agent/internal/integrations/aws"
	awsflowlogs "github.com/coordimap/agent/internal/integrations/aws_flow_logs"
	"github.com/coordimap/agent/internal/integrations/kubernetes"
	"github.com/coordimap/agent/internal/integrations/mariadb"
	"github.com/coordimap/agent/internal/integrations/mongodb"
	"github.com/coordimap/agent/internal/integrations/postgres"

	"github.com/coordimap/agent/pkg/domain/agent"
)

func IntegrationsFactory(name string, dataSource *agent.DataSource, outChannel chan *agent.CloudCrawlData) (Crawler, error) {
	switch name {
	case INTEGRATION_AWS:
		return aws.MakeAWS(dataSource, outChannel)

	case INTEGRATION_POSTGRES:
		return postgres.NewPostgresCrawler(dataSource, outChannel)

	case INTEGRATION_KUBERNETES:
		return kubernetes.MakeKubernetesCrawler(dataSource, outChannel)

	case INTEGRATION_AWS_FLOW_LOGS:
		return awsflowlogs.NewAWSFlowLogs(dataSource, outChannel)

	case INTEGRATION_MONGODB:
		return mongodb.NewMongoDBCrawler(dataSource, outChannel)

	case INTEGRATION_MARIADB:
		return mariadb.NewMariadbCrawler(dataSource, outChannel)

	case INTEGRATION_MYSQL:
		return mariadb.NewMysqlCrawler(dataSource, outChannel)

	case INTEGRATION_GCP:
		return gcp.NewGCPCrawler(dataSource, outChannel)

	case INTEGRATION_EBPF_FLOWS:
		return flows.NewFlowsCrawler(dataSource, outChannel)

	default:
		return nil, fmt.Errorf("unknown integration %s", name)
	}
}
