package configuration

import (
	"github.com/coordimap/agent/internal/metrics"
	"github.com/coordimap/agent/pkg/domain/agent"
)

// Config defines the interface for retrieving configuration data.
type Config interface {
	GetAllDataSources() map[string][]*agent.DataSource
	GetCoordimapKey() (string, error)
	GetSkipFields() []string
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

// Coordimap holds the configuration specific to the Coordimap integration.
type Coordimap struct {
	APIKey      string                      `yaml:"api_key"`
	SkipFields  []string                    `yaml:"skip_fields"`
	DataSources []CoordimapConfigDataSource `yaml:"data_sources"`
}

// CoordimapConfig represents the top-level configuration structure for Coordimap.
type CoordimapConfig struct {
	Coordimap Coordimap `yaml:"coordimap"`
}
