package aws

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/coordimap/agent/pkg/utils"

	"github.com/rs/zerolog/log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/coordimap/agent/pkg/domain/agent"
	aws_shared_model "github.com/coordimap/agent/pkg/domain/aws"
)

type AwsCrawl struct {
	ds              *agent.DataSource
	outChannel      chan *agent.CloudCrawlData
	secretAccessKey string
	accessKeyID     string
	scopeID         string
}

// MakeAWS creates an AWS cloud struct
func MakeAWS(dsConfig *agent.DataSource, outChannel chan *agent.CloudCrawlData) (*AwsCrawl, error) {
	secretAccessKey := ""
	accessKeyID := ""

	scopeID := ""
	for _, dsConfigValuePair := range dsConfig.Config.ValuePairs {
		switch dsConfigValuePair.Key {
		case "access_key_id":
			accessKeyID, _ = utils.LoadValueFromEnvConfig(dsConfigValuePair.Value)
		case "secret_access_key":
			secretAccessKey, _ = utils.LoadValueFromEnvConfig(dsConfigValuePair.Value)
		case "scope_id":
			scopeID = dsConfigValuePair.Value
		}
	}

	if scopeID == "" {
		return nil, fmt.Errorf("AWS crawler config error: scope_id must be provided for data source %s", dsConfig.DataSourceID)
	}

	return &AwsCrawl{
		ds:              dsConfig,
		outChannel:      outChannel,
		secretAccessKey: secretAccessKey,
		accessKeyID:     accessKeyID,
		scopeID:         scopeID,
	}, nil
}

func (awsCrawl *AwsCrawl) Crawl() {
	durationInterval, errInterval := awsCrawl.GetCrawlInterval()
	log.Info().Msgf("Ticker duration is %d seconds", durationInterval/time.Second)
	if errInterval != nil {
		// stop crawling
		log.Info().Msgf("Error in getting the interval from the configuration. %s", errInterval.Error())
		return
	}

	crawlTicker := time.NewTicker(durationInterval)

	log.Info().Msgf("Starting ticker for: %s", awsCrawl.scopeID)
	for range crawlTicker.C {
		crawledData, errCrawl := awsCrawl.crawl()
		if errCrawl != nil {
			// do not ship any data
			log.Info().Msg(errCrawl.Error())
			continue
		}
		// ship the crawledData to the backend
		log.Info().Msgf("Crawled %d AWS cloud elements for connection %s", len(crawledData.CrawledData.Data), awsCrawl.ds.Info.Name)
		awsCrawl.outChannel <- crawledData
	}
}

func (awsCrawl *AwsCrawl) GetCrawlInterval() (time.Duration, error) {
	for _, config := range awsCrawl.ds.Config.ValuePairs {
		if config.Key == "crawl_interval" {
			amountStr := string(config.Value[:len(config.Value)-1])
			durationStr := string(config.Value[len(config.Value)-1])

			amount, errConv := strconv.ParseInt(amountStr, 10, 32)
			if errConv != nil {
				return 0, errConv
			}

			switch durationStr {
			case "s":
				return time.Duration(amount) * time.Second, nil

			case "m":
				return time.Duration(amount) * time.Minute, nil

			default:
				return 0, fmt.Errorf("the provided duration time of %s is not one of (s, m)", durationStr)
			}
		}
	}

	return 0, errors.New("could not find crawl_interval configuration value")
}

// Crawl retrieves all the VPCs found in the specified region
// It returns a list of VPC IDs.
// 1. Create intial session and retrieve all the regions.
// 2. Loop through all the regions and store slices of each element, i.e. allVPCs
// 3. Assign all the elements to the CloudData object
// 4. return the CloudData object
func (awsCrawl *AwsCrawl) crawl() (*agent.CloudCrawlData, error) {
	crawlTime := time.Now().UTC()
	var crawledData agent.CrawledData

	initSession, _ := session.NewSession(
		&aws.Config{
			Region: aws.String("us-east-1"),
			Credentials: credentials.NewCredentials(&credentials.StaticProvider{
				Value: credentials.Value{
					AccessKeyID:     awsCrawl.accessKeyID,
					SecretAccessKey: awsCrawl.secretAccessKey,
				},
			}),
		},
	)

	awsRegions, errRegions := describeAllRegions(initSession, awsCrawl.scopeID, crawlTime)
	if errRegions != nil {
		return nil, fmt.Errorf("could not retrieve AWS regions")
	}

	crawledData.Data = append(crawledData.Data, awsRegions...)
	accountID, _ := getAwsAccountID(initSession)
	owner := []*string{accountID}
	ownerElem, errOwnerElem := utils.CreateAWSElement(owner, *accountID, *accountID, aws_shared_model.AwsTypeOwner, agent.StatusNoStatus, "", crawlTime)
	if errOwnerElem == nil {
		crawledData.Data = append(crawledData.Data, ownerElem)
	}
	results := make(chan []*agent.Element, 5000)
	var wg sync.WaitGroup

	for _, region := range awsRegions {
		// var err error = nil
		regionSession, errRegionSession := session.NewSession(
			&aws.Config{
				Region: aws.String(region.Name),
				Credentials: credentials.NewCredentials(&credentials.StaticProvider{
					Value: credentials.Value{
						AccessKeyID:     awsCrawl.accessKeyID,
						SecretAccessKey: awsCrawl.secretAccessKey,
					},
				}),
			},
		)

		if errRegionSession != nil {
			log.Err(errRegionSession).Msg("Region Session creation.")
			return nil, fmt.Errorf("Cannot create session for the AWS region: %w", errRegionSession)
		}

		wg.Add(1)
		go worker("vpcs", owner, regionSession, results, &wg, awsCrawl.scopeID, crawlTime)

		wg.Add(1)
		go worker("route_tables", owner, regionSession, results, &wg, awsCrawl.scopeID, crawlTime)

		wg.Add(1)
		go worker("dhcp_options", owner, regionSession, results, &wg, awsCrawl.scopeID, crawlTime)

		wg.Add(1)
		go worker("subnets", owner, regionSession, results, &wg, awsCrawl.scopeID, crawlTime)

		wg.Add(1)
		go worker("natgws", owner, regionSession, results, &wg, awsCrawl.scopeID, crawlTime)

		wg.Add(1)
		go worker("net_acls", owner, regionSession, results, &wg, awsCrawl.scopeID, crawlTime)

		wg.Add(1)
		go worker("azs", owner, regionSession, results, &wg, awsCrawl.scopeID, crawlTime)

		wg.Add(1)
		go worker("amis", owner, regionSession, results, &wg, awsCrawl.scopeID, crawlTime)

		wg.Add(1)
		go worker("ec2", owner, regionSession, results, &wg, awsCrawl.scopeID, crawlTime)

		wg.Add(1)
		go worker("sec_groups", owner, regionSession, results, &wg, awsCrawl.scopeID, crawlTime)

		wg.Add(1)
		go worker("vols", owner, regionSession, results, &wg, awsCrawl.scopeID, crawlTime)

		wg.Add(1)
		go worker("lbs", owner, regionSession, results, &wg, awsCrawl.scopeID, crawlTime)

		wg.Add(1)
		go worker("lambdas", owner, regionSession, results, &wg, awsCrawl.scopeID, crawlTime)

		wg.Add(1)
		go worker("rds", owner, regionSession, results, &wg, awsCrawl.scopeID, crawlTime)

		wg.Add(1)
		go worker(aws_shared_model.AwsTypeEKS, owner, regionSession, results, &wg, awsCrawl.scopeID, crawlTime)

		wg.Add(1)
		go worker(aws_shared_model.AwsTypeECRRepository, owner, regionSession, results, &wg, awsCrawl.scopeID, crawlTime)

		wg.Add(1)
		go worker(aws_shared_model.AwsTypeAutoscalingGroup, owner, regionSession, results, &wg, awsCrawl.scopeID, crawlTime)
	}

	wg.Add(1)
	go worker("s3-buckets", owner, initSession, results, &wg, awsCrawl.scopeID, crawlTime)

	ownerElement, errOwnerElement := utils.CreateElement(owner, *owner[0], *owner[0], aws_shared_model.AwsTypeOwner, agent.StatusNoStatus, "", crawlTime)
	if errOwnerElement == nil {
		results <- []*agent.Element{ownerElement}
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for res := range results {
		if len(res) != 0 {
			crawledData.Data = append(crawledData.Data, res...)
			log.Info().Msgf("Got: %s", res[0].Name)
			// log.Info().Msgf("%s  ---   %v", res[0].ID, res[0].Data)
		}
	}

	// return &crawledData, nil
	return &agent.CloudCrawlData{
		Timestamp:       crawlTime,
		DataSource:      *awsCrawl.ds,
		CrawledData:     crawledData,
		CrawlInternalID: awsCrawl.ds.Info.Name,
	}, nil
}
