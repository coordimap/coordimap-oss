package storage_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/coordimap/agent/internal/app/ingest"
	"github.com/coordimap/agent/internal/app/ports"
	"github.com/coordimap/agent/internal/storage/postgres"
	"github.com/coordimap/agent/internal/storage/sqlite"
	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/coordimap/agent/pkg/utils"
)

func TestQueryRepositorySQLite(t *testing.T) {
	store, err := sqlite.Open("file:coordimap_query_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	testQueryRepository(t, store)
}

func TestSQLiteMigrateLegacyCrawledElementsToAssets(t *testing.T) {
	ctx := context.Background()
	dsn := "file:coordimap_storage_upgrade_" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TIMESTAMP NOT NULL);
CREATE TABLE data_sources (id TEXT PRIMARY KEY, type TEXT NOT NULL, name TEXT NOT NULL, description TEXT NOT NULL, config_json TEXT NOT NULL, first_seen TIMESTAMP NOT NULL, last_seen TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL);
CREATE TABLE crawl_runs (id TEXT PRIMARY KEY, data_source_id TEXT NOT NULL REFERENCES data_sources(id), crawl_internal_id TEXT NOT NULL, started_at TIMESTAMP NOT NULL, completed_at TIMESTAMP NOT NULL, element_count INTEGER NOT NULL, relationship_count INTEGER NOT NULL, error TEXT);
CREATE TABLE crawled_elements (data_source_id TEXT NOT NULL REFERENCES data_sources(id), internal_id TEXT NOT NULL, element_type TEXT NOT NULL, name TEXT NOT NULL, hash TEXT NOT NULL, retrieved_at TIMESTAMP NOT NULL, is_json_data BOOLEAN NOT NULL, raw_data BLOB NOT NULL, raw_json TEXT, version TEXT NOT NULL, status TEXT NOT NULL, first_seen TIMESTAMP NOT NULL, last_seen TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, PRIMARY KEY(data_source_id, internal_id, element_type));
CREATE TABLE crawled_element_versions (id INTEGER PRIMARY KEY AUTOINCREMENT, data_source_id TEXT NOT NULL REFERENCES data_sources(id), crawl_run_id TEXT NOT NULL REFERENCES crawl_runs(id), internal_id TEXT NOT NULL, element_type TEXT NOT NULL, name TEXT NOT NULL, hash TEXT NOT NULL, retrieved_at TIMESTAMP NOT NULL, is_json_data BOOLEAN NOT NULL, raw_data BLOB NOT NULL, raw_json TEXT, version TEXT NOT NULL, status TEXT NOT NULL, observed_at TIMESTAMP NOT NULL, UNIQUE(data_source_id, internal_id, element_type, hash));`); err != nil {
		t.Fatalf("provision legacy schema: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	rawData := []byte(`{"kind":"legacy"}`)
	if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations (version, name, applied_at) VALUES (1, 'initial', ?), (2, 'read_path_indexes', ?)`, now, now); err != nil {
		t.Fatalf("record legacy migrations: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO data_sources (id, type, name, description, config_json, first_seen, last_seen, updated_at) VALUES ('legacy-source', 'test', 'legacy', '', '{}', ?, ?, ?)`, now, now, now); err != nil {
		t.Fatalf("seed legacy data source: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO crawl_runs (id, data_source_id, crawl_internal_id, started_at, completed_at, element_count, relationship_count) VALUES ('legacy-run', 'legacy-source', '', ?, ?, 1, 0)`, now, now); err != nil {
		t.Fatalf("seed legacy crawl run: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO crawled_elements (data_source_id, internal_id, element_type, name, hash, retrieved_at, is_json_data, raw_data, raw_json, version, status, first_seen, last_seen, updated_at) VALUES ('legacy-source', 'legacy-id', 'test.asset', 'legacy asset', 'legacy-hash', ?, TRUE, ?, ?, 'v1', 'Green', ?, ?, ?)`, now, rawData, string(rawData), now, now, now); err != nil {
		t.Fatalf("seed legacy asset: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO crawled_element_versions (data_source_id, crawl_run_id, internal_id, element_type, name, hash, retrieved_at, is_json_data, raw_data, raw_json, version, status, observed_at) VALUES ('legacy-source', 'legacy-run', 'legacy-id', 'test.asset', 'legacy asset', 'legacy-hash', ?, TRUE, ?, ?, 'v1', 'Green', ?)`, now, rawData, string(rawData), now); err != nil {
		t.Fatalf("seed legacy raw asset: %v", err)
	}

	store, err := sqlite.Open(dsn)
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("store.Migrate() error = %v", err)
	}

	var name, rawJSON string
	var migratedRawData []byte
	if err := db.QueryRowContext(ctx, `SELECT a.name, ra.raw_data, ra.raw_json FROM assets a JOIN raw_assets ra ON ra.data_source_id = a.data_source_id AND ra.internal_id = a.internal_id AND ra.element_type = a.element_type WHERE a.data_source_id = 'legacy-source'`).Scan(&name, &migratedRawData, &rawJSON); err != nil {
		t.Fatalf("query migrated asset: %v", err)
	}
	if name != "legacy asset" || !bytes.Equal(migratedRawData, rawData) || rawJSON != string(rawData) {
		t.Errorf("migrated asset = name %q raw %q json %q, want preserved legacy values", name, migratedRawData, rawJSON)
	}
	var hashColumnCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('assets') WHERE name = 'hash'`).Scan(&hashColumnCount); err != nil {
		t.Fatalf("inspect assets columns: %v", err)
	}
	if hashColumnCount != 0 {
		t.Error("assets still has a hash column")
	}
	for _, table := range []string{"crawled_elements", "crawled_element_versions"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			t.Fatalf("check %s existence: %v", table, err)
		}
		if count != 0 {
			t.Errorf("legacy table %s still exists", table)
		}
	}
}

func TestQueryRepositoryPostgres(t *testing.T) {
	dsn := os.Getenv("COORDIMAP_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("COORDIMAP_POSTGRES_TEST_DSN is not set")
	}
	store, err := postgres.Open(dsn)
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	testQueryRepository(t, store)
}

func testQueryRepository(t *testing.T, store ports.Store) {
	t.Helper()
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("store.Migrate() error = %v", err)
	}

	prefix := uuid.NewString()
	dataSourceA := prefix + "-source-a"
	dataSourceB := prefix + "-source-b"
	assetAID := prefix + "-asset-a"
	assetBID := prefix + "-asset-b"
	binaryID := prefix + "-binary"
	now := time.Now().UTC().Truncate(time.Microsecond)

	disconnectedID := prefix + "-disconnected"
	unresolvedID := prefix + "-unresolved"
	assetA, err := utils.CreateElement(map[string]string{"kind": "primary"}, prefix+" Alpha Asset", assetAID, "test.asset", agent.StatusGreen, "v1", now.Add(-2*time.Minute))
	if err != nil {
		t.Fatalf("CreateElement(assetA) error = %v", err)
	}
	binary := &agent.Element{
		ID:          binaryID,
		Name:        prefix + " Binary Asset",
		Type:        "test.binary",
		Hash:        "binary-hash-" + prefix,
		Data:        []byte{0, 1, 2, 3},
		RetrievedAt: now.Add(-time.Minute),
		Version:     "v1",
		Status:      agent.StatusOrange,
	}
	localRelationship, err := utils.CreateRelationship(assetAID, binaryID, "contains", agent.ParentChildTypeRelation, now.Add(-30*time.Second))
	if err != nil {
		t.Fatalf("CreateRelationship(local) error = %v", err)
	}
	crossRelationship, err := utils.CreateRelationship(assetAID, assetBID, "flows_to", agent.GenericFlowTypeRelation, now.Add(-20*time.Second))
	if err != nil {
		t.Fatalf("CreateRelationship(cross) error = %v", err)
	}
	assetB, err := utils.CreateElement(map[string]string{"kind": "remote"}, prefix+" Remote Asset", assetBID, "test.asset", agent.StatusNoStatus, "v2", now)
	if err != nil {
		t.Fatalf("CreateElement(assetB) error = %v", err)
	}
	disconnected, err := utils.CreateElement(map[string]string{"kind": "disconnected"}, prefix+" Disconnected Asset", disconnectedID, "test.asset", agent.StatusNoStatus, "v1", now)
	if err != nil {
		t.Fatalf("CreateElement(disconnected) error = %v", err)
	}
	unresolvedRelationship, err := utils.CreateRelationship(assetAID, unresolvedID, "flows_to", agent.GenericFlowTypeRelation, now.Add(-10*time.Second))
	if err != nil {
		t.Fatalf("CreateRelationship(unresolved) error = %v", err)
	}

	assetAUpdated, err := utils.CreateElement(map[string]string{"kind": "updated"}, prefix+" Alpha Asset", assetAID, "test.asset", agent.StatusGreen, "v2", now.Add(-30*time.Second))
	if err != nil {
		t.Fatalf("CreateElement(assetAUpdated) error = %v", err)
	}

	service := ingest.NewService(store)
	if err := service.StoreCrawl(ctx, crawlData(dataSourceA, now.Add(-time.Minute), assetA, binary, disconnected, localRelationship, crossRelationship, unresolvedRelationship)); err != nil {
		t.Fatalf("StoreCrawl(source A) error = %v", err)
	}
	if err := service.StoreCrawl(ctx, crawlData(dataSourceB, now, assetB)); err != nil {
		t.Fatalf("StoreCrawl(source B) error = %v", err)
	}
	if err := service.StoreCrawl(ctx, crawlData(dataSourceA, now.Add(-30*time.Second), assetAUpdated)); err != nil {
		t.Fatalf("StoreCrawl(updated source A) error = %v", err)
	}

	if err := store.WithTx(ctx, func(ctx context.Context, repos ports.Repositories) error {
		dataSources, err := repos.Query().ListDataSources(ctx)
		if err != nil {
			return err
		}
		assertDataSourceLastCrawl(t, dataSources, dataSourceA)
		assertDataSourceLastCrawl(t, dataSources, dataSourceB)

		assets, err := repos.Query().SearchAssets(ctx, ports.AssetSearch{Query: prefix, Type: "test.asset", Limit: 1})
		if err != nil {
			return err
		}
		if len(assets) != 1 || assets[0].InternalID != assetBID {
			t.Errorf("SearchAssets() = %#v, want newest asset %q", assets, assetBID)
		}

		assets, err = repos.Query().SearchAssets(ctx, ports.AssetSearch{Query: "alpha", DataSourceID: dataSourceA, Status: agent.StatusGreen, Limit: 100})
		if err != nil {
			return err
		}
		if len(assets) != 1 || assets[0].InternalID != assetAID {
			t.Errorf("filtered SearchAssets() = %#v, want %q", assets, assetAID)
		}

		jsonAssets, err := repos.Query().GetAssets(ctx, assetAID, "", "")
		if err != nil {
			return err
		}
		if len(jsonAssets) != 1 || jsonAssets[0].RawJSON == nil {
			t.Errorf("GetAssets(json) = %#v, want JSON payload", jsonAssets)
		} else if !json.Valid([]byte(*jsonAssets[0].RawJSON)) {
			t.Errorf("GetAssets(json) raw JSON = %q, want valid JSON", *jsonAssets[0].RawJSON)
		} else if jsonAssets[0].Hash != assetAUpdated.Hash || *jsonAssets[0].RawJSON != string(assetAUpdated.Data) {
			t.Errorf("GetAssets(json) = %#v, want latest raw asset %q", jsonAssets[0], assetAUpdated.Hash)
		}

		binaryAssets, err := repos.Query().GetAssets(ctx, binaryID, "", dataSourceA)
		if err != nil {
			return err
		}
		if len(binaryAssets) != 1 || binaryAssets[0].RawJSON != nil || string(binaryAssets[0].RawData) != string(binary.Data) {
			t.Errorf("GetAssets(binary) = %#v, want preserved binary payload", binaryAssets)
		}

		relationships, err := repos.Query().GetRelationships(ctx, ports.RelationshipSearch{InternalID: assetAID, Direction: "outgoing", Limit: 100})
		if err != nil {
			return err
		}
		if len(relationships) != 3 {
			t.Errorf("GetRelationships(outgoing) count = %d, want 3", len(relationships))
		}
		for _, relationship := range relationships {
			if relationship.DestinationInternalID == unresolvedID {
				if relationship.DestinationName != nil || relationship.DestinationType != nil {
					t.Errorf("GetRelationships() resolved unknown endpoint = %#v", relationship)
				}
				continue
			}
			if relationship.SourceName == nil || relationship.SourceType == nil || relationship.DestinationName == nil || relationship.DestinationType == nil {
				t.Errorf("GetRelationships() unresolved endpoint = %#v", relationship)
			}
		}

		relationType := agent.GenericFlowTypeRelation
		relationships, err = repos.Query().GetRelationships(ctx, ports.RelationshipSearch{InternalID: assetAID, Direction: "both", RelationType: &relationType, Limit: 100})
		if err != nil {
			return err
		}
		if len(relationships) != 2 {
			t.Errorf("GetRelationships(type) = %#v, want two generic-flow relationships", relationships)
		}

		summary, err := repos.Query().GetInfrastructureSummary(ctx, "")
		if err != nil {
			return err
		}
		if len(summary.Assets) != 4 || len(summary.Relationships) != 2 {
			t.Errorf("GetInfrastructureSummary() = %#v, want four asset groups and two relationship groups", summary)
		}

		relationshipTypes, err := repos.Query().ListRelationshipTypes(ctx, dataSourceA)
		if err != nil {
			return err
		}
		if len(relationshipTypes) != 2 || relationshipTypes[0].Count+relationshipTypes[1].Count != 3 {
			t.Errorf("ListRelationshipTypes() = %#v, want three observed relationships across two types", relationshipTypes)
		}

		runs, err := repos.Query().ListCrawlRuns(ctx, ports.CrawlRunSearch{DataSourceID: dataSourceA, Limit: 100})
		if err != nil {
			return err
		}
		if len(runs) != 2 || runs[0].StartedAt.Before(runs[1].StartedAt) {
			t.Errorf("ListCrawlRuns() = %#v, want newest-first source A runs", runs)
		}

		versions, err := repos.Query().GetAssetVersions(ctx, ports.AssetVersionSearch{InternalID: assetAID, Type: "test.asset", DataSourceID: dataSourceA, Limit: 25})
		if err != nil {
			return err
		}
		if len(versions) != 2 || versions[0].Hash != assetAUpdated.Hash || versions[0].RawJSON == nil {
			t.Errorf("GetAssetVersions() = %#v, want newest JSON version first", versions)
		}

		topology, err := repos.Query().ExploreTopology(ctx, ports.TopologySearch{InternalID: assetAID, Direction: "outgoing", MaxDepth: 2, MaxNodes: 100, MaxRelationships: 200})
		if err != nil {
			return err
		}
		if len(topology.Nodes) != 4 || len(topology.Relationships) != 3 || topology.Truncated {
			t.Errorf("ExploreTopology() = %#v, want complete topology including unresolved endpoint", topology)
		}
		truncatedTopology, err := repos.Query().ExploreTopology(ctx, ports.TopologySearch{InternalID: assetAID, Direction: "outgoing", MaxDepth: 2, MaxNodes: 2, MaxRelationships: 200})
		if err != nil {
			return err
		}
		if !truncatedTopology.Truncated {
			t.Errorf("ExploreTopology(max_nodes) = %#v, want truncated result", truncatedTopology)
		}

		path, err := repos.Query().FindRelationshipPath(ctx, ports.PathSearch{FromInternalID: binaryID, ToInternalID: assetBID, Direction: "both", MaxHops: 2})
		if err != nil {
			return err
		}
		if !path.Found || len(path.Relationships) != 2 || path.Relationships[0].DestinationInternalID != binaryID {
			t.Errorf("FindRelationshipPath() = %#v, want deterministic two-hop path", path)
		}
		noPath, err := repos.Query().FindRelationshipPath(ctx, ports.PathSearch{FromInternalID: disconnectedID, ToInternalID: assetBID, Direction: "both", MaxHops: 2})
		if err != nil {
			return err
		}
		if noPath.Found || noPath.Truncated {
			t.Errorf("FindRelationshipPath(disconnected) = %#v, want a complete no-path result", noPath)
		}
		boundedPath, err := repos.Query().FindRelationshipPath(ctx, ports.PathSearch{FromInternalID: binaryID, ToInternalID: assetBID, Direction: "both", MaxHops: 1})
		if err != nil {
			return err
		}
		if boundedPath.Found || !boundedPath.Truncated {
			t.Errorf("FindRelationshipPath(max_hops) = %#v, want truncated no-path result", boundedPath)
		}
		return nil
	}); err != nil {
		t.Fatalf("query transaction error = %v", err)
	}
}

func crawlData(dataSourceID string, timestamp time.Time, elements ...*agent.Element) agent.CloudCrawlData {
	return agent.CloudCrawlData{
		DataSource: agent.DataSource{
			DataSourceID: dataSourceID,
			Info:         agent.DataSourceInfo{Name: dataSourceID, Desc: dataSourceID, Type: "test"},
		},
		CrawledData: agent.CrawledData{Data: elements},
		Timestamp:   timestamp,
	}
}

func assertDataSourceLastCrawl(t *testing.T, sources []ports.DataSourceSummary, id string) {
	t.Helper()
	for _, source := range sources {
		if source.ID == id {
			if source.LastCrawlAt == nil {
				t.Errorf("ListDataSources(%q) last crawl is nil", id)
			}
			return
		}
	}
	t.Errorf("ListDataSources() missing %q", id)
}
