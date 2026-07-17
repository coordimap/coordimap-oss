package storage_test

import (
	"context"
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

	service := ingest.NewService(store)
	if err := service.StoreCrawl(ctx, crawlData(dataSourceA, now.Add(-time.Minute), assetA, binary, localRelationship, crossRelationship)); err != nil {
		t.Fatalf("StoreCrawl(source A) error = %v", err)
	}
	if err := service.StoreCrawl(ctx, crawlData(dataSourceB, now, assetB)); err != nil {
		t.Fatalf("StoreCrawl(source B) error = %v", err)
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
		if len(relationships) != 2 {
			t.Errorf("GetRelationships(outgoing) count = %d, want 2", len(relationships))
		}
		for _, relationship := range relationships {
			if relationship.SourceName == nil || relationship.SourceType == nil || relationship.DestinationName == nil || relationship.DestinationType == nil {
				t.Errorf("GetRelationships() unresolved endpoint = %#v", relationship)
			}
		}

		relationType := agent.GenericFlowTypeRelation
		relationships, err = repos.Query().GetRelationships(ctx, ports.RelationshipSearch{InternalID: assetAID, Direction: "both", RelationType: &relationType, Limit: 100})
		if err != nil {
			return err
		}
		if len(relationships) != 1 || relationships[0].DestinationInternalID != assetBID {
			t.Errorf("GetRelationships(type) = %#v, want cross-source relationship to %q", relationships, assetBID)
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
