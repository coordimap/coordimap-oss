package gcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	cloudutils "github.com/coordimap/agent/internal/cloud/utils"
	"github.com/coordimap/agent/internal/metrics"

	"cloud.google.com/go/compute/metadata"
	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/logging/v2"
	"google.golang.org/api/monitoring/v3"
	"google.golang.org/api/option"
)

func NewGCPCrawler(dataSource *agent.DataSource, outChannel chan *agent.CloudCrawlData) (Crawler, error) {
	gcpCrawler := gcpCrawler{
		clientOpts:          []option.ClientOption{},
		crawlInterval:       30 * time.Second,
		outputChan:          outChannel,
		dataSource:          *dataSource,
		InGCPEnvironment:    false,
		credentialsFile:     "",
		ConfiguredProjectID: "",
		logClient:           nil,
		monitoringClient:    nil,
		includedRegions:     []string{},
		internalIDMapper:    map[string]string{},
		cloudSQLZones:       map[string]string{},
		externalMappings:    map[string]string{},
		metricRules:         []metrics.RuleConfig{},
		scopeID:             "",
	}

	flowConfigured := false
	metricRulesConfigured := len(dataSource.Config.MetricRules) > 0
	gcpCrawler.metricRules = append(gcpCrawler.metricRules, dataSource.Config.MetricRules...)

	for _, config := range dataSource.Config.ValuePairs {
		switch config.Key {
		case gcpConfigInGoogleCloud:
			if strings.Compare(config.Value, "true") != 0 {
				continue
			}
			if len(gcpCrawler.clientOpts) > 0 {
				log.Info().Str("DataSourceID", gcpCrawler.dataSource.DataSourceID).Str("DataSource Type", gcpCrawler.dataSource.Info.Type).Msg("Will not take into account the credentials file as it seems that the dsta source credentials were already configured")
			}

			if !metadata.OnGCE() {
				return nil, errors.New("the agent is instructed that it will run in the Google Cloud but unfortunately no metadata server was found")
			}

			ts := google.ComputeTokenSource("")
			gcpCrawler.clientOpts = append(gcpCrawler.clientOpts, option.WithTokenSource(ts))

		case gcpConfigCredentialsFile:
			if len(gcpCrawler.clientOpts) > 0 {
				log.Info().Str("DataSourceID", gcpCrawler.dataSource.DataSourceID).Str("DataSource Type", gcpCrawler.dataSource.Info.Type).Msg("Will not take into account the credentials file as it seems that the dsta source credentials were already configured")
			}

			if _, err := os.Stat(config.Value); os.IsNotExist(err) {
				return nil, fmt.Errorf("credentials file not found: %s", config.Value)
			}
			gcpCrawler.credentialsFile = config.Value
			gcpCrawler.clientOpts = append(gcpCrawler.clientOpts, option.WithCredentialsFile(config.Value))

		case gcpConfigFlows:
			flowConfigured = true

		case gcpConfigExternalMappings:
			mappings, errMappings := cloudutils.SplitConfiguredMappings(config.Value)
			if errMappings != nil {
				log.Error().Str("ConfiguredMappings", config.Value).Msg("Could not generate and use mapping configs.")
				continue
			}

			gcpCrawler.externalMappings = mappings

		case gcpConfigIncludeRegions:
			gcpCrawler.includedRegions = append(gcpCrawler.includedRegions, strings.Split(config.Value, ",")...)

		case gcpConfigScopeID:
			gcpCrawler.scopeID = config.Value

		case gcpProjectID:
			if config.Value == "" {
				return nil, errors.New("project_name must not be empty")
			}
			gcpCrawler.ConfiguredProjectID = strings.ToLower(config.Value)

		case gcpConfigCrawlInterval:
			amountStr := string(config.Value[:len(config.Value)-1])
			durationStr := string(config.Value[len(config.Value)-1])

			amount, errConv := strconv.ParseInt(amountStr, 10, 32)
			if errConv != nil {
				return &gcpCrawler, errConv
			}

			switch durationStr {
			case "s":
				gcpCrawler.crawlInterval = time.Duration(amount) * time.Second

			case "m":
				gcpCrawler.crawlInterval = time.Duration(amount) * time.Minute

			}

		}
	}

	credsProjectID, errCredsProjectID := gcpCrawler.GetProjectID(context.Background())
	if errCredsProjectID != nil {
		return nil, errCredsProjectID
	}
	if gcpCrawler.ConfiguredProjectID != credsProjectID {
		return nil, fmt.Errorf("the configured ProjectID %s does not match the ProjectID %s from the authentication", gcpCrawler.ConfiguredProjectID, credsProjectID)
	}
	if gcpCrawler.scopeID == "" {
		return nil, fmt.Errorf("GCP crawler config error: scope_id must be provided for data source %s", gcpCrawler.dataSource.DataSourceID)
	}

	if flowConfigured {
		client, errClient := logging.NewService(context.Background(), gcpCrawler.clientOpts...)
		if errClient != nil {
			return nil, errClient
		}

		gcpCrawler.logClient = client
	}

	if metricRulesConfigured {
		monitoringClient, errMonitoringClient := monitoring.NewService(context.Background(), gcpCrawler.clientOpts...)
		if errMonitoringClient != nil {
			return nil, fmt.Errorf("could not create gcp monitoring client: %w", errMonitoringClient)
		}

		gcpCrawler.monitoringClient = monitoringClient
	}

	return &gcpCrawler, nil
}

func (crawler *gcpCrawler) validateConfig() bool {
	if crawler.ConfiguredProjectID != "" && crawler.dataSource.DataSourceID != "" {
		return false
	}

	return true
}

func (gcpCrawler *gcpCrawler) Crawl() {
	crawlTicker := time.NewTicker(gcpCrawler.crawlInterval)

	log.Info().Msgf("Starting ticker for: %s", gcpCrawler.dataSource.DataSourceID)
	for range crawlTicker.C {
		_, errCrawl := gcpCrawler.crawl()
		log.Info().Msgf("Crawling GCP Project for %s", gcpCrawler.dataSource.DataSourceID)
		if errCrawl != nil {
			// do not ship any data
			log.Info().Msg(errCrawl.Error())
			continue
		}
		// ship the crawledData to the backend
	}
}

func (gcpCrawler *gcpCrawler) GetProjectID(ctx context.Context) (string, error) {
	// Try to get project ID from credentials file first if available
	if gcpCrawler.credentialsFile != "" {
		projectID, err := GetProjectIDFromCredentialsFile(gcpCrawler.credentialsFile)
		if err == nil {
			return projectID, nil
		}
		// Log the error but continue with other methods
		log.Printf("Warning: Could not get project ID from credentials file: %v", err)
	}

	// Try metadata server if running on GCP
	if metadata.OnGCE() {
		projectID, err := metadata.ProjectIDWithContext(context.Background())
		if err != nil {
			return "", fmt.Errorf("failed to get project ID from metadata: %v", err)
		}
		return projectID, nil
	}

	// Try application default credentials as last resort
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", fmt.Errorf("failed to get project ID from default credentials: %v", err)
	}

	if creds.ProjectID == "" {
		return "", fmt.Errorf("no project ID found in any available source")
	}

	return creds.ProjectID, nil
}

func (gcpCrawler *gcpCrawler) crawl() (*agent.CloudCrawlData, error) {
	logger := log.With().Str("DataSourceType", "gcp").Str("ProjectID", gcpCrawler.ConfiguredProjectID).Str("DataSourceID", gcpCrawler.dataSource.DataSourceID).Logger()
	crawlTime := time.Now().UTC()
	allCrawledElemsAndRelationships := []*agent.Element{}

	bucketElems, errBucketElems := gcpCrawler.GetBuckets(crawlTime)
	if errBucketElems != nil {
		logger.Debug().Msgf("could not retrieve buckets because %s", errBucketElems.Error())
	} else {
		allCrawledElemsAndRelationships = append(allCrawledElemsAndRelationships, bucketElems...)
	}

	cloudRunElems, errCloudRunElems := gcpCrawler.GetCloudRuns(crawlTime)
	if errCloudRunElems != nil {
		logger.Err(errCloudRunElems).Msgf("could not retrieve cloud runs.")
	} else {
		allCrawledElemsAndRelationships = append(allCrawledElemsAndRelationships, cloudRunElems...)
	}

	computeElems, errComputeElems := gcpCrawler.GetComputeElems(crawlTime)
	if errComputeElems == nil {
		allCrawledElemsAndRelationships = append(allCrawledElemsAndRelationships, computeElems...)
	}

	gkeClusterElems, errGkeClusterElems := gcpCrawler.getGKEClusters(crawlTime)
	if errGkeClusterElems == nil {
		allCrawledElemsAndRelationships = append(allCrawledElemsAndRelationships, gkeClusterElems...)
	}

	sqlElems, errSqlElems := gcpCrawler.getSqlInstances(crawlTime)
	if errSqlElems == nil {
		allCrawledElemsAndRelationships = append(allCrawledElemsAndRelationships, sqlElems...)
	}

	iamElems, errIAMElems := gcpCrawler.getIAMElements(crawlTime)
	if errIAMElems != nil {
		logger.Err(errIAMElems).Msg("could not retrieve IAM elements")
	} else {
		allCrawledElemsAndRelationships = append(allCrawledElemsAndRelationships, iamElems...)
	}

	if gcpCrawler.logClient != nil {
		flowRels, errFlowRels := gcpCrawler.getFlowLogsRelationships()
		if errFlowRels == nil {
			allCrawledElemsAndRelationships = append(allCrawledElemsAndRelationships, flowRels...)
		}
	}

	metricTriggerElems, errMetricTriggers := gcpCrawler.getMetricTriggerElements(crawlTime)
	if errMetricTriggers != nil {
		logger.Err(errMetricTriggers).Msg("could not evaluate gcp metric rules")
	} else {
		allCrawledElemsAndRelationships = append(allCrawledElemsAndRelationships, metricTriggerElems...)
	}

	crawledData := agent.CrawledData{
		Data: allCrawledElemsAndRelationships,
	}

	gcpCrawler.outputChan <- &agent.CloudCrawlData{
		Timestamp:       crawlTime,
		DataSource:      gcpCrawler.dataSource,
		CrawledData:     crawledData,
		CrawlInternalID: gcpCrawler.dataSource.DataSourceID,
	}

	return nil, nil
}
