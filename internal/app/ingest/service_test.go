package ingest_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/coordimap/agent/internal/app/ingest"
	"github.com/coordimap/agent/internal/storage/sqlite"
	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/coordimap/agent/pkg/utils"
)

func TestStoreCrawlSQLite(t *testing.T) {
	ctx := context.Background()
	dsn := "file:coordimap_ingest_test?mode=memory&cache=shared"
	store, err := sqlite.Open(dsn)
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("store.Close() error = %v", err)
		}
	})
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("store.Migrate() error = %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	normal, err := utils.CreateElement(map[string]string{"name": "resource"}, "resource", "resource-1", "test.resource", agent.StatusNoStatus, "v1", now)
	if err != nil {
		t.Fatalf("CreateElement() error = %v", err)
	}
	relationship, err := utils.CreateRelationship("resource-1", "resource-2", "contains", agent.ParentChildTypeRelation, now)
	if err != nil {
		t.Fatalf("CreateRelationship() error = %v", err)
	}
	data := agent.CloudCrawlData{
		DataSource: agent.DataSource{
			DataSourceID: "source-1",
			Info: agent.DataSourceInfo{
				Name: "source",
				Desc: "source description",
				Type: "test",
			},
		},
		CrawledData:     agent.CrawledData{Data: []*agent.Element{normal, nil, relationship}},
		Timestamp:       now,
		CrawlInternalID: "crawl-1",
	}
	service := ingest.NewService(store)
	if err := service.StoreCrawl(ctx, data); err != nil {
		t.Fatalf("StoreCrawl() error = %v", err)
	}
	if err := service.StoreCrawl(ctx, data); err != nil {
		t.Fatalf("StoreCrawl() duplicate error = %v", err)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	assertRowCount(t, db, "data_sources", 1)
	assertRowCount(t, db, "crawl_runs", 2)
	assertRowCount(t, db, "crawled_elements", 2)
	assertRowCount(t, db, "crawled_element_versions", 2)
	assertRowCount(t, db, "relationships", 1)
}

func TestStoreCrawlInvalidRelationshipJSONStoresElementOnly(t *testing.T) {
	ctx := context.Background()
	dsn := "file:coordimap_invalid_relationship_test?mode=memory&cache=shared"
	store, err := sqlite.Open(dsn)
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("store.Migrate() error = %v", err)
	}

	service := ingest.NewService(store)
	if err := service.StoreCrawl(ctx, agent.CloudCrawlData{
		DataSource: agent.DataSource{DataSourceID: "source-1", Info: agent.DataSourceInfo{Name: "source", Desc: "source", Type: "test"}},
		CrawledData: agent.CrawledData{Data: []*agent.Element{{
			ID:          "invalid-relationship",
			Name:        "invalid-relationship",
			Type:        agent.RelationshipType,
			Hash:        "invalid-hash",
			Data:        []byte("not-json"),
			RetrievedAt: time.Now().UTC(),
		}}},
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("StoreCrawl() error = %v", err)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	assertRowCount(t, db, "crawled_elements", 1)
	assertRowCount(t, db, "crawled_element_versions", 1)
	assertRowCount(t, db, "relationships", 0)
}

func assertRowCount(t *testing.T, db *sql.DB, table string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Errorf("%s count = %d, want %d", table, got, want)
	}
}
