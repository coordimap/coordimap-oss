package awsflowlogs

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coordimap/agent/pkg/utils"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/coordimap/agent/pkg/domain/awsflowlogs"
	"github.com/rs/zerolog/log"
)

func NewAWSFlowLogs(dataSource *agent.DataSource, outChannel chan *agent.CloudCrawlData) (Crawler, error) {
	crawler := &awsFlowLogsCrawler{
		logFormat:      "${account-id} ${action} ${az-id} ${bytes} ${dstaddr} ${dstport} ${end} ${flow-direction} ${instance-id} ${interface-id} ${log-status} ${packets} ${pkt-dst-aws-service} ${pkt-dstaddr} ${pkt-src-aws-service} ${pkt-srcaddr} ${protocol} ${region} ${srcaddr} ${srcport} ${start} ${sublocation-id} ${sublocation-type} ${subnet-id} ${tcp-flags} ${traffic-path} ${type} ${version} ${vpc-id}",
		bucketName:     "",
		region:         "",
		outputChannel:  outChannel,
		crawlInterval:  DEFAULT_CRAWL_TIME,
		dataSource:     dataSource,
		lastHandledKey: "",
		mutex:          sync.Mutex{},
		stateFilename:  "./flowLogsState.json",
	}

	flowLogsState, errFlowLogState := loadState(crawler.stateFilename)
	if errFlowLogState == nil {
		crawler.lastHandledKey = flowLogsState.LastFileProcessed
	}

	for _, dsConfig := range dataSource.Config.ValuePairs {
		switch dsConfig.Key {
		case "log_format":
			switch dsConfig.Value {
			case AWS_FLOW_LOG_FORMAT_DEFAULT:
				crawler.logFormat = AWS_FLOW_LOG_FORMAT_DEFAULT

			case AWS_FLOW_LOG_FORMAT_TYPE_ALL:
				crawler.logFormat = AWS_FLOW_LOG_FORMAT_ALL

			default:
				crawler.logFormat = dsConfig.Value

			}

		case "region":
			crawler.region = dsConfig.Value

		case "bucket_name":
			crawler.bucketName = dsConfig.Value

		case "account_id":
			crawler.accountID = dsConfig.Value

		case "crawl_interval":
			amountStr := string(dsConfig.Value[:len(dsConfig.Value)-1])
			durationStr := string(dsConfig.Value[len(dsConfig.Value)-1])

			amount, errConv := strconv.ParseInt(amountStr, 10, 32)
			if errConv != nil {
				return crawler, errConv
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

	awsSession, errAwsSession := connectToAWS(crawler.region)
	if errAwsSession != nil {
		return crawler, errAwsSession
	}

	crawler.awsSession = s3.New(awsSession)

	return crawler, nil
}

func (crawler *awsFlowLogsCrawler) Crawl() {
	crawlTicker := time.NewTicker(crawler.crawlInterval)

	log.Info().Msgf("Starting ticker for: %s", crawler.scopeID)
	for range crawlTicker.C {
		go func() {
			crawledData, errCrawl := crawler.crawl()
			if errCrawl != nil {
				// do not ship any data
				log.Info().Msg(errCrawl.Error())
				return
			}
			// ship the crawledData to the backend
			log.Info().Msgf("Crawled %d AWS Flow Logs elements for connection %s", len(crawledData.CrawledData.Data), crawler.scopeID)
			crawler.outputChannel <- crawledData
		}()
	}
}

func (crawler *awsFlowLogsCrawler) crawl() (*agent.CloudCrawlData, error) {
	crawlTime := time.Now().UTC()
	crawler.mutex.Lock()
	defer crawler.mutex.Unlock()

	flowLogsState, errFlowLogState := loadState(crawler.stateFilename)
	if errFlowLogState == nil {
		crawler.lastHandledKey = flowLogsState.LastFileProcessed
	}

	allCrawledElements := []*agent.Element{}
	allFlows := map[string]awsflowlogs.FlowRelation{}
	input := &s3.ListObjectsV2Input{
		Bucket:     &crawler.bucketName,
		StartAfter: &crawler.lastHandledKey,
	}
	objects, errListObjects := crawler.awsSession.ListObjectsV2(input)
	if errListObjects != nil {
		return nil, errListObjects
	}

	for _, object := range objects.Contents {
		if time.Now().UTC().Add(-DEFAULT_CRAWL_TIME).Before(*object.LastModified) || !strings.HasSuffix(*object.Key, ".gz") {
			continue
		}

		params := &s3.GetObjectInput{
			Bucket: aws.String(crawler.bucketName),
			Key:    aws.String(*object.Key),
		}
		resp, errGetObject := crawler.awsSession.GetObject(params)
		if errGetObject != nil {
			// TODO: log here
			continue
		}

		reader, _ := gzip.NewReader(resp.Body)

		r := bufio.NewReader(reader)
		scanner := bufio.NewScanner(r)

		for scanner.Scan() {
			row := strings.Split(scanner.Text(), " ")

			flowLog := &awsflowlogs.AWSFlowLog{
				Version:          getRowValue(row, crawler.logFormat, LOG_FIELD_VERSION),
				AccountID:        getRowValue(row, crawler.logFormat, LOG_FIELD_ACCOUNT_ID),
				InterfaceID:      getRowValue(row, crawler.logFormat, LOG_FIELD_INTERFACE_ID),
				SrcAddr:          getRowValue(row, crawler.logFormat, LOG_FIELD_SRC_ADDR),
				SrcPort:          getRowValue(row, crawler.logFormat, LOG_FIELD_SRC_PORT),
				DstAddr:          getRowValue(row, crawler.logFormat, LOG_FIELD_DST_ADDR),
				DstPort:          getRowValue(row, crawler.logFormat, LOG_FIELD_DST_PORT),
				Protocol:         getRowValue(row, crawler.logFormat, LOG_FIELD_PROTOCOL),
				Packets:          getRowValue(row, crawler.logFormat, LOG_FIELD_PACKETS),
				Bytes:            getRowValue(row, crawler.logFormat, LOG_FIELD_BYTES),
				Start:            getRowValue(row, crawler.logFormat, LOG_FIELD_START),
				End:              getRowValue(row, crawler.logFormat, LOG_FIELD_END),
				Action:           getRowValue(row, crawler.logFormat, LOG_FIELD_ACTION),
				LogStatus:        getRowValue(row, crawler.logFormat, LOG_FIELD_LOG_STATUS),
				VpcID:            getRowValue(row, crawler.logFormat, LOG_FIELD_VPC_ID),
				SubnetID:         getRowValue(row, crawler.logFormat, LOG_FIELD_SUBNET_ID),
				InstanceID:       getRowValue(row, crawler.logFormat, LOG_FIELD_INSTANCE_ID),
				TCPFlags:         getRowValue(row, crawler.logFormat, LOG_FIELD_TCP_FLAGS),
				Type:             getRowValue(row, crawler.logFormat, LOG_FIELD_TYPE),
				PktSrcAddr:       getRowValue(row, crawler.logFormat, LOG_FIELD_PKT_SRCADDR),
				PktDstAddr:       getRowValue(row, crawler.logFormat, LOG_FIELD_PKT_DSTADDR),
				Region:           getRowValue(row, crawler.logFormat, LOG_FIELD_REGION),
				AZID:             getRowValue(row, crawler.logFormat, LOG_FIELD_AZ_ID),
				SublocationType:  getRowValue(row, crawler.logFormat, LOG_FIELD_SUBLOCATION_TYPE),
				SublocationID:    getRowValue(row, crawler.logFormat, LOG_FIELD_SUBLOCATION_ID),
				PktSrcAWSService: getRowValue(row, crawler.logFormat, LOG_FIELD_PKT_SRC_AWS_SERVICE),
				PktDstAWSService: getRowValue(row, crawler.logFormat, LOG_FIELD_PKT_DST_AWS_SERVICE),
				FlowDirection:    getRowValue(row, crawler.logFormat, LOG_FIELD_FLOW_DIRECTION),
				TrafficPath:      getRowValue(row, crawler.logFormat, LOG_FIELD_TRAFFIC_PATH),
			}

			// check if the flow is an internal flow otherwise discard it as we are not interested in outside flows
			if !isInternalFlow(*flowLog) {
				continue
			}

			// check if element exists
			relation := fmt.Sprintf("%s-%s", flowLog.SrcAddr, flowLog.DstAddr)

			// update either src or dst based on FlowDirection
			foundRelation, relationExists := allFlows[relation]
			if !relationExists {
				foundRelation = awsflowlogs.FlowRelation{
					Src: nil,
					Dst: nil,
				}
			}
			if flowLog.FlowDirection == "ingress" {
				foundRelation.Dst = flowLog
			} else if flowLog.FlowDirection == "egress" {
				foundRelation.Src = flowLog
			}

			allFlows[relation] = foundRelation
		}

		log.Info().Msgf("Processing AWS Flow Log file: %s", *object.Key)

		crawler.lastHandledKey = *object.Key
		reader.Close()
	}

	for _, foundFlow := range allFlows {
		if foundFlow.Src == nil || foundFlow.Dst == nil {
			continue
		}

		unixTime, errUnixTime := strconv.ParseInt(foundFlow.Src.End, 10, 64)
		if errUnixTime != nil {
			// TODO: log this
			continue
		}

		tm := time.Unix(unixTime, 0).UTC()
		fmt.Println(tm)

		// FIXME: add a custom date time
		elementName := fmt.Sprintf("%s-%s", foundFlow.Src.InterfaceID, foundFlow.Dst.InterfaceID)
		flowElement, errFlowElement := utils.CreateElement(foundFlow, elementName, elementName, awsflowlogs.AWS_FLOW_LOGS_TYPE_EC2_SKIPINSERT, agent.StatusNoStatus, "", crawlTime)
		if errFlowElement != nil {
			continue
		}

		allCrawledElements = append(allCrawledElements, flowElement)
	}

	// Create an element here and append to crawledElements
	crawledData := agent.CrawledData{
		Data: allCrawledElements,
	}

	errFlowLogStateWrite := writeState(crawler.stateFilename, crawler.lastHandledKey)
	if errFlowLogStateWrite != nil {
		return &agent.CloudCrawlData{
			Timestamp:   crawlTime,
			DataSource:  *crawler.dataSource,
			CrawledData: agent.CrawledData{},
		}, fmt.Errorf("could not store flow logs state in file %s because %w", crawler.stateFilename, errFlowLogStateWrite)
	}

	return &agent.CloudCrawlData{
		Timestamp:       crawlTime,
		DataSource:      *crawler.dataSource,
		CrawledData:     crawledData,
		CrawlInternalID: crawler.scopeID,
	}, nil
}
