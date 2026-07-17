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

// InfrastructureSummary describes the stored inventory composition.
type InfrastructureSummary struct {
	Assets        []AssetTypeSummary        `json:"assets"`
	Relationships []RelationshipTypeSummary `json:"relationships"`
	DataSourceID  string                    `json:"data_source_id,omitempty"`
}

// AssetTypeSummary counts observed assets by type and status.
type AssetTypeSummary struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Count  int    `json:"count"`
}

// RelationshipTypeSummary counts observed relationships by their persisted kind.
type RelationshipTypeSummary struct {
	RelationType     int    `json:"relation_type"`
	RelationshipType string `json:"relationship_type"`
	Count            int    `json:"count"`
}

// CrawlRunSearch specifies crawl-run filters.
type CrawlRunSearch struct {
	DataSourceID string
	Limit        int
}

// CrawlRunSummary describes a completed or failed stored crawl.
type CrawlRunSummary struct {
	ID                string     `json:"id"`
	DataSourceID      string     `json:"data_source_id"`
	CrawlInternalID   string     `json:"crawl_internal_id"`
	StartedAt         time.Time  `json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at"`
	ElementCount      int        `json:"element_count"`
	RelationshipCount int        `json:"relationship_count"`
	Error             *string    `json:"error"`
}

// AssetVersionSearch specifies asset-version filters after resolving asset identity.
type AssetVersionSearch struct {
	InternalID   string
	Type         string
	DataSourceID string
	Limit        int
}

// AssetVersion is one unique raw payload observation.
type AssetVersion struct {
	Hash       string    `json:"hash"`
	CrawlRunID string    `json:"crawl_run_id"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
	RawData    []byte    `json:"-"`
	RawJSON    *string   `json:"-"`
}

// TopologySearch specifies a bounded relationship traversal.
type TopologySearch struct {
	InternalID       string
	DataSourceID     string
	RelationType     *int
	Direction        string
	MaxDepth         int
	MaxNodes         int
	MaxRelationships int
}

// PathSearch specifies a bounded shortest-path traversal.
type PathSearch struct {
	FromInternalID string
	ToInternalID   string
	DataSourceID   string
	RelationType   *int
	Direction      string
	MaxHops        int
}

// TopologyNode represents either a resolved asset or an unresolved relationship endpoint.
type TopologyNode struct {
	InternalID   string     `json:"internal_id"`
	Type         *string    `json:"type"`
	DataSourceID *string    `json:"data_source_id"`
	Name         *string    `json:"name"`
	Status       *string    `json:"status"`
	LastSeen     *time.Time `json:"last_seen"`
}

// Topology is a bounded graph traversal result.
type Topology struct {
	Root          AssetSummary   `json:"root"`
	Nodes         []TopologyNode `json:"nodes"`
	Relationships []Relationship `json:"relationships"`
	MaxDepth      int            `json:"max_depth"`
	Truncated     bool           `json:"truncated"`
}

// RelationshipPath is one bounded shortest relationship path.
type RelationshipPath struct {
	Found         bool           `json:"found"`
	Relationships []Relationship `json:"relationships"`
	Nodes         []TopologyNode `json:"nodes"`
	Truncated     bool           `json:"truncated"`
}

// QueryRepository provides read-only access to persisted crawler data.
type QueryRepository interface {
	ListDataSources(ctx context.Context) ([]DataSourceSummary, error)
	SearchAssets(ctx context.Context, search AssetSearch) ([]AssetSummary, error)
	GetAssets(ctx context.Context, internalID, elementType, dataSourceID string) ([]Asset, error)
	GetRelationships(ctx context.Context, search RelationshipSearch) ([]Relationship, error)
	GetInfrastructureSummary(ctx context.Context, dataSourceID string) (InfrastructureSummary, error)
	ListRelationshipTypes(ctx context.Context, dataSourceID string) ([]RelationshipTypeSummary, error)
	ListCrawlRuns(ctx context.Context, search CrawlRunSearch) ([]CrawlRunSummary, error)
	GetAssetVersions(ctx context.Context, search AssetVersionSearch) ([]AssetVersion, error)
	ExploreTopology(ctx context.Context, search TopologySearch) (Topology, error)
	FindRelationshipPath(ctx context.Context, search PathSearch) (RelationshipPath, error)
}
