package awsflowlogs

import (
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/coordimap/agent/pkg/domain/awsflowlogs"
)

const (
	AWS_FLOW_LOG_FORMAT_TYPE_DEFAULT = "default"
	AWS_FLOW_LOG_FORMAT_TYPE_ALL     = "all"
)

const (
	AWS_FLOW_LOG_FORMAT_DEFAULT = "${version} ${account-id} ${interface-id} ${srcaddr} ${dstaddr} ${srcport} ${dstport} ${protocol} ${packets} ${bytes} ${start} ${end} ${action} ${log-status}"
	AWS_FLOW_LOG_FORMAT_ALL     = "${account-id} ${action} ${az-id} ${bytes} ${dstaddr} ${dstport} ${end} ${flow-direction} ${instance-id} ${interface-id} ${log-status} ${packets} ${pkt-dst-aws-service} ${pkt-dstaddr} ${pkt-src-aws-service} ${pkt-srcaddr} ${protocol} ${region} ${srcaddr} ${srcport} ${start} ${sublocation-id} ${sublocation-type} ${subnet-id} ${tcp-flags} ${traffic-path} ${type} ${version} ${vpc-id}"
)

const (
	LOG_FIELD_VERSION             = "version"
	LOG_FIELD_ACCOUNT_ID          = "account-id"
	LOG_FIELD_INTERFACE_ID        = "interface-id"
	LOG_FIELD_SRC_ADDR            = "srcaddr"
	LOG_FIELD_DST_ADDR            = "dstaddr"
	LOG_FIELD_SRC_PORT            = "srcport"
	LOG_FIELD_DST_PORT            = "dstport"
	LOG_FIELD_PROTOCOL            = "protocol"
	LOG_FIELD_PACKETS             = "packets"
	LOG_FIELD_BYTES               = "bytes"
	LOG_FIELD_START               = "start"
	LOG_FIELD_END                 = "end"
	LOG_FIELD_ACTION              = "action"
	LOG_FIELD_LOG_STATUS          = "log-status"
	LOG_FIELD_VPC_ID              = "vpc-id"
	LOG_FIELD_INSTANCE_ID         = "subnet-id"
	LOG_FIELD_SUBNET_ID           = "instance-id"
	LOG_FIELD_TCP_FLAGS           = "tcp-flags"
	LOG_FIELD_TYPE                = "type"
	LOG_FIELD_PKT_SRCADDR         = "pkt-srcaddr"
	LOG_FIELD_PKT_DSTADDR         = "pkt-dstaddr"
	LOG_FIELD_REGION              = "region"
	LOG_FIELD_AZ_ID               = "az-id"
	LOG_FIELD_SUBLOCATION_TYPE    = "sublocation-type"
	LOG_FIELD_SUBLOCATION_ID      = "sublocation-id"
	LOG_FIELD_PKT_SRC_AWS_SERVICE = "pkt-src-aws-service"
	LOG_FIELD_PKT_DST_AWS_SERVICE = "pkt-dst-aws-service"
	LOG_FIELD_FLOW_DIRECTION      = "flow-direction"
	LOG_FIELD_TRAFFIC_PATH        = "traffic-path"
)

const DEFAULT_CRAWL_TIME = 15 * time.Minute

type awsFlowLogsCrawler struct {
	logFormat          string
	bucketName         string
	region             string
	accountID          string
	outputChannel      chan *agent.CloudCrawlData
	crawlInterval      time.Duration
	dataSource         *agent.DataSource
	awsSession         *s3.S3
	foundRelationships map[string]awsflowlogs.AWSFlowLog
	lastHandledKey     string
	scopeID            string
	mutex              sync.Mutex
	stateFilename      string
}

type Crawler interface {
	Crawl()
}

type floLogsState struct {
	LastFileProcessed string `json:"last_file_processed"`
}
