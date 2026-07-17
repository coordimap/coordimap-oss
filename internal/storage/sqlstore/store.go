// Package sqlstore implements relational storage repositories shared by supported SQL dialects.
package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/coordimap/agent/internal/app/ports"
	"github.com/coordimap/agent/pkg/domain/agent"
	"strings"
	"time"
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
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
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

func (r repositories) Query() ports.QueryRepository {
	return queryRepository{repositories: r}
}
func (r repositories) Assets() ports.AssetRepository {
	return assetRepository{repositories: r}
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

type assetRepository struct{ repositories }

func (r assetRepository) Upsert(ctx context.Context, dataSourceID string, crawlRunID string, elem *agent.Element) error {
	_ = crawlRunID
	values := assetValues(dataSourceID, elem)
	query := `INSERT INTO assets (data_source_id, internal_id, element_type, name, retrieved_at, is_json_data, version, status, first_seen, last_seen, updated_at) VALUES (` + r.placeholders(11) + `)
ON CONFLICT (data_source_id, internal_id, element_type) DO UPDATE SET name = excluded.name, retrieved_at = excluded.retrieved_at, is_json_data = excluded.is_json_data, version = excluded.version, status = excluded.status, last_seen = excluded.last_seen, updated_at = excluded.updated_at`
	_, err := r.executor.ExecContext(ctx, query, values...)
	if err != nil {
		return fmt.Errorf("upsert asset: %w", err)
	}
	return nil
}

func (r assetRepository) UpsertRawAsset(ctx context.Context, dataSourceID string, crawlRunID string, elem *agent.Element) error {
	observedAt := elementObservedAt(elem)
	query := `INSERT INTO raw_assets (data_source_id, internal_id, element_type, hash, crawl_run_id, raw_data, raw_json, first_seen, last_seen, updated_at) VALUES (` + r.placeholders(10) + `)
ON CONFLICT (data_source_id, internal_id, element_type, hash) DO UPDATE SET last_seen = excluded.last_seen, updated_at = excluded.updated_at`
	_, err := r.executor.ExecContext(ctx, query, dataSourceID, elem.ID, elem.Type, elem.Hash, crawlRunID, elem.Data, rawJSON(elem), observedAt, observedAt, observedAt)
	if err != nil {
		return fmt.Errorf("upsert raw asset: %w", err)
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

func assetValues(dataSourceID string, elem *agent.Element) []any {
	observedAt := elementObservedAt(elem)
	return []any{dataSourceID, elem.ID, elem.Type, elem.Name, elem.RetrievedAt, elem.IsJSONData, elem.Version, elementStatus(elem), observedAt, observedAt, observedAt}
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

type queryRepository struct{ repositories }

func (r queryRepository) ListDataSources(ctx context.Context) ([]ports.DataSourceSummary, error) {
	rows, err := r.executor.QueryContext(ctx, `SELECT d.id, d.type, d.name, MAX(c.completed_at)
FROM data_sources d
LEFT JOIN crawl_runs c ON c.data_source_id = d.id
GROUP BY d.id, d.type, d.name
ORDER BY d.name, d.id`)
	if err != nil {
		return nil, fmt.Errorf("list data sources: %w", err)
	}
	defer rows.Close()

	var summaries []ports.DataSourceSummary
	for rows.Next() {
		var summary ports.DataSourceSummary
		var lastCrawl nullableTime
		if err := rows.Scan(&summary.ID, &summary.Type, &summary.Name, &lastCrawl); err != nil {
			return nil, fmt.Errorf("scan data source: %w", err)
		}
		if lastCrawl.Valid {
			value := lastCrawl.Time
			summary.LastCrawlAt = &value
		}
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate data sources: %w", err)
	}
	return summaries, nil
}

func (r queryRepository) SearchAssets(ctx context.Context, search ports.AssetSearch) ([]ports.AssetSummary, error) {
	where := []string{"LOWER(name) LIKE LOWER(" + r.placeholder(1) + ")"}
	args := []any{"%" + search.Query + "%"}
	if search.Type != "" {
		where = append(where, "element_type = "+r.placeholder(len(args)+1))
		args = append(args, search.Type)
	}
	if search.DataSourceID != "" {
		where = append(where, "data_source_id = "+r.placeholder(len(args)+1))
		args = append(args, search.DataSourceID)
	}
	if search.Status != "" {
		where = append(where, "status = "+r.placeholder(len(args)+1))
		args = append(args, search.Status)
	}
	args = append(args, boundedLimit(search.Limit, 25, 100))
	query := `SELECT internal_id, element_type, data_source_id, name, status, last_seen
FROM assets
WHERE ` + strings.Join(where, " AND ") + `
ORDER BY last_seen DESC
LIMIT ` + r.placeholder(len(args))
	rows, err := r.executor.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search assets: %w", err)
	}
	defer rows.Close()

	var assets []ports.AssetSummary
	for rows.Next() {
		var asset ports.AssetSummary
		var lastSeen nullableTime
		if err := rows.Scan(&asset.InternalID, &asset.Type, &asset.DataSourceID, &asset.Name, &asset.Status, &lastSeen); err != nil {
			return nil, fmt.Errorf("scan asset summary: %w", err)
		}
		asset.LastSeen = lastSeen.Time
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate asset summaries: %w", err)
	}
	return assets, nil
}

func (r queryRepository) GetAssets(ctx context.Context, internalID, elementType, dataSourceID string) ([]ports.Asset, error) {
	where := []string{"a.internal_id = " + r.placeholder(1)}
	args := []any{internalID}
	if elementType != "" {
		where = append(where, "a.element_type = "+r.placeholder(len(args)+1))
		args = append(args, elementType)
	}
	if dataSourceID != "" {
		where = append(where, "a.data_source_id = "+r.placeholder(len(args)+1))
		args = append(args, dataSourceID)
	}
	query := `WITH latest_raw_assets AS (
	SELECT data_source_id, internal_id, element_type, hash, raw_data, raw_json,
		ROW_NUMBER() OVER (
			PARTITION BY data_source_id, internal_id, element_type
			ORDER BY last_seen DESC, updated_at DESC, hash DESC
		) AS row_number
	FROM raw_assets
)
SELECT a.internal_id, a.element_type, a.data_source_id, a.name, a.status, a.last_seen, ra.hash, a.is_json_data, ra.raw_data, ra.raw_json, a.version, a.retrieved_at, a.first_seen
FROM assets a
JOIN latest_raw_assets ra ON ra.data_source_id = a.data_source_id AND ra.internal_id = a.internal_id AND ra.element_type = a.element_type AND ra.row_number = 1
WHERE ` + strings.Join(where, " AND ") + `
ORDER BY a.last_seen DESC`
	rows, err := r.executor.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get assets: %w", err)
	}
	defer rows.Close()

	var assets []ports.Asset
	for rows.Next() {
		var asset ports.Asset
		var rawJSON sql.NullString
		var lastSeen, retrievedAt, firstSeen nullableTime
		if err := rows.Scan(&asset.InternalID, &asset.Type, &asset.DataSourceID, &asset.Name, &asset.Status, &lastSeen, &asset.Hash, &asset.IsJSONData, &asset.RawData, &rawJSON, &asset.Version, &retrievedAt, &firstSeen); err != nil {
			return nil, fmt.Errorf("scan asset: %w", err)
		}
		asset.LastSeen = lastSeen.Time
		asset.RetrievedAt = retrievedAt.Time
		asset.FirstSeen = firstSeen.Time
		if rawJSON.Valid {
			value := rawJSON.String
			asset.RawJSON = &value
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate assets: %w", err)
	}
	return assets, nil
}

func (r queryRepository) GetRelationships(ctx context.Context, search ports.RelationshipSearch) ([]ports.Relationship, error) {
	where := make([]string, 0, 2)
	args := make([]any, 0, 3)
	switch search.Direction {
	case "", "both":
		where = append(where, "(source_internal_id = "+r.placeholder(1)+" OR destination_internal_id = "+r.placeholder(2)+")")
		args = append(args, search.InternalID, search.InternalID)
	case "incoming":
		where = append(where, "destination_internal_id = "+r.placeholder(1))
		args = append(args, search.InternalID)
	case "outgoing":
		where = append(where, "source_internal_id = "+r.placeholder(1))
		args = append(args, search.InternalID)
	default:
		return nil, fmt.Errorf("invalid relationship direction %q", search.Direction)
	}
	if search.RelationType != nil {
		where = append(where, "relation_type = "+r.placeholder(len(args)+1))
		args = append(args, *search.RelationType)
	}
	args = append(args, boundedLimit(search.Limit, 50, 200))
	query := `SELECT r.data_source_id, r.source_internal_id, r.destination_internal_id, r.relationship_type, r.relation_type,
	(SELECT a.name FROM assets a WHERE a.internal_id = r.source_internal_id ORDER BY a.last_seen DESC LIMIT 1),
	(SELECT a.element_type FROM assets a WHERE a.internal_id = r.source_internal_id ORDER BY a.last_seen DESC LIMIT 1),
	(SELECT a.name FROM assets a WHERE a.internal_id = r.destination_internal_id ORDER BY a.last_seen DESC LIMIT 1),
	(SELECT a.element_type FROM assets a WHERE a.internal_id = r.destination_internal_id ORDER BY a.last_seen DESC LIMIT 1),
	r.first_seen, r.last_seen
FROM relationships r
WHERE ` + strings.Join(where, " AND ") + `
ORDER BY r.last_seen DESC
LIMIT ` + r.placeholder(len(args))
	rows, err := r.executor.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get relationships: %w", err)
	}
	defer rows.Close()

	var relationships []ports.Relationship
	for rows.Next() {
		var relationship ports.Relationship
		var sourceName, sourceType, destinationName, destinationType sql.NullString
		var firstSeen, lastSeen nullableTime
		if err := rows.Scan(&relationship.DataSourceID, &relationship.SourceInternalID, &relationship.DestinationInternalID, &relationship.RelationshipType, &relationship.RelationType, &sourceName, &sourceType, &destinationName, &destinationType, &firstSeen, &lastSeen); err != nil {
			return nil, fmt.Errorf("scan relationship: %w", err)
		}
		relationship.SourceName = stringPointer(sourceName)
		relationship.SourceType = stringPointer(sourceType)
		relationship.DestinationName = stringPointer(destinationName)
		relationship.DestinationType = stringPointer(destinationType)
		relationship.FirstSeen = firstSeen.Time
		relationship.LastSeen = lastSeen.Time
		relationships = append(relationships, relationship)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate relationships: %w", err)
	}
	return relationships, nil
}

func boundedLimit(limit, defaultLimit, maxLimit int) int {
	if limit < 1 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

func stringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

type nullableTime struct {
	Time  time.Time
	Valid bool
}

func (value *nullableTime) Scan(source any) error {
	if source == nil {
		value.Time = time.Time{}
		value.Valid = false
		return nil
	}
	if timestamp, ok := source.(time.Time); ok {
		value.Time = timestamp
		value.Valid = true
		return nil
	}
	var text string
	switch source := source.(type) {
	case string:
		text = source
	case []byte:
		text = string(source)
	default:
		return fmt.Errorf("unsupported timestamp type %T", source)
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999 -0700 MST", "2006-01-02 15:04:05.999999999Z07:00"} {
		timestamp, err := time.Parse(layout, text)
		if err == nil {
			value.Time = timestamp
			value.Valid = true
			return nil
		}
	}
	return fmt.Errorf("parse timestamp %q", text)
}
