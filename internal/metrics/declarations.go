package metrics

import (
	"fmt"
	"strings"
)

const (
	RuleModeCustom     = "custom"
	RuleModePredefined = "predefined"
)

// CustomRuleConfig contains provider-specific query fields for custom rules.
type CustomRuleConfig struct {
	Query              string   `yaml:"query"`
	Filter             string   `yaml:"filter"`
	MetricType         string   `yaml:"metric_type"`
	AlignmentPeriod    string   `yaml:"alignment_period"`
	PerSeriesAligner   string   `yaml:"per_series_aligner"`
	CrossSeriesReducer string   `yaml:"cross_series_reducer"`
	GroupByFields      []string `yaml:"group_by_fields"`
}

// PredefinedRuleConfig references one predefined rule template.
type PredefinedRuleConfig struct {
	Name   string         `yaml:"name"`
	Params map[string]any `yaml:"params"`
}

// RuleDeclaration is the YAML declaration model for one metric rule.
type RuleDeclaration struct {
	DataSourceID   string               `yaml:"data_source_id"`
	DataSourceType string               `yaml:"data_source_type"`
	ID             string               `yaml:"id"`
	Name           string               `yaml:"name"`
	Provider       string               `yaml:"provider"`
	Mode           string               `yaml:"mode"`
	Lookback       string               `yaml:"lookback"`
	Threshold      *ThresholdConfig     `yaml:"threshold"`
	Target         *TargetConfig        `yaml:"target"`
	Enabled        *bool                `yaml:"enabled"`
	Custom         CustomRuleConfig     `yaml:"custom"`
	Predefined     PredefinedRuleConfig `yaml:"predefined"`
}

// ResolveRuleDeclaration converts a YAML declaration into a normalized rule config.
func ResolveRuleDeclaration(declaration RuleDeclaration) (RuleConfig, error) {
	if declaration.ID == "" {
		return RuleConfig{}, fmt.Errorf("metric rule id is required")
	}

	if declaration.Provider == "" {
		return RuleConfig{}, fmt.Errorf("provider is required for metric rule %q", declaration.ID)
	}

	mode := strings.ToLower(strings.TrimSpace(declaration.Mode))
	if mode == "" {
		return RuleConfig{}, fmt.Errorf("mode is required for metric rule %q", declaration.ID)
	}

	baseRule := RuleConfig{
		ID:       declaration.ID,
		Name:     declaration.Name,
		Provider: declaration.Provider,
		Lookback: declaration.Lookback,
		Enabled:  declaration.Enabled,
	}

	switch mode {
	case RuleModeCustom:
		baseRule.Query = declaration.Custom.Query
		baseRule.Filter = declaration.Custom.Filter
		baseRule.MetricType = declaration.Custom.MetricType
		baseRule.AlignmentPeriod = declaration.Custom.AlignmentPeriod
		baseRule.PerSeriesAligner = declaration.Custom.PerSeriesAligner
		baseRule.CrossSeriesReducer = declaration.Custom.CrossSeriesReducer
		baseRule.GroupByFields = declaration.Custom.GroupByFields

		if declaration.Threshold != nil {
			baseRule.Threshold = *declaration.Threshold
		}
		if declaration.Target != nil {
			baseRule.Target = *declaration.Target
		}

	case RuleModePredefined:
		predefinedRule, errPredefined := BuildPredefinedRule(declaration.Provider, declaration.Predefined.Name, declaration.Predefined.Params)
		if errPredefined != nil {
			return RuleConfig{}, fmt.Errorf("could not build predefined rule %q for provider %q: %w", declaration.Predefined.Name, declaration.Provider, errPredefined)
		}

		baseRule.Query = predefinedRule.Query
		baseRule.Filter = predefinedRule.Filter
		baseRule.MetricType = predefinedRule.MetricType
		baseRule.AlignmentPeriod = predefinedRule.AlignmentPeriod
		baseRule.PerSeriesAligner = predefinedRule.PerSeriesAligner
		baseRule.CrossSeriesReducer = predefinedRule.CrossSeriesReducer
		baseRule.GroupByFields = predefinedRule.GroupByFields
		baseRule.Threshold = predefinedRule.Threshold
		baseRule.Target = predefinedRule.Target
		if baseRule.Lookback == "" {
			baseRule.Lookback = predefinedRule.Lookback
		}

		if declaration.Threshold != nil {
			baseRule.Threshold = *declaration.Threshold
		}
		if declaration.Target != nil {
			baseRule.Target = *declaration.Target
		}

	default:
		return RuleConfig{}, fmt.Errorf("unsupported mode %q for metric rule %q", declaration.Mode, declaration.ID)
	}

	normalized, errNormalize := NormalizeAndValidateRule(baseRule)
	if errNormalize != nil {
		return RuleConfig{}, errNormalize
	}

	return normalized, nil
}
