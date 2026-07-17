package main

import (
	"context"

	"github.com/coordimap/agent/internal/app/crawl"
	"github.com/coordimap/agent/internal/app/ingest"
	configuration "github.com/coordimap/agent/internal/config"
	"github.com/coordimap/agent/internal/graph/dedup"
	"github.com/coordimap/agent/internal/mcp"
	"github.com/coordimap/agent/internal/storage"
	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	configFile = kingpin.Flag("config", "The config file path.").Default("config.yaml").OverrideDefaultFromEnvar("COORDIMAP_CONFIG_PATH").String()
	debug      = kingpin.Flag("debug", "Displays debug statements giving the user more information as to what is happening inside the local server.").Bool()
)

func main() {
	kingpin.Version("0.1.0")
	kingpin.Parse()

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	config, err := configuration.NewYamlFileConfig(*configFile)
	if err != nil {
		log.Error().Msg(err.Error())
		return
	}
	databaseConfig, err := config.GetDatabaseConfig()
	if err != nil {
		log.Error().Msgf("Could not load database configuration: %s", err)
		return
	}
	if databaseConfig == nil {
		log.Error().Msg("coordimap.database is required for local MCP server")
		return
	}

	store, err := storage.Open(databaseConfig.Driver, databaseConfig.ConnectionString)
	if err != nil {
		log.Error().Msgf("Could not open local storage: %s", err)
		return
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Error().Msgf("Could not close local storage: %s", err)
		}
	}()
	if err := store.Migrate(context.Background()); err != nil {
		log.Error().Msgf("Could not migrate local storage: %s", err)
		return
	}

	sender := make(chan *agent.CloudCrawlData, 5000)
	runner := crawl.NewRunner(config.GetAllDataSources(), sender)
	if err := runner.Start(); err != nil {
		log.Error().Msg(err.Error())
		return
	}
	service := ingest.NewService(store)
	go storeCrawls(config, service, sender)

	if err := mcp.ServeStdio(store, runner); err != nil {
		log.Error().Msgf("MCP stdio server stopped: %s", err)
	}
}

func storeCrawls(config configuration.Config, service *ingest.Service, sender <-chan *agent.CloudCrawlData) {
	for crawledData := range sender {
		if crawledData.DataSource.DataSourceID == "" {
			log.Error().Msgf("Cannot store data because no data source id was found for data source type: %s", crawledData.DataSource.Info.Type)
			continue
		}
		dedupResult := dedup.CloudCrawlData(crawledData)
		crawledData = dedupResult.CloudCrawlData
		if dedupResult.DroppedAssetDuplicates > 0 || dedupResult.DroppedRelDuplicates > 0 || dedupResult.ConflictCount > 0 {
			log.Info().Str("DataSourceID", crawledData.DataSource.DataSourceID).Int("InputCount", dedupResult.InputCount).Int("OutputCount", dedupResult.OutputCount).Int("DroppedAssetDuplicates", dedupResult.DroppedAssetDuplicates).Int("DroppedRelationshipDuplicates", dedupResult.DroppedRelDuplicates).Int("ConflictCount", dedupResult.ConflictCount).Msg("Deduplicated crawled data before storing locally")
		}
		if err := service.StoreCrawl(context.Background(), *crawledData); err != nil {
			log.Error().Err(err).Str("DataSourceID", crawledData.DataSource.DataSourceID).Msg("Could not store crawled data locally")
		}
	}
}
