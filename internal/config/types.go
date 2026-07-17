package configuration

import (
	"github.com/coordimap/agent/internal/metrics"
	"github.com/coordimap/agent/pkg/domain/agent"
)

// Config defines the interface for retrieving configuration data.
type Config interface {
	GetAllDataSources() map[string][]*agent.DataSource
	GetDatabaseConfig() (*DatabaseConfig, error)
}

// CoordimapConfigNameValueConfig represents a name-value pair configuration item.
type CoordimapConfigNameValueConfig struct {
	Name  string
	Value string
	Send  bool
}

// CoordimapConfigDataSource represents a data source configuration.
type CoordimapConfigDataSource struct {
	Type        string
	ID          string
	Config      []CoordimapConfigNameValueConfig
	MetricRules []metrics.RuleDeclaration `yaml:"metric_rules"`
}

// DatabaseConfig represents optional local crawl storage configuration.
type DatabaseConfig struct {
	Driver           string `yaml:"driver"`
	ConnectionString string `yaml:"connection_string"`
}

// Coordimap holds the configuration specific to the Coordimap integration.
type Coordimap struct {
	Database    *DatabaseConfig             `yaml:"database,omitempty"`
	DataSources []CoordimapConfigDataSource `yaml:"data_sources"`
}

// CoordimapConfig represents the top-level configuration structure for Coordimap.
type CoordimapConfig struct {
	Coordimap Coordimap `yaml:"coordimap"`
}
