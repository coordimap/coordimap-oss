package main

import (
	"context"
	"fmt"
	"time"

	"github.com/coordimap/agent/internal/app/crawl"
	"github.com/coordimap/agent/internal/app/ingest"
	configuration "github.com/coordimap/agent/internal/config"
	"github.com/coordimap/agent/internal/graph/dedup"
	"github.com/coordimap/agent/internal/storage"
	"github.com/coordimap/agent/pkg/utils"

	"github.com/parnurzeal/gorequest"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/coordimap/agent/pkg/domain/collector"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	endpoint   = kingpin.Flag("endpoint", "The server URL where to send data.").Default("http://localhost:8000/crawlers/infra/aws").OverrideDefaultFromEnvar("COORDIMAP_ENDPOINT").String()
	configFile = kingpin.Flag("config", "The config file path.").Default("config.yaml").OverrideDefaultFromEnvar("COORDIMAP_CONFIG_PATH").String()
	debug      = kingpin.Flag("debug", "Displays debug statements giving the user more information as to what is happening inside the agent.").Bool()
)

func main() {
	kingpin.Version("0.1.0")
	kingpin.Parse()

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	configuration, errConfig := configuration.NewYamlFileConfig(*configFile)
	if errConfig != nil {
		log.Error().Msg(errConfig.Error())
		return
	}
	log.Info().Msgf("Loading configuration file %s", *configFile)

	databaseConfig, errDatabaseConfig := configuration.GetDatabaseConfig()
	if errDatabaseConfig != nil {
		log.Error().Msgf("Could not load database configuration: %s", errDatabaseConfig)
		return
	}

	var ingestService *ingest.Service
	if databaseConfig != nil {
		store, errStore := storage.Open(databaseConfig.Driver, databaseConfig.ConnectionString)
		if errStore != nil {
			log.Error().Msgf("Could not open local storage: %s", errStore)
			return
		}
		if errMigrate := store.Migrate(context.Background()); errMigrate != nil {
			_ = store.Close()
			log.Error().Msgf("Could not migrate local storage: %s", errMigrate)
			return
		}
		defer func() {
			if errClose := store.Close(); errClose != nil {
				log.Error().Msgf("Could not close local storage: %s", errClose)
			}
		}()
		ingestService = ingest.NewService(store)
	}

	coordimapKey, errCoordimapKey := configuration.GetCoordimapKey()
	if errCoordimapKey != nil || coordimapKey == "" {
		log.Fatal().Msg("COORDIMAP_API_KEY is not set or is empty. Stopping the crawler.")
		return
	}

	allDataSources := configuration.GetAllDataSources()
	sender := make(chan *agent.CloudCrawlData, 5000)
	runner := crawl.NewRunner(allDataSources, sender)
	if errStart := runner.Start(); errStart != nil {
		log.Error().Msg(errStart.Error())
		return
	}

	for crawledData := range sender {
		// call the endpoint

		if crawledData.DataSource.DataSourceID == "" {
			log.Error().Msgf("Cannot push data to the cloud because no data source id was found for the data source of type: %s", crawledData.DataSource.DataSourceID)
			continue
		}

		dedupResult := dedup.CloudCrawlData(crawledData)
		crawledData = dedupResult.CloudCrawlData
		if dedupResult.DroppedAssetDuplicates > 0 || dedupResult.DroppedRelDuplicates > 0 || dedupResult.ConflictCount > 0 {
			log.Info().
				Str("DataSourceID", crawledData.DataSource.DataSourceID).
				Int("InputCount", dedupResult.InputCount).
				Int("OutputCount", dedupResult.OutputCount).
				Int("DroppedAssetDuplicates", dedupResult.DroppedAssetDuplicates).
				Int("DroppedRelationshipDuplicates", dedupResult.DroppedRelDuplicates).
				Int("ConflictCount", dedupResult.ConflictCount).
				Msg("Deduplicated crawled data before sending to collector")
		}

		sanitizedDataSource := *utils.CleanUpDataSource(&crawledData.DataSource, configuration.GetSkipFields())
		sanitizedCrawledData := *crawledData
		sanitizedCrawledData.DataSource = sanitizedDataSource
		if ingestService != nil {
			if errStore := ingestService.StoreCrawl(context.Background(), sanitizedCrawledData); errStore != nil {
				log.Error().Err(errStore).Str("DataSourceID", sanitizedDataSource.DataSourceID).Msg("Could not store crawled data locally")
			}
		}

		requestStruct := collector.AddCrawledInfraFromAgentRequest{
			CloudCrawlData: sanitizedCrawledData,
		}
		coordimapKey, errCoordimapKey := configuration.GetCoordimapKey()
		if errCoordimapKey != nil || coordimapKey == "" {
			log.Fatal().Msg("COORDIMAP_API_KEY is not set or is empty. Stopping the crawler.")
			return
		}

		var respData collector.AddCrawledInfraFromAgentResponse
		req := gorequest.New().Timeout(15 * time.Second)
		resp, _, errs := req.Post(*endpoint).Set("Api-Key", coordimapKey).SendStruct(requestStruct).EndStruct(&respData)
		if len(errs) > 0 {
			log.Info().Msgf("Error from collector %s. Error: %s", *endpoint, errs[0].Error())
			continue
		}

		if respData.Status.HTTPCode != 200 {
			log.Info().Msgf("Error from collector %s. ErrorCode: %s Error: %s", *endpoint, respData.Status.ErrorCode, respData.Status.Message)
			continue
		}

		log.Info().Msgf("Sending %d elements to the collector %s for %s", len(crawledData.CrawledData.Data), *endpoint, crawledData.DataSource.DataSourceID)

		if resp.StatusCode != 200 {
			log.Error().Msgf("Could not ship any elements to the collector for data source: %s. Response was %d", crawledData.DataSource.DataSourceID, resp.StatusCode)
			continue
		}

		if errClose := resp.Body.Close(); errClose != nil {
			log.Error().Msgf("Could not close collector response body: %s", errClose)
		}
		log.Info().
			Str("CrawlTime", time.Since(crawledData.Timestamp).String()).
			Str("DataSourceID", crawledData.DataSource.DataSourceID).
			Msgf("Successfully shipped all elements.")
	}

	fmt.Println("Goodbye!!!")
}
