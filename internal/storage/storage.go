// Package storage opens configured local crawl storage.
package storage

import (
	"fmt"

	"github.com/coordimap/agent/internal/app/ports"
	"github.com/coordimap/agent/internal/storage/postgres"
	"github.com/coordimap/agent/internal/storage/sqlite"
)

// Open opens a store for driver using connectionString.
func Open(driver, connectionString string) (ports.Store, error) {
	switch driver {
	case "sqlite":
		return sqlite.Open(connectionString)
	case "postgres":
		return postgres.Open(connectionString)
	default:
		return nil, fmt.Errorf("unsupported storage driver %q", driver)
	}
}
