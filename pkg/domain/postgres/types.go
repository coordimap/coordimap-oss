package postgres

const (
	POSTGRES_TYPE_DB                = "postgres.database"
	POSTGRES_TYPE_SCHEMA            = "postgres.schema"
	POSTGRES_TYPE_TABLE             = "postgres.table"
	POSTGRES_TYPE_INDEX             = "postgres.index"
	POSTGRES_TYPE_VIEW              = "postgres.view"
	POSTGRES_TYPE_MATERIALIZED_VIEW = "postgres.materialized_view"
	POSTGRES_TYPE_FUNCTION          = "postgres.function"
	POSTGRES_TYPE_PROCEDURE         = "postgres.procedure"
)

const (
	POSTGRES_CONSTRAINT_PK     = "postgres.primary_key"
	POSTGRES_CONSTRAINT_FK     = "postgres.foreign_key"
	POSTGRES_CONSTRAINT_UNIQUE = "postgres.unique"
)
