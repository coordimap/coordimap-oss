// Package sqlstore implements relational storage repositories shared by supported SQL dialects.
package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/coordimap/agent/internal/app/ports"
	"github.com/coordimap/agent/pkg/domain/agent"
)

// Dialect controls SQL placeholder syntax.
type Dialect int

const (
	// SQLite uses question-mark placeholders.
	SQLite Dialect = iota
	// Postgres uses numbered placeholders.
	Postgres
)

// Migration is one ordered schema migration.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// NewStore creates a Store backed by db and migrations for dialect.
func NewStore(db *sql.DB, dialect Dialect, migrations []Migration) ports.Store {
	return &store{db: db, dialect: dialect, migrations: migrations}
}

type store struct {
	db         *sql.DB
	dialect    Dialect
	migrations []Migration
}

func (s *store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TIMESTAMP NOT NULL)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	for _, migration := range s.migrations {
		var applied bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = `+s.placeholder(1)+`)`, migration.Version).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %d: %w", migration.Version, err)
		}
		if applied {
			continue
		}

		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", migration.Version, err)
		}
		if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d %s: %w", migration.Version, migration.Name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, name, applied_at) VALUES (`+s.placeholders(3)+`)`, migration.Version, migration.Name, time.Now().UTC()); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", migration.Version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", migration.Version, err)
		}
	}

	return nil
}

func (s *store) WithTx(ctx context.Context, fn func(ctx context.Context, repos ports.Repositories) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if err := fn(ctx, repositories{executor: tx, dialect: s.dialect}); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (s *store) Close() error {
	return s.db.Close()
}

type executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type repositories struct {
	executor executor
	dialect  Dialect
}

func (r repositories) DataSources() ports.DataSourceRepository {
	return dataSourceRepository{repositories: r}
}
func (r repositories) CrawlRuns() ports.CrawlRunRepository {
	return crawlRunRepository{repositories: r}
}
func (r repositories) CrawledElements() ports.CrawledElementRepository {
	return crawledElementRepository{repositories: r}
}
func (r repositories) Relationships() ports.RelationshipRepository {
	return relationshipRepository{repositories: r}
}

func (r repositories) placeholder(index int) string {
	if r.dialect == Postgres {
		return fmt.Sprintf("$%d", index)
	}
	return "?"
}

func (r repositories) placeholders(count int) string {
	result := ""
	for index := 1; index <= count; index++ {
		if index > 1 {
			result += ", "
		}
		result += r.placeholder(index)
	}
	return result
}

func (s *store) placeholder(index int) string {
	return repositories{dialect: s.dialect}.placeholder(index)
}
func (s *store) placeholders(count int) string {
	return repositories{dialect: s.dialect}.placeholders(count)
}

type dataSourceRepository struct{ repositories }

func (r dataSourceRepository) Upsert(ctx context.Context, dataSource agent.DataSource, observedAt time.Time) error {
	configJSON, err := json.Marshal(dataSource.Config)
	if err != nil {
		return fmt.Errorf("marshal data source config: %w", err)
	}
	query := `INSERT INTO data_sources (id, type, name, description, config_json, first_seen, last_seen, updated_at) VALUES (` + r.placeholders(8) + `)
ON CONFLICT (id) DO UPDATE SET type = excluded.type, name = excluded.name, description = excluded.description, config_json = excluded.config_json, last_seen = excluded.last_seen, updated_at = excluded.updated_at`
	_, err = r.executor.ExecContext(ctx, query, dataSource.DataSourceID, dataSource.Info.Type, dataSource.Info.Name, dataSource.Info.Desc, string(configJSON), observedAt, observedAt, observedAt)
	if err != nil {
		return fmt.Errorf("upsert data source: %w", err)
	}
	return nil
}

type crawlRunRepository struct{ repositories }

func (r crawlRunRepository) Insert(ctx context.Context, run ports.CrawlRun) error {
	query := `INSERT INTO crawl_runs (id, data_source_id, crawl_internal_id, started_at, completed_at, element_count, relationship_count, error) VALUES (` + r.placeholders(8) + `)`
	_, err := r.executor.ExecContext(ctx, query, run.ID, run.DataSourceID, run.CrawlInternalID, run.StartedAt, run.CompletedAt, run.ElementCount, run.RelationshipCount, run.Error)
	if err != nil {
		return fmt.Errorf("insert crawl run: %w", err)
	}
	return nil
}

type crawledElementRepository struct{ repositories }

func (r crawledElementRepository) Upsert(ctx context.Context, dataSourceID string, crawlRunID string, elem *agent.Element) error {
	_ = crawlRunID
	values := elementValues(dataSourceID, elem)
	query := `INSERT INTO crawled_elements (data_source_id, internal_id, element_type, name, hash, retrieved_at, is_json_data, raw_data, raw_json, version, status, first_seen, last_seen, updated_at) VALUES (` + r.placeholders(14) + `)
ON CONFLICT (data_source_id, internal_id, element_type) DO UPDATE SET name = excluded.name, hash = excluded.hash, retrieved_at = excluded.retrieved_at, is_json_data = excluded.is_json_data, raw_data = excluded.raw_data, raw_json = excluded.raw_json, version = excluded.version, status = excluded.status, last_seen = excluded.last_seen, updated_at = excluded.updated_at`
	_, err := r.executor.ExecContext(ctx, query, values...)
	if err != nil {
		return fmt.Errorf("upsert crawled element: %w", err)
	}
	return nil
}

func (r crawledElementRepository) InsertVersion(ctx context.Context, dataSourceID string, crawlRunID string, elem *agent.Element) error {
	observedAt := elementObservedAt(elem)
	rawJSON := rawJSON(elem)
	query := `INSERT INTO crawled_element_versions (data_source_id, crawl_run_id, internal_id, element_type, name, hash, retrieved_at, is_json_data, raw_data, raw_json, version, status, observed_at) VALUES (` + r.placeholders(13) + `)
ON CONFLICT (data_source_id, internal_id, element_type, hash) DO NOTHING`
	_, err := r.executor.ExecContext(ctx, query, dataSourceID, crawlRunID, elem.ID, elem.Type, elem.Name, elem.Hash, elem.RetrievedAt, elem.IsJSONData, elem.Data, rawJSON, elem.Version, elementStatus(elem), observedAt)
	if err != nil {
		return fmt.Errorf("insert crawled element version: %w", err)
	}
	return nil
}

type relationshipRepository struct{ repositories }

func (r relationshipRepository) Upsert(ctx context.Context, dataSourceID string, crawlRunID string, elem *agent.Element, rel agent.RelationshipElement) error {
	_ = crawlRunID
	observedAt := elementObservedAt(elem)
	query := `INSERT INTO relationships (data_source_id, crawl_run_id, source_internal_id, destination_internal_id, relationship_type, relation_type, first_seen, last_seen, updated_at) VALUES (` + r.placeholders(9) + `)
ON CONFLICT (data_source_id, source_internal_id, destination_internal_id, relationship_type, relation_type) DO UPDATE SET crawl_run_id = excluded.crawl_run_id, last_seen = excluded.last_seen, updated_at = excluded.updated_at`
	_, err := r.executor.ExecContext(ctx, query, dataSourceID, crawlRunID, rel.SourceID, rel.DestinationID, rel.RelationshipType, rel.RelationType, observedAt, observedAt, observedAt)
	if err != nil {
		return fmt.Errorf("upsert relationship: %w", err)
	}
	return nil
}

func elementValues(dataSourceID string, elem *agent.Element) []any {
	observedAt := elementObservedAt(elem)
	return []any{dataSourceID, elem.ID, elem.Type, elem.Name, elem.Hash, elem.RetrievedAt, elem.IsJSONData, elem.Data, rawJSON(elem), elem.Version, elementStatus(elem), observedAt, observedAt, observedAt}
}

func elementObservedAt(elem *agent.Element) time.Time {
	if elem.RetrievedAt.IsZero() {
		return time.Now().UTC()
	}
	return elem.RetrievedAt
}

func rawJSON(elem *agent.Element) any {
	if elem.IsJSONData && json.Valid(elem.Data) {
		return string(elem.Data)
	}
	return nil
}

func elementStatus(elem *agent.Element) string {
	if elem.Status == "" {
		return agent.StatusNoStatus
	}
	return elem.Status
}
