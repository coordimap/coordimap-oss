package main

import (
	"context"

	"github.com/coordimap/agent/internal/app/crawl"
	"github.com/coordimap/agent/internal/app/ingest"
	configuration "github.com/coordimap/agent/internal/config"
	"github.com/coordimap/agent/internal/graph/dedup"
	"github.com/coordimap/agent/internal/storage"
	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
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

	if databaseConfig == nil {
		log.Error().Msg("coordimap.database is required for local storage")
		return
	}

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
	ingestService := ingest.NewService(store)

	allDataSources := configuration.GetAllDataSources()
	sender := make(chan *agent.CloudCrawlData, 5000)
	runner := crawl.NewRunner(allDataSources, sender)
	if errStart := runner.Start(); errStart != nil {
		log.Error().Msg(errStart.Error())
		return
	}

	for crawledData := range sender {
		if crawledData.DataSource.DataSourceID == "" {
			log.Error().Msgf("Cannot store data because no data source id was found for data source type: %s", crawledData.DataSource.Info.Type)
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
				Msg("Deduplicated crawled data before storing locally")
		}

		if errStore := ingestService.StoreCrawl(context.Background(), *crawledData); errStore != nil {
			log.Error().Err(errStore).Str("DataSourceID", crawledData.DataSource.DataSourceID).Msg("Could not store crawled data locally")
			continue
		}

		log.Info().
			Int("ElementCount", len(crawledData.CrawledData.Data)).
			Str("DataSourceID", crawledData.DataSource.DataSourceID).
			Msg("Stored crawled data locally")
	}
}
