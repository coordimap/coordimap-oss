package configuration

import (
	"fmt"
	"os"

	"github.com/coordimap/agent/internal/metrics"
	_ "github.com/coordimap/agent/internal/metrics/gcp"
	_ "github.com/coordimap/agent/internal/metrics/prometheus"
	"github.com/coordimap/agent/pkg/utils"

	"github.com/coordimap/agent/pkg/domain/agent"
	"gopkg.in/yaml.v3"
)

type yamlConfig struct {
	parsedConfig   *Coordimap
	yamlConfigPath string
}

func (coordimapConfig *yamlConfig) GetCoordimapKey() (string, error) {
	if coordimapConfig.parsedConfig == nil {
		return "", fmt.Errorf("configuration is nil")
	}

	value, err := utils.LoadValueFromEnvConfig(coordimapConfig.parsedConfig.APIKey)
	if err != nil {
		return "", err
	}

	return value, nil
}

func (coordimapConfig *yamlConfig) GetSkipFields() []string {
	if coordimapConfig.parsedConfig == nil {
		return []string{}
	}

	return coordimapConfig.parsedConfig.SkipFields
}

func (coordimapConfig *yamlConfig) GetAllDataSources() map[string][]*agent.DataSource {
	if coordimapConfig.parsedConfig == nil {
		return map[string][]*agent.DataSource{}
	}

	allDataSources := map[string][]*agent.DataSource{}
	for _, dataSource := range coordimapConfig.parsedConfig.DataSources {
		info := agent.DataSourceInfo{
			Name: dataSource.ID,
			Desc: dataSource.ID,
			Type: dataSource.Type,
		}

		dsValuePairs := []agent.KeyValue{}
		for _, valuePair := range dataSource.Config {
			dsValuePairs = append(dsValuePairs, agent.KeyValue{
				Key:   valuePair.Name,
				Value: valuePair.Value,
			})
		}

		resolvedRules := []metrics.RuleConfig{}
		for _, ruleDeclaration := range dataSource.MetricRules {
			if ruleDeclaration.ID == "" {
				continue
			}

			resolvedRule, errResolve := metrics.ResolveRuleDeclaration(ruleDeclaration)
			if errResolve != nil {
				continue
			}

			resolvedRules = append(resolvedRules, resolvedRule)
		}

		currentDS := &agent.DataSource{
			Info:         info,
			DataSourceID: dataSource.ID,
			Config: agent.DataSourceConfig{
				ValuePairs:  dsValuePairs,
				MetricRules: resolvedRules,
			},
		}

		allDataSources[info.Type] = append(allDataSources[info.Type], currentDS)
	}

	return allDataSources
}

// NewYamlFileConfig reads in the yaml file provided in the path and generates the correct config structure
func NewYamlFileConfig(filePath string) (Config, error) {
	yamlFile, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	parsedYaml, errParsedYaml := NewYamlStringConfig(string(yamlFile))
	if errParsedYaml != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", errParsedYaml)
	}

	return &yamlConfig{
		parsedConfig:   &parsedYaml.Coordimap,
		yamlConfigPath: filePath,
	}, nil
}

// NewYamlStringConfig reads in the yaml string provided and generates the correct config structure
func NewYamlStringConfig(yamlContent string) (*CoordimapConfig, error) {
	config := CoordimapConfig{}

	if errorUnmarshal := yaml.Unmarshal([]byte(yamlContent), &config); errorUnmarshal != nil {
		return nil, fmt.Errorf("failed to unmarshal yaml: %w", errorUnmarshal)
	}

	// Basic validation
	if config.Coordimap.APIKey == "" {
		// Check if it's an env var placeholder, if not, it's missing
		// Actually, even if it is a placeholder, it should be present in the struct.
		// If the string is empty, it means the key is missing from YAML.
		return nil, fmt.Errorf("missing required field: coordimap.api_key")
	}

	for _, configuredDataSource := range config.Coordimap.DataSources {
		if len(configuredDataSource.MetricRules) == 0 {
			continue
		}

		if configuredDataSource.Type != "kubernetes" && configuredDataSource.Type != "gcp" {
			return nil, fmt.Errorf("metric_rules are currently supported only for kubernetes and gcp data sources, found type %q for datasource %q", configuredDataSource.Type, configuredDataSource.ID)
		}

		rules := make([]metrics.RuleConfig, 0, len(configuredDataSource.MetricRules))
		for _, configuredRule := range configuredDataSource.MetricRules {
			resolvedRule, errResolve := metrics.ResolveRuleDeclaration(configuredRule)
			if errResolve != nil {
				return nil, fmt.Errorf("invalid metric rule %q for datasource %q: %w", configuredRule.ID, configuredDataSource.ID, errResolve)
			}

			rules = append(rules, resolvedRule)
		}

		if _, errMetricRules := metrics.NormalizeAndValidateRules(rules); errMetricRules != nil {
			return nil, fmt.Errorf("invalid metric rules for datasource %q: %w", configuredDataSource.ID, errMetricRules)
		}
	}

	return &config, nil
}
