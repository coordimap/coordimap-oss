package metrics

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/coordimap/agent/pkg/utils"
)

const (
	ConfigMetricRules = "metric_rules"

	ProviderPrometheus    = "prometheus"
	ProviderGCPMonitoring = "gcp_monitoring"

	ResolverKubernetesService     = "kubernetes_service"
	ResolverKubernetesDeployment  = "kubernetes_deployment"
	ResolverKubernetesPod         = "kubernetes_pod"
	ResolverKubernetesPVC         = "kubernetes_pvc"
	ResolverKubernetesStatefulSet = "kubernetes_statefulset"
	ResolverGCPCloudSQL           = "gcp_cloudsql"
	ResolverGCPVMInstance         = "gcp_vm_instance"
	ResolverExternalMapping       = "external_mapping"

	MappingValueTypeInternalID = "internal_id"
)

var placeholderRegex = regexp.MustCompile(`\$\{([^}]+)\}`)

type ThresholdConfig = agent.MetricRuleThreshold
type TargetConfig = agent.MetricRuleTarget
type RuleConfig = agent.MetricRule

func isEnabled(rule RuleConfig) bool {
	if rule.Enabled == nil {
		return true
	}

	return *rule.Enabled
}

func normalizedRule(rule RuleConfig) RuleConfig {
	normalized := rule
	if normalized.Name == "" {
		normalized.Name = normalized.ID
	}
	if normalized.ID == "" {
		normalized.ID = normalized.Name
	}
	if normalized.Lookback == "" {
		normalized.Lookback = "5m"
	}
	if normalized.Threshold.Operator == "" {
		normalized.Threshold.Operator = ">"
	}
	if normalized.Target.MappingValueType == "" {
		normalized.Target.MappingValueType = MappingValueTypeInternalID
	}

	return normalized
}

// NormalizeAndValidateRule applies defaults and validates one rule.
func NormalizeAndValidateRule(rule RuleConfig) (RuleConfig, error) {
	normalized := normalizedRule(rule)
	if errValidate := validateRule(normalized); errValidate != nil {
		return RuleConfig{}, errValidate
	}

	return normalized, nil
}

// NormalizeAndValidateRules applies defaults and validates all rules while checking duplicate IDs.
func NormalizeAndValidateRules(rules []RuleConfig) ([]RuleConfig, error) {
	normalizedRules := make([]RuleConfig, 0, len(rules))
	idsSeen := map[string]struct{}{}
	for _, parsed := range rules {
		normalized, errNormalize := NormalizeAndValidateRule(parsed)
		if errNormalize != nil {
			return nil, errNormalize
		}

		if _, exists := idsSeen[normalized.ID]; exists {
			return nil, fmt.Errorf("duplicate metric rule id %q", normalized.ID)
		}
		idsSeen[normalized.ID] = struct{}{}

		normalizedRules = append(normalizedRules, normalized)
	}

	return normalizedRules, nil
}

func validateRule(rule RuleConfig) error {
	if !isEnabled(rule) {
		return nil
	}

	if rule.Provider == "" {
		return fmt.Errorf("provider is required for rule %q", rule.ID)
	}

	if rule.ID == "" {
		return fmt.Errorf("id is required for metric rule")
	}

	if rule.Target.Resolver == "" {
		return fmt.Errorf("target.resolver is required for rule %q", rule.ID)
	}

	if !isThresholdOperatorAllowed(rule.Threshold.Operator) {
		return fmt.Errorf("unsupported threshold operator %q for rule %q", rule.Threshold.Operator, rule.ID)
	}

	switch rule.Provider {
	case ProviderPrometheus:
		if strings.TrimSpace(rule.Query) == "" {
			return fmt.Errorf("query is required for prometheus rule %q", rule.ID)
		}
	case ProviderGCPMonitoring:
		if strings.TrimSpace(rule.Filter) == "" && strings.TrimSpace(rule.MetricType) == "" {
			return fmt.Errorf("either filter or metric_type is required for gcp_monitoring rule %q", rule.ID)
		}
	default:
		return fmt.Errorf("unsupported metric provider %q in rule %q", rule.Provider, rule.ID)
	}

	return nil
}

// ParseRulesFromDataSource parses and validates metric rules from datasource config.
func ParseRulesFromDataSource(dataSource agent.DataSource) ([]RuleConfig, error) {
	rules := append([]RuleConfig{}, dataSource.Config.MetricRules...)

	for _, config := range dataSource.Config.ValuePairs {
		if config.Key != ConfigMetricRules {
			continue
		}

		value, errValue := utils.LoadValueFromEnvConfig(config.Value)
		if errValue != nil {
			return nil, fmt.Errorf("could not load metric_rules from env for datasource %s: %w", dataSource.DataSourceID, errValue)
		}

		parsedRules, errParse := ParseRules(strings.TrimSpace(value))
		if errParse != nil {
			return nil, fmt.Errorf("could not parse metric_rules for datasource %s: %w", dataSource.DataSourceID, errParse)
		}

		rules = append(rules, parsedRules...)
	}

	return rules, nil
}

// ParseRules parses one metric_rules JSON blob.
func ParseRules(raw string) ([]RuleConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return []RuleConfig{}, nil
	}

	parsedRules := []RuleConfig{}

	if strings.HasPrefix(strings.TrimSpace(raw), "[") {
		if errUnmarshal := json.Unmarshal([]byte(raw), &parsedRules); errUnmarshal != nil {
			return nil, fmt.Errorf("invalid metric rules json array: %w", errUnmarshal)
		}
	} else {
		single := RuleConfig{}
		if errUnmarshal := json.Unmarshal([]byte(raw), &single); errUnmarshal != nil {
			return nil, fmt.Errorf("invalid metric rule json object: %w", errUnmarshal)
		}
		parsedRules = append(parsedRules, single)
	}

	return NormalizeAndValidateRules(parsedRules)
}

// EvaluateThreshold checks a floating metric sample value against the configured threshold.
func EvaluateThreshold(value float64, threshold ThresholdConfig) bool {
	switch threshold.Operator {
	case ">":
		return value > threshold.Value
	case ">=":
		return value >= threshold.Value
	case "<":
		return value < threshold.Value
	case "<=":
		return value <= threshold.Value
	case "==":
		return value == threshold.Value
	case "!=":
		return value != threshold.Value
	default:
		return false
	}
}

// BuildTriggerElementID creates a deterministic ID for one metric trigger element.
func BuildTriggerElementID(dataSourceID, ruleID, timestampBucket string, targetIDs []string) string {
	idsCopy := make([]string, 0, len(targetIDs))
	idsCopy = append(idsCopy, targetIDs...)
	sort.Strings(idsCopy)
	return fmt.Sprintf("metric-trigger:%s:%s:%s:%s", dataSourceID, ruleID, timestampBucket, strings.Join(idsCopy, ","))
}

func isThresholdOperatorAllowed(operator string) bool {
	switch operator {
	case ">", ">=", "<", "<=", "==", "!=":
		return true
	default:
		return false
	}
}

// RenderTemplate resolves placeholders from metric labels and resource labels.
// Supported forms are ${label.<key>} and ${resource.<key>}.
func RenderTemplate(template string, labels, resourceLabels map[string]string) string {
	if template == "" {
		return ""
	}

	if labels == nil {
		labels = map[string]string{}
	}
	if resourceLabels == nil {
		resourceLabels = map[string]string{}
	}

	return placeholderRegex.ReplaceAllStringFunc(template, func(match string) string {
		groups := placeholderRegex.FindStringSubmatch(match)
		if len(groups) != 2 {
			return ""
		}

		reference := groups[1]
		if strings.HasPrefix(reference, "label.") {
			return labels[strings.TrimPrefix(reference, "label.")]
		}

		if strings.HasPrefix(reference, "resource.") {
			return resourceLabels[strings.TrimPrefix(reference, "resource.")]
		}

		if value, ok := labels[reference]; ok {
			return value
		}

		return resourceLabels[reference]
	})
}
