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
	Query() QueryRepository
	Assets() AssetRepository
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

// AssetRepository persists current asset state and raw payload history.
type AssetRepository interface {
	Upsert(ctx context.Context, dataSourceID string, crawlRunID string, elem *agent.Element) error
	UpsertRawAsset(ctx context.Context, dataSourceID string, crawlRunID string, elem *agent.Element) error
}

// RelationshipRepository persists current relationship state.
type RelationshipRepository interface {
	Upsert(ctx context.Context, dataSourceID string, crawlRunID string, elem *agent.Element, rel agent.RelationshipElement) error
}

// DataSourceSummary describes a persisted data source and its latest completed crawl.
type DataSourceSummary struct {
	ID          string
	Type        string
	Name        string
	LastCrawlAt *time.Time
}

// AssetSearch specifies optional filters for asset summaries.
type AssetSearch struct {
	Query        string
	Type         string
	DataSourceID string
	Status       string
	Limit        int
}

// AssetSummary describes an asset without its payload.
type AssetSummary struct {
	InternalID   string
	Type         string
	DataSourceID string
	Name         string
	Status       string
	LastSeen     time.Time
}

// Asset includes asset metadata and its payload from raw_assets.
type Asset struct {
	AssetSummary
	// Hash identifies the selected raw asset payload.
	Hash        string
	IsJSONData  bool
	RawData     []byte
	RawJSON     *string
	Version     string
	RetrievedAt time.Time
	FirstSeen   time.Time
	LastSeen    time.Time
}

// RelationshipSearch specifies relationship filters.
type RelationshipSearch struct {
	InternalID   string
	Direction    string
	RelationType *int
	Limit        int
}

// Relationship describes a persisted edge with best-effort endpoint metadata.
type Relationship struct {
	DataSourceID          string
	SourceInternalID      string
	DestinationInternalID string
	RelationshipType      string
	RelationType          int
	SourceName            *string
	SourceType            *string
	DestinationName       *string
	DestinationType       *string
	FirstSeen             time.Time
	LastSeen              time.Time
}

// QueryRepository provides read-only access to persisted crawler data.
type QueryRepository interface {
	ListDataSources(ctx context.Context) ([]DataSourceSummary, error)
	SearchAssets(ctx context.Context, search AssetSearch) ([]AssetSummary, error)
	GetAssets(ctx context.Context, internalID, elementType, dataSourceID string) ([]Asset, error)
	GetRelationships(ctx context.Context, search RelationshipSearch) ([]Relationship, error)
}
