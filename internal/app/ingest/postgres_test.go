package ingest_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/coordimap/agent/internal/app/ingest"
	"github.com/coordimap/agent/internal/storage/postgres"
	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/coordimap/agent/pkg/utils"
)

func TestStoreCrawlPostgres(t *testing.T) {
	dsn := os.Getenv("COORDIMAP_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("COORDIMAP_TEST_POSTGRES_DSN is not set")
	}

	ctx := context.Background()
	store, err := postgres.Open(dsn)
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("store.Migrate() error = %v", err)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`TRUNCATE raw_assets, relationships, assets, crawl_runs, data_sources CASCADE`)
		_ = db.Close()
	})
	if _, err := db.Exec(`TRUNCATE raw_assets, relationships, assets, crawl_runs, data_sources CASCADE`); err != nil {
		t.Fatalf("truncate storage tables: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	normal, err := utils.CreateElement(map[string]string{"name": "resource"}, "resource", "resource-1", "test.resource", agent.StatusNoStatus, "", now)
	if err != nil {
		t.Fatalf("CreateElement() error = %v", err)
	}
	relationship, err := utils.CreateRelationship("resource-1", "resource-2", "contains", agent.ParentChildTypeRelation, now)
	if err != nil {
		t.Fatalf("CreateRelationship() error = %v", err)
	}
	data := agent.CloudCrawlData{
		DataSource:  agent.DataSource{DataSourceID: "source-1", Info: agent.DataSourceInfo{Name: "source", Desc: "source", Type: "test"}},
		CrawledData: agent.CrawledData{Data: []*agent.Element{normal, relationship}},
		Timestamp:   now,
	}
	if err := ingest.NewService(store).StoreCrawl(ctx, data); err != nil {
		t.Fatalf("StoreCrawl() error = %v", err)
	}

	assertRowCount(t, db, "data_sources", 1)
	assertRowCount(t, db, "crawl_runs", 1)
	assertRowCount(t, db, "assets", 2)
	assertRowCount(t, db, "raw_assets", 2)
	assertRowCount(t, db, "relationships", 1)
}
