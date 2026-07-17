// Package sqlstore implements relational storage repositories shared by supported SQL dialects.
package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/coordimap/agent/internal/app/ports"
	"github.com/coordimap/agent/pkg/domain/agent"
	"sort"
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

func (r queryRepository) GetInfrastructureSummary(ctx context.Context, dataSourceID string) (ports.InfrastructureSummary, error) {
	summary := ports.InfrastructureSummary{DataSourceID: dataSourceID}
	assetWhere, assetArgs := optionalDataSourceFilter(r, dataSourceID, "data_source_id")
	assetRows, err := r.executor.QueryContext(ctx, `SELECT element_type, status, COUNT(*) FROM assets`+assetWhere+` GROUP BY element_type, status ORDER BY element_type, status`, assetArgs...)
	if err != nil {
		return summary, fmt.Errorf("get infrastructure asset summary: %w", err)
	}
	defer assetRows.Close()
	for assetRows.Next() {
		var item ports.AssetTypeSummary
		if err := assetRows.Scan(&item.Type, &item.Status, &item.Count); err != nil {
			return summary, fmt.Errorf("scan infrastructure asset summary: %w", err)
		}
		summary.Assets = append(summary.Assets, item)
	}
	if err := assetRows.Err(); err != nil {
		return summary, fmt.Errorf("iterate infrastructure asset summary: %w", err)
	}
	relationshipWhere, relationshipArgs := optionalDataSourceFilter(r, dataSourceID, "data_source_id")
	relationshipRows, err := r.executor.QueryContext(ctx, `SELECT relation_type, relationship_type, COUNT(*) FROM relationships`+relationshipWhere+` GROUP BY relation_type, relationship_type ORDER BY relation_type, relationship_type`, relationshipArgs...)
	if err != nil {
		return summary, fmt.Errorf("get infrastructure relationship summary: %w", err)
	}
	defer relationshipRows.Close()
	for relationshipRows.Next() {
		var item ports.RelationshipTypeSummary
		if err := relationshipRows.Scan(&item.RelationType, &item.RelationshipType, &item.Count); err != nil {
			return summary, fmt.Errorf("scan infrastructure relationship summary: %w", err)
		}
		summary.Relationships = append(summary.Relationships, item)
	}
	if err := relationshipRows.Err(); err != nil {
		return summary, fmt.Errorf("iterate infrastructure relationship summary: %w", err)
	}
	return summary, nil
}

func (r queryRepository) ListRelationshipTypes(ctx context.Context, dataSourceID string) ([]ports.RelationshipTypeSummary, error) {
	where, args := optionalDataSourceFilter(r, dataSourceID, "data_source_id")
	rows, err := r.executor.QueryContext(ctx, `SELECT relation_type, relationship_type, COUNT(*) FROM relationships`+where+` GROUP BY relation_type, relationship_type ORDER BY COUNT(*) DESC, relation_type, relationship_type`, args...)
	if err != nil {
		return nil, fmt.Errorf("list relationship types: %w", err)
	}
	defer rows.Close()
	var result []ports.RelationshipTypeSummary
	for rows.Next() {
		var item ports.RelationshipTypeSummary
		if err := rows.Scan(&item.RelationType, &item.RelationshipType, &item.Count); err != nil {
			return nil, fmt.Errorf("scan relationship type: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate relationship types: %w", err)
	}
	return result, nil
}

func (r queryRepository) ListCrawlRuns(ctx context.Context, search ports.CrawlRunSearch) ([]ports.CrawlRunSummary, error) {
	where, args := optionalDataSourceFilter(r, search.DataSourceID, "data_source_id")
	args = append(args, boundedLimit(search.Limit, 25, 100))
	rows, err := r.executor.QueryContext(ctx, `SELECT id, data_source_id, crawl_internal_id, started_at, completed_at, element_count, relationship_count, error FROM crawl_runs`+where+` ORDER BY started_at DESC, id DESC LIMIT `+r.placeholder(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("list crawl runs: %w", err)
	}
	defer rows.Close()
	var result []ports.CrawlRunSummary
	for rows.Next() {
		var item ports.CrawlRunSummary
		var completedAt nullableTime
		var crawlError sql.NullString
		if err := rows.Scan(&item.ID, &item.DataSourceID, &item.CrawlInternalID, &item.StartedAt, &completedAt, &item.ElementCount, &item.RelationshipCount, &crawlError); err != nil {
			return nil, fmt.Errorf("scan crawl run: %w", err)
		}
		if completedAt.Valid {
			value := completedAt.Time
			item.CompletedAt = &value
		}
		item.Error = stringPointer(crawlError)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate crawl runs: %w", err)
	}
	return result, nil
}

func (r queryRepository) GetAssetVersions(ctx context.Context, search ports.AssetVersionSearch) ([]ports.AssetVersion, error) {
	where := []string{"internal_id = " + r.placeholder(1)}
	args := []any{search.InternalID}
	if search.Type != "" {
		where = append(where, "element_type = "+r.placeholder(len(args)+1))
		args = append(args, search.Type)
	}
	if search.DataSourceID != "" {
		where = append(where, "data_source_id = "+r.placeholder(len(args)+1))
		args = append(args, search.DataSourceID)
	}
	args = append(args, boundedLimit(search.Limit, 10, 25))
	rows, err := r.executor.QueryContext(ctx, `SELECT hash, crawl_run_id, first_seen, last_seen, raw_data, raw_json FROM raw_assets WHERE `+strings.Join(where, " AND ")+` ORDER BY last_seen DESC, updated_at DESC, hash DESC LIMIT `+r.placeholder(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("get asset versions: %w", err)
	}
	defer rows.Close()
	var versions []ports.AssetVersion
	for rows.Next() {
		var version ports.AssetVersion
		var rawJSON sql.NullString
		if err := rows.Scan(&version.Hash, &version.CrawlRunID, &version.FirstSeen, &version.LastSeen, &version.RawData, &rawJSON); err != nil {
			return nil, fmt.Errorf("scan asset version: %w", err)
		}
		version.RawJSON = stringPointer(rawJSON)
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate asset versions: %w", err)
	}
	return versions, nil
}

func (r queryRepository) ExploreTopology(ctx context.Context, search ports.TopologySearch) (ports.Topology, error) {
	root, err := r.latestAssetSummary(ctx, search.InternalID, search.DataSourceID)
	if err != nil {
		return ports.Topology{}, err
	}
	result := ports.Topology{Root: root, MaxDepth: search.MaxDepth}
	frontier := []string{search.InternalID}
	seenNodes := map[string]bool{search.InternalID: true}
	seenEdges := make(map[string]bool)
	for depth := 0; len(frontier) > 0 && depth < search.MaxDepth; depth++ {
		edges, err := r.frontierRelationships(ctx, frontier, search.DataSourceID, search.RelationType, search.Direction)
		if err != nil {
			return ports.Topology{}, err
		}
		next := make([]string, 0)
		for _, edge := range edges {
			key := relationshipKey(edge)
			if seenEdges[key] {
				continue
			}
			newNodes := 0
			if !seenNodes[edge.SourceInternalID] {
				newNodes++
			}
			if !seenNodes[edge.DestinationInternalID] {
				newNodes++
			}
			if len(seenNodes)+newNodes > search.MaxNodes {
				result.Truncated = true
				continue
			}
			if len(result.Relationships) >= search.MaxRelationships {
				result.Truncated = true
				continue
			}
			seenEdges[key] = true
			result.Relationships = append(result.Relationships, edge)
			for _, id := range []string{edge.SourceInternalID, edge.DestinationInternalID} {
				if !seenNodes[id] {
					seenNodes[id] = true
					next = append(next, id)
				}
			}
		}
		frontier = next
	}
	if len(frontier) > 0 {
		result.Truncated = true
	}
	nodes, err := r.topologyNodes(ctx, seenNodes)
	if err != nil {
		return ports.Topology{}, err
	}
	result.Nodes = nodes
	return result, nil
}

func (r queryRepository) FindRelationshipPath(ctx context.Context, search ports.PathSearch) (ports.RelationshipPath, error) {
	if _, err := r.latestAssetSummary(ctx, search.FromInternalID, search.DataSourceID); err != nil {
		return ports.RelationshipPath{}, err
	}
	if _, err := r.latestAssetSummary(ctx, search.ToInternalID, search.DataSourceID); err != nil {
		return ports.RelationshipPath{}, err
	}
	if search.FromInternalID == search.ToInternalID {
		nodes, err := r.topologyNodes(ctx, map[string]bool{search.FromInternalID: true})
		return ports.RelationshipPath{Found: true, Nodes: nodes}, err
	}
	frontier := []string{search.FromInternalID}
	seen := map[string]bool{search.FromInternalID: true}
	previous := make(map[string]ports.Relationship)
	parent := make(map[string]string)
	for hops := 0; len(frontier) > 0 && hops < search.MaxHops; hops++ {
		edges, err := r.frontierRelationships(ctx, frontier, search.DataSourceID, search.RelationType, search.Direction)
		if err != nil {
			return ports.RelationshipPath{}, err
		}
		next := make([]string, 0)
		for _, edge := range edges {
			for _, neighbor := range relationshipNeighbors(edge, frontier) {
				if seen[neighbor] {
					continue
				}
				seen[neighbor] = true
				previous[neighbor] = edge
				parent[neighbor] = oppositeEndpoint(edge, neighbor)
				if neighbor == search.ToInternalID {
					return r.buildPath(ctx, search.FromInternalID, neighbor, parent, previous)
				}
				next = append(next, neighbor)
			}
		}
		frontier = next
	}
	nodes, err := r.topologyNodes(ctx, seen)
	if err != nil {
		return ports.RelationshipPath{}, err
	}
	return ports.RelationshipPath{Nodes: nodes, Truncated: len(frontier) > 0}, nil
}

func optionalDataSourceFilter(r queryRepository, dataSourceID, column string) (string, []any) {
	if dataSourceID == "" {
		return "", nil
	}
	return " WHERE " + column + " = " + r.placeholder(1), []any{dataSourceID}
}

func (r queryRepository) latestAssetSummary(ctx context.Context, internalID, dataSourceID string) (ports.AssetSummary, error) {
	where := "internal_id = " + r.placeholder(1)
	args := []any{internalID}
	if dataSourceID != "" {
		where += " AND data_source_id = " + r.placeholder(2)
		args = append(args, dataSourceID)
	}
	var asset ports.AssetSummary
	var lastSeen nullableTime
	err := r.executor.QueryRowContext(ctx, `SELECT internal_id, element_type, data_source_id, name, status, last_seen FROM assets WHERE `+where+` ORDER BY last_seen DESC, data_source_id, element_type LIMIT 1`, args...).Scan(&asset.InternalID, &asset.Type, &asset.DataSourceID, &asset.Name, &asset.Status, &lastSeen)
	if err != nil {
		return ports.AssetSummary{}, err
	}
	asset.LastSeen = lastSeen.Time
	return asset, nil
}

func (r queryRepository) frontierRelationships(ctx context.Context, frontier []string, dataSourceID string, relationType *int, direction string) ([]ports.Relationship, error) {
	if len(frontier) == 0 {
		return nil, nil
	}
	if direction == "" {
		direction = "both"
	}
	where := make([]string, 0, 3)
	args := make([]any, 0, len(frontier)*2+2)
	appendIDs := func(column string) string {
		placeholders := make([]string, len(frontier))
		for index, id := range frontier {
			args = append(args, id)
			placeholders[index] = r.placeholder(len(args))
		}
		return column + " IN (" + strings.Join(placeholders, ", ") + ")"
	}
	switch direction {
	case "incoming":
		where = append(where, appendIDs("r.destination_internal_id"))
	case "outgoing":
		where = append(where, appendIDs("r.source_internal_id"))
	case "both":
		where = append(where, "("+appendIDs("r.source_internal_id")+" OR "+appendIDs("r.destination_internal_id")+")")
	default:
		return nil, fmt.Errorf("invalid relationship direction %q", direction)
	}
	if dataSourceID != "" {
		args = append(args, dataSourceID)
		where = append(where, "r.data_source_id = "+r.placeholder(len(args)))
	}
	if relationType != nil {
		args = append(args, *relationType)
		where = append(where, "r.relation_type = "+r.placeholder(len(args)))
	}
	rows, err := r.executor.QueryContext(ctx, `SELECT r.data_source_id, r.source_internal_id, r.destination_internal_id, r.relationship_type, r.relation_type,
	(SELECT a.name FROM assets a WHERE a.internal_id = r.source_internal_id ORDER BY a.last_seen DESC LIMIT 1),
	(SELECT a.element_type FROM assets a WHERE a.internal_id = r.source_internal_id ORDER BY a.last_seen DESC LIMIT 1),
	(SELECT a.name FROM assets a WHERE a.internal_id = r.destination_internal_id ORDER BY a.last_seen DESC LIMIT 1),
	(SELECT a.element_type FROM assets a WHERE a.internal_id = r.destination_internal_id ORDER BY a.last_seen DESC LIMIT 1),
	r.first_seen, r.last_seen
FROM relationships r WHERE `+strings.Join(where, " AND ")+`
ORDER BY r.last_seen DESC, r.data_source_id, r.source_internal_id, r.destination_internal_id, r.relationship_type, r.relation_type`, args...)
	if err != nil {
		return nil, fmt.Errorf("query frontier relationships: %w", err)
	}
	defer rows.Close()
	var relationships []ports.Relationship
	for rows.Next() {
		var relationship ports.Relationship
		var sourceName, sourceType, destinationName, destinationType sql.NullString
		var firstSeen, lastSeen nullableTime
		if err := rows.Scan(&relationship.DataSourceID, &relationship.SourceInternalID, &relationship.DestinationInternalID, &relationship.RelationshipType, &relationship.RelationType, &sourceName, &sourceType, &destinationName, &destinationType, &firstSeen, &lastSeen); err != nil {
			return nil, fmt.Errorf("scan frontier relationship: %w", err)
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
		return nil, fmt.Errorf("iterate frontier relationships: %w", err)
	}
	return relationships, nil
}

func (r queryRepository) topologyNodes(ctx context.Context, ids map[string]bool) ([]ports.TopologyNode, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	orderedIDs := make([]string, 0, len(ids))
	for id := range ids {
		orderedIDs = append(orderedIDs, id)
	}
	sort.Strings(orderedIDs)
	args := make([]any, len(orderedIDs))
	for index, id := range orderedIDs {
		args[index] = id
	}
	rows, err := r.executor.QueryContext(ctx, `WITH ranked_assets AS (
	SELECT internal_id, element_type, data_source_id, name, status, last_seen,
	ROW_NUMBER() OVER (PARTITION BY internal_id ORDER BY last_seen DESC, data_source_id, element_type) AS row_number
	FROM assets WHERE internal_id IN (`+r.placeholders(len(args))+`)
) SELECT internal_id, element_type, data_source_id, name, status, last_seen FROM ranked_assets WHERE row_number = 1 ORDER BY internal_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("get topology nodes: %w", err)
	}
	defer rows.Close()
	resolved := make(map[string]ports.TopologyNode, len(ids))
	for rows.Next() {
		var node ports.TopologyNode
		var elementType, dataSourceID, name, status sql.NullString
		var lastSeen nullableTime
		if err := rows.Scan(&node.InternalID, &elementType, &dataSourceID, &name, &status, &lastSeen); err != nil {
			return nil, fmt.Errorf("scan topology node: %w", err)
		}
		node.Type = stringPointer(elementType)
		node.DataSourceID = stringPointer(dataSourceID)
		node.Name = stringPointer(name)
		node.Status = stringPointer(status)
		if lastSeen.Valid {
			value := lastSeen.Time
			node.LastSeen = &value
		}
		resolved[node.InternalID] = node
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate topology nodes: %w", err)
	}
	nodes := make([]ports.TopologyNode, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		if node, ok := resolved[id]; ok {
			nodes = append(nodes, node)
		} else {
			nodes = append(nodes, ports.TopologyNode{InternalID: id})
		}
	}
	return nodes, nil
}

func relationshipKey(relationship ports.Relationship) string {
	return strings.Join([]string{relationship.DataSourceID, relationship.SourceInternalID, relationship.DestinationInternalID, relationship.RelationshipType, fmt.Sprint(relationship.RelationType)}, "\x00")
}

func relationshipNeighbors(relationship ports.Relationship, frontier []string) []string {
	current := make(map[string]bool, len(frontier))
	for _, id := range frontier {
		current[id] = true
	}
	neighbors := make([]string, 0, 2)
	if current[relationship.SourceInternalID] && relationship.DestinationInternalID != relationship.SourceInternalID {
		neighbors = append(neighbors, relationship.DestinationInternalID)
	}
	if current[relationship.DestinationInternalID] && relationship.SourceInternalID != relationship.DestinationInternalID {
		neighbors = append(neighbors, relationship.SourceInternalID)
	}
	return neighbors
}

func oppositeEndpoint(relationship ports.Relationship, endpoint string) string {
	if relationship.SourceInternalID == endpoint {
		return relationship.DestinationInternalID
	}
	return relationship.SourceInternalID
}

func (r queryRepository) buildPath(ctx context.Context, fromID, toID string, parent map[string]string, previous map[string]ports.Relationship) (ports.RelationshipPath, error) {
	ids := []string{toID}
	relationships := make([]ports.Relationship, 0)
	for current := toID; current != fromID; current = parent[current] {
		relationships = append(relationships, previous[current])
		ids = append(ids, parent[current])
	}
	for left, right := 0, len(relationships)-1; left < right; left, right = left+1, right-1 {
		relationships[left], relationships[right] = relationships[right], relationships[left]
	}
	nodes, err := r.topologyNodes(ctx, sliceSet(ids))
	if err != nil {
		return ports.RelationshipPath{}, err
	}
	return ports.RelationshipPath{Found: true, Relationships: relationships, Nodes: nodes}, nil
}

func sliceSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
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
