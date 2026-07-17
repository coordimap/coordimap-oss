// Package ports defines storage contracts used by crawl ingestion.
package ports

import (
	"context"
	"time"

	"github.com/coordimap/agent/pkg/domain/agent"
)

// Store manages database lifecycle and transactional repository access.
type Store interface {
	Migrate(ctx context.Context) error
	WithTx(ctx context.Context, fn func(ctx context.Context, repos Repositories) error) error
	Close() error
}

// Repositories provides repositories bound to the active transaction.
type Repositories interface {
	DataSources() DataSourceRepository
	CrawlRuns() CrawlRunRepository
	CrawledElements() CrawledElementRepository
	Relationships() RelationshipRepository
}

// DataSourceRepository persists current data-source state.
type DataSourceRepository interface {
	Upsert(ctx context.Context, dataSource agent.DataSource, observedAt time.Time) error
}

// CrawlRun records one complete crawler result batch.
type CrawlRun struct {
	ID                string
	DataSourceID      string
	CrawlInternalID   string
	StartedAt         time.Time
	CompletedAt       time.Time
	ElementCount      int
	RelationshipCount int
	Error             *string
}

// CrawlRunRepository persists crawl-run history.
type CrawlRunRepository interface {
	Insert(ctx context.Context, run CrawlRun) error
}

// CrawledElementRepository persists current state and immutable versions.
type CrawledElementRepository interface {
	Upsert(ctx context.Context, dataSourceID string, crawlRunID string, elem *agent.Element) error
	InsertVersion(ctx context.Context, dataSourceID string, crawlRunID string, elem *agent.Element) error
}

// RelationshipRepository persists current relationship state.
type RelationshipRepository interface {
	Upsert(ctx context.Context, dataSourceID string, crawlRunID string, elem *agent.Element, rel agent.RelationshipElement) error
}
