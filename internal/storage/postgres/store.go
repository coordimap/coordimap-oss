// Package postgres opens PostgreSQL-backed local crawl storage.
package postgres

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/coordimap/agent/internal/app/ports"
	"github.com/coordimap/agent/internal/storage/sqlstore"
	_ "github.com/lib/pq"
)

// Open opens a PostgreSQL store with crawler-aligned connection pooling.
func Open(connectionString string) (ports.Store, error) {
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL database: %w", err)
	}
	db.SetMaxIdleConns(10)
	db.SetConnMaxIdleTime(time.Hour)
	db.SetMaxOpenConns(20)
	db.SetConnMaxLifetime(20 * time.Minute)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping PostgreSQL database: %w", err)
	}
	return sqlstore.NewStore(db, sqlstore.Postgres, migrations), nil
}

var migrations = []sqlstore.Migration{
	{
		Version: 1,
		Name:    "initial",
		SQL: `CREATE TABLE data_sources (
	id TEXT PRIMARY KEY,
	type TEXT NOT NULL,
	name TEXT NOT NULL,
	description TEXT NOT NULL,
	config_json TEXT NOT NULL,
	first_seen TIMESTAMP NOT NULL,
	last_seen TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);
CREATE TABLE crawl_runs (
	id TEXT PRIMARY KEY,
	data_source_id TEXT NOT NULL REFERENCES data_sources(id),
	crawl_internal_id TEXT NOT NULL,
	started_at TIMESTAMP NOT NULL,
	completed_at TIMESTAMP NOT NULL,
	element_count INTEGER NOT NULL,
	relationship_count INTEGER NOT NULL,
	error TEXT
);
CREATE TABLE crawled_elements (
	data_source_id TEXT NOT NULL REFERENCES data_sources(id),
	internal_id TEXT NOT NULL,
	element_type TEXT NOT NULL,
	name TEXT NOT NULL,
	hash TEXT NOT NULL,
	retrieved_at TIMESTAMP NOT NULL,
	is_json_data BOOLEAN NOT NULL,
	raw_data BYTEA NOT NULL,
	raw_json TEXT,
	version TEXT NOT NULL,
	status TEXT NOT NULL,
	first_seen TIMESTAMP NOT NULL,
	last_seen TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL,
	PRIMARY KEY(data_source_id, internal_id, element_type)
);
CREATE TABLE crawled_element_versions (
	id BIGSERIAL PRIMARY KEY,
	data_source_id TEXT NOT NULL REFERENCES data_sources(id),
	crawl_run_id TEXT NOT NULL REFERENCES crawl_runs(id),
	internal_id TEXT NOT NULL,
	element_type TEXT NOT NULL,
	name TEXT NOT NULL,
	hash TEXT NOT NULL,
	retrieved_at TIMESTAMP NOT NULL,
	is_json_data BOOLEAN NOT NULL,
	raw_data BYTEA NOT NULL,
	raw_json TEXT,
	version TEXT NOT NULL,
	status TEXT NOT NULL,
	observed_at TIMESTAMP NOT NULL,
	UNIQUE(data_source_id, internal_id, element_type, hash)
);
CREATE TABLE relationships (
	data_source_id TEXT NOT NULL REFERENCES data_sources(id),
	crawl_run_id TEXT NOT NULL REFERENCES crawl_runs(id),
	source_internal_id TEXT NOT NULL,
	destination_internal_id TEXT NOT NULL,
	relationship_type TEXT NOT NULL,
	relation_type INTEGER NOT NULL,
	first_seen TIMESTAMP NOT NULL,
	last_seen TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL,
	PRIMARY KEY(data_source_id, source_internal_id, destination_internal_id, relationship_type, relation_type)
);`,
	},
	{
		Version: 2,
		Name:    "read_path_indexes",
		SQL: `CREATE INDEX crawled_elements_data_source_type_idx ON crawled_elements(data_source_id, element_type);
CREATE INDEX crawled_elements_name_idx ON crawled_elements(name);
CREATE INDEX crawled_elements_last_seen_idx ON crawled_elements(last_seen);
CREATE INDEX relationships_source_internal_id_idx ON relationships(source_internal_id);
CREATE INDEX relationships_destination_internal_id_idx ON relationships(destination_internal_id);
CREATE INDEX relationships_relation_type_idx ON relationships(relation_type);
CREATE INDEX crawl_runs_data_source_started_at_idx ON crawl_runs(data_source_id, started_at);`,
	},
}
