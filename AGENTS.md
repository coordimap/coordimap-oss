# Coordimap Local-First Agent

This document is derived from `GEMINI.md`, corrected against the current repository, and defines the approved local-first implementation roadmap. It describes future work only; it does not change the current crawler implementation.

## Project overview

Coordimap Agent is a Go infrastructure crawler that gathers inventory, configuration, and network-flow data from cloud providers, databases, Kubernetes, and flow sources.

- **Multi-source crawling:** AWS, GCP, Kubernetes, PostgreSQL, MariaDB/MySQL, MongoDB, and cloud flow logs.
- **Factory-based modular architecture:** integrations are selected through a factory so new crawlers can be added without changing existing crawler APIs.

### Build and run

- Build: `go build -o agent cmd/agent/main.go`
- Test: `go test ./...`
- Build the Docker image: `docker build -t coordimap-agent .`
- The module declares Go 1.26. `GEMINI.md`'s Go 1.23+ prerequisite is stale.

### Configuration today

YAML configuration defaults to `config.yaml`; the example is `configs/agent.example.yaml`. Current CLI flags include `--config`, `--endpoint`, and `--debug`. Values in YAML may resolve from environment variables. `build/package/agent/nfpm.yaml` is existing package metadata; it is not part of the local-first runtime architecture.

## Current code facts

- `cmd/agent/main.go` loads YAML with `configuration.NewYamlFileConfig`, validates Kubernetes scope mappings, starts one goroutine per crawler from `integrations.IntegrationsFactory`, receives `*agent.CloudCrawlData` through `sender := make(chan *agent.CloudCrawlData, 5000)`, deduplicates each batch with `dedup.CloudCrawlData`, cleans sensitive datasource configuration with `utils.CleanUpDataSource`, then POSTs `collector.AddCrawledInfraFromAgentRequest` to `--endpoint` with `gorequest`.
- `internal/integrations/types.go` defines `type Crawler interface { Crawl() }` and these integration constants: `postgres`, `aws`, `kubernetes`, `aws_flow_logs`, `mongodb`, `mariadb`, `mysql`, `gcp`, and `flows`. `README.md` documents all except the internal-only `flows` integration.
- `internal/integrations/integrations.go` maps those constants to constructors that produce crawlers writing to `chan *agent.CloudCrawlData`.
- `pkg/domain/agent/types.go` defines the shared envelope: `Element{RetrievedAt time.Time, Name string, Type string, ID string, Hash string, Data []byte, IsJSONData bool, Version string, Status string}`, `DataSource`, `CloudCrawlData`, and `RelationshipElement{SourceID string, DestinationID string, RelationshipType string, RelationType int}`.
- `pkg/utils/helpers.go` creates JSON elements with `CreateElement`, gob-encoded AWS elements with `CreateAWSElement`, and relationship wrapper elements with `CreateRelationship`. Relationship wrapper elements have `Type == agent.RelationshipType` (`"coordimap.relationship_skipinsert"`).
- `internal/graph/dedup/dedup.go` deduplicates assets by `(elem.Type, elem.ID)` and relationships by `(SourceID, DestinationID, RelationshipType, RelationType)` before remote upload.
- `cmd/collector` is a small Redis-publishing HTTP sample, not the local-storage target.
- Imports use `github.com/coordimap/agent/...`; shared domain types live under `pkg/domain/...`, and the YAML parser is `internal/config/yaml_config.go`.

## Target architecture

Keep existing crawler packages as producers. Do not rewrite AWS, GCP, Kubernetes, PostgreSQL, MySQL, MariaDB, MongoDB, or flow crawlers for storage.

1. `cmd/agent/main.go` continues to receive and deduplicate batches before any sink.
2. When `coordimap.database` is configured, the agent sanitizes the data source and persists the deduplicated batch through `internal/app/ingest.Service` in addition to the existing collector POST. When it is omitted, the agent keeps collector-only behavior.
3. Use `database/sql` for SQLite and PostgreSQL. Use existing `github.com/lib/pq` for PostgreSQL and `modernc.org/sqlite` for SQLite, preserving the CGO-free Docker build (`CGO_ENABLED=0`).
4. Use `mark3labs/mcp-go` for the MCP server: it supports Go MCP servers, stdio transport, tools, resources, and JSON-RPC handling. Stdio is the default local-agent transport. If dependency policy rejects it, implement a minimal JSON-RPC MCP transport under `internal/mcp` with the same tool names and schemas below.
5. Keep dependencies pointing inward: domain types in `pkg/domain/agent`, use cases in `internal/app`, repository interfaces in `internal/app/ports`, SQL implementations in `internal/storage/sqlite` and `internal/storage/postgres`, the storage factory in `internal/storage`, the MCP adapter in `internal/mcp`, and executable composition in `cmd/agent`.

## Storage schema

The portable local schema is inspired by `/home/ermalguni/MEGA/devops/asset-repository/migrations`, especially `01_add_tables.up.sql` (`data_source_infos`, `asset_types`, `assets`, `raw_assets`, `relation_types`, `asset_relations`, and first/last-seen timestamps). `27_raw_asset_status_and_version.up.sql` motivates `version` and `status`; use `TEXT CHECK(status IN ('NoStatus','Green','Orange','Red'))` rather than a PostgreSQL enum.

`34_add_agent_relation_types.up.sql` and `pkg/domain/agent/types.go` define seeded relation IDs: `3 parent_child`, `4 er`, `100 generic_flow`, `101 gcp_network_flow`, `102 kubernetes_retina_flow`, `103 kubernetes_istio_flow`, and `105 aws_network_flow`.

`33_diagram_spec_and_flows.up.sql` motivates `is_active`, `source_kind`, and relationship observation timestamps. Do not copy tenant columns, PostgreSQL `JSONB`, `tsrange`, exclusion constraints, triggers, or functions.

Create these portable tables:

- `data_sources(id TEXT PRIMARY KEY, type TEXT NOT NULL, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', config_json TEXT NOT NULL DEFAULT '{}', created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`.
- `asset_types(type TEXT PRIMARY KEY, name TEXT NOT NULL, created_at TIMESTAMP NOT NULL)`.
- `assets(internal_id TEXT NOT NULL, type TEXT NOT NULL, data_source_id TEXT NOT NULL REFERENCES data_sources(id), name TEXT NOT NULL, hash TEXT NOT NULL, is_json_data BOOLEAN NOT NULL, raw_data BLOB NOT NULL, raw_json TEXT, version TEXT, status TEXT NOT NULL DEFAULT 'NoStatus', first_seen TIMESTAMP NOT NULL, last_seen TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, PRIMARY KEY(internal_id, type, data_source_id))`.
- `asset_versions(id INTEGER/BIGSERIAL PRIMARY KEY, internal_id TEXT NOT NULL, type TEXT NOT NULL, data_source_id TEXT NOT NULL, hash TEXT NOT NULL, raw_data BLOB NOT NULL, raw_json TEXT, version TEXT, status TEXT NOT NULL, first_seen TIMESTAMP NOT NULL, last_seen TIMESTAMP NOT NULL, UNIQUE(internal_id, type, data_source_id, hash))`. SQLite migrations use `INTEGER PRIMARY KEY AUTOINCREMENT`; PostgreSQL migrations use `BIGSERIAL PRIMARY KEY`.
- `relation_types(id INTEGER PRIMARY KEY, type TEXT NOT NULL UNIQUE, label TEXT NOT NULL)`, seeded with the exact IDs and names above.
- `relationships(source_internal_id TEXT NOT NULL, destination_internal_id TEXT NOT NULL, relationship_type TEXT NOT NULL, relation_type_id INTEGER NOT NULL REFERENCES relation_types(id), observed_by_data_source_id TEXT NOT NULL REFERENCES data_sources(id), source_kind TEXT NOT NULL DEFAULT 'agent', is_active BOOLEAN NOT NULL DEFAULT TRUE, first_seen TIMESTAMP NOT NULL, last_seen TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, PRIMARY KEY(source_internal_id, destination_internal_id, relationship_type, relation_type_id, observed_by_data_source_id))`.
- `crawl_runs(id TEXT PRIMARY KEY, data_source_id TEXT NOT NULL REFERENCES data_sources(id), started_at TIMESTAMP NOT NULL, completed_at TIMESTAMP, element_count INTEGER NOT NULL DEFAULT 0, relationship_count INTEGER NOT NULL DEFAULT 0, error TEXT)`.

Create indexes on `assets(data_source_id, type)`, `assets(name)`, `assets(last_seen)`, `relationships(source_internal_id)`, `relationships(destination_internal_id)`, `relationships(relation_type_id)`, and `crawl_runs(data_source_id, started_at)`.

Always store `Element.Data` in `raw_data`. Set `raw_json` to `string(Element.Data)` only when `Element.IsJSONData` is true and `json.Valid(Element.Data)` is true; otherwise set `raw_json = NULL`. This preserves gob-encoded AWS payloads.

## Go package layout

The optional storage path uses these packages:

- `internal/app/ingest/service.go`: transactional orchestration accepting deduplicated, sanitized `agent.CloudCrawlData`.
- `internal/app/ports/repositories.go`: storage repository interfaces.
- `internal/storage/sqlstore/store.go`: shared `database/sql` transaction wrapper and dialect abstraction.
- `internal/storage/sqlite/store.go`: SQLite driver registration and migrations.
- `internal/storage/postgres/store.go`: PostgreSQL driver registration and migrations.
- `internal/storage/storage.go`: driver factory used by `cmd/agent`.
- `cmd/agent/main.go`: crawler startup, deduplication, optional persistence, and collector delivery.

Preserve existing `internal/integrations/*` APIs and reuse `internal/graph/dedup.CloudCrawlData`.

## Repository contracts

```go
type Store interface {
    Migrate(ctx context.Context) error
    WithTx(ctx context.Context, fn func(ctx context.Context, repos Repositories) error) error
    Close() error
}

type Repositories interface {
    DataSources() DataSourceRepository
    CrawlRuns() CrawlRunRepository
    CrawledElements() CrawledElementRepository
    Relationships() RelationshipRepository
}

type DataSourceRepository interface {
    Upsert(ctx context.Context, dataSource agent.DataSource, observedAt time.Time) error
}

type CrawlRunRepository interface {
    Insert(ctx context.Context, run CrawlRun) error
}

type CrawledElementRepository interface {
    Upsert(ctx context.Context, dataSourceID string, crawlRunID string, elem *agent.Element) error
    InsertVersion(ctx context.Context, dataSourceID string, crawlRunID string, elem *agent.Element) error
}

type RelationshipRepository interface {
    Upsert(ctx context.Context, dataSourceID string, crawlRunID string, elem *agent.Element, rel agent.RelationshipElement) error
}
```

`CrawledElements()` stores current state by `(data_source_id, internal_id, element_type)` and stores unique payload versions by hash. Relationship elements remain crawled elements; valid relationship payloads are also upserted into `relationships`. Malformed relationship JSON is retained only as a crawled element.

## Crawler ingestion flow

1. Load YAML with `configuration.NewYamlFileConfig` and `GetAllDataSources`, exactly as `cmd/agent/main.go` does now.
2. Run `validateKubernetesScopeMappings` before crawler startup.
3. Create `sender := make(chan *agent.CloudCrawlData, 5000)` and pass it to `integrations.IntegrationsFactory`, exactly as today.
4. For each received batch: reject an empty `DataSourceID`; call `dedup.CloudCrawlData`; sanitize configuration with `utils.CleanUpDataSource(&batch.DataSource, configuration.GetSkipFields())`; persist the sanitized data source first; persist every non-relationship element; persist every relationship element; then complete a `crawl_runs` row. One batch is one SQL transaction.
5. Preserve cross-source relationships by storing raw `SourceID` and `DestinationID`; do not require endpoint assets to exist. Queries may join `assets.internal_id` when endpoints are present.
6. On a per-element storage failure, fail the transaction and mark the crawl run failed. On malformed relationship payload, log and skip only that relationship because existing deduplication tolerates it.

## MCP server

Default transport is stdio. HTTP/SSE may be added later, but stdio is the required local-agent integration path.

### Tools

- `coordimap_list_data_sources`: no input; return configured/stored data-source IDs, types, names, and last crawl time.
- `coordimap_search_assets`: input `{ "query": string, "type": string optional, "data_source_id": string optional, "status": string optional, "limit": integer default 25 max 100 }`; return matching asset summaries ordered by `last_seen DESC`.
- `coordimap_get_asset`: input `{ "internal_id": string, "type": string optional, "data_source_id": string optional }`; return the selected asset with decoded JSON when `raw_json` is non-null and base64 raw bytes otherwise.
- `coordimap_get_relationships`: input `{ "internal_id": string, "direction": "incoming"|"outgoing"|"both", "relation_type_id": integer optional, "limit": integer default 50 max 200 }`; return relationships plus best-effort resolved endpoint names and types.
- `coordimap_run_crawl`: input `{ "data_source_id": string optional }`; start crawls for all sources or the matching source and return a crawl-run ID. If crawlers already run continuously, return current running state rather than start duplicate goroutines.

Expose read-only resources backed by `QueryRepository`: `coordimap://data-sources`, `coordimap://assets/{internal_id}`, and `coordimap://relationships/{internal_id}`.

## Configuration

Storage is configured in YAML under `coordimap.database`:

```yaml
coordimap:
  database:
    driver: sqlite # sqlite or postgres
    connection_string: file:coordimap.db
```

`connection_string` also accepts `${ENVIRONMENT_VARIABLE}` syntax. Omit `coordimap.database` to retain collector-only operation. Existing `--config`, `--endpoint`, `--debug`, and `COORDIMAP_CONFIG_PATH` behavior remains unchanged.

## Implementation sequence

1. Add repository interfaces and DTOs under `internal/app/ports`.
2. Add portable migrations and `database/sql` store implementations for SQLite and PostgreSQL.
3. Add `internal/app/ingest.Service` and tests using in-memory SQLite.
4. Refactor `cmd/agent/main.go` only enough to share crawler startup and validation with `cmd/coordimap-local/main.go`; do not change crawler constructors.
5. Add `cmd/coordimap-local/main.go` composition: load config, open store, migrate, start MCP stdio server, start configured crawlers, and pipe batches into the ingest service.
6. Add MCP tool handlers backed only by `QueryRepository` and the crawl runner.
7. Update `configs/agent.example.yaml` comments only if needed to document local-storage flags; do not move datasource configuration into the database in the first implementation.

## Verification

For the future implementation, use this proof sequence:

- Unit tests: `go test ./internal/graph/dedup ./internal/config ./internal/app/... ./internal/storage/... ./internal/mcp/...`.
- SQLite ingestion: create two JSON assets with `utils.CreateElement`, one relationship with `utils.CreateRelationship`, and `agent.CloudCrawlData{DataSource: agent.DataSource{DataSourceID:"test-ds", Info: agent.DataSourceInfo{Type:"test", Name:"test-ds"}}}`. Call `ingest.Service.IngestCloudCrawlData`; assert one `data_sources` row, two `assets` rows, one `relationships` row, and successful `QueryRepository.GetRelationships` retrieval.
- PostgreSQL repository: run the same repository contract suite behind `COORDIMAP_POSTGRES_TEST_DSN`; skip when it is empty.
- MCP: start the local binary with a temporary SQLite database; send JSON-RPC `initialize` and a `tools/call` for `coordimap_search_assets`; expect a valid MCP JSON response containing the ingested asset summary.
- Full local smoke test: run `go run ./cmd/coordimap-local --config <temp config> --storage-driver sqlite --database-url file:<temp>.db --mcp-transport stdio`; trigger `coordimap_run_crawl` for a mock/test datasource or fixture-backed crawler, then query `coordimap_list_data_sources` and `coordimap_search_assets` through MCP.

This document itself is documentation only. Its verification is a content review; Go tests belong to the later implementation sequence above.
