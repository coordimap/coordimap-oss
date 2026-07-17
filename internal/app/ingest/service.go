// Package ingest persists deduplicated crawler output.
package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/coordimap/agent/internal/app/ports"
	"github.com/coordimap/agent/pkg/domain/agent"
)

// Service coordinates transactional storage of crawler output.
type Service struct {
	store ports.Store
}

// NewService creates a crawl ingestion service backed by store.
func NewService(store ports.Store) *Service {
	return &Service{store: store}
}

// StoreCrawl stores one deduplicated crawl batch in a single transaction.
func (s *Service) StoreCrawl(ctx context.Context, data agent.CloudCrawlData) error {
	observedAt := data.Timestamp
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}

	elementCount, relationshipCount := crawlCounts(data.CrawledData.Data)
	run := ports.CrawlRun{
		ID:                uuid.NewString(),
		DataSourceID:      data.DataSource.DataSourceID,
		CrawlInternalID:   data.CrawlInternalID,
		StartedAt:         observedAt,
		CompletedAt:       time.Now().UTC(),
		ElementCount:      elementCount,
		RelationshipCount: relationshipCount,
	}

	return s.store.WithTx(ctx, func(ctx context.Context, repos ports.Repositories) error {
		if err := repos.DataSources().Upsert(ctx, data.DataSource, observedAt); err != nil {
			return fmt.Errorf("upsert data source: %w", err)
		}
		if err := repos.CrawlRuns().Insert(ctx, run); err != nil {
			return fmt.Errorf("insert crawl run: %w", err)
		}

		for _, elem := range data.CrawledData.Data {
			if elem == nil {
				continue
			}
			if err := repos.Assets().Upsert(ctx, data.DataSource.DataSourceID, run.ID, elem); err != nil {
				return fmt.Errorf("upsert asset %q: %w", elem.ID, err)
			}
			if err := repos.Assets().UpsertRawAsset(ctx, data.DataSource.DataSourceID, run.ID, elem); err != nil {
				return fmt.Errorf("upsert raw asset %q: %w", elem.ID, err)
			}

			if elem.Type != agent.RelationshipType {
				continue
			}

			var rel agent.RelationshipElement
			if err := json.Unmarshal(elem.Data, &rel); err != nil {
				continue
			}
			if err := repos.Relationships().Upsert(ctx, data.DataSource.DataSourceID, run.ID, elem, rel); err != nil {
				return fmt.Errorf("upsert relationship %q to %q: %w", rel.SourceID, rel.DestinationID, err)
			}
		}

		return nil
	})
}

func crawlCounts(elements []*agent.Element) (int, int) {
	elementCount := 0
	relationshipCount := 0
	for _, elem := range elements {
		if elem == nil {
			continue
		}
		elementCount++
		if elem.Type != agent.RelationshipType {
			continue
		}

		var rel agent.RelationshipElement
		if json.Unmarshal(elem.Data, &rel) == nil {
			relationshipCount++
		}
	}
	return elementCount, relationshipCount
}
