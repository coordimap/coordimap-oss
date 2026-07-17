package metrictrigger

import "time"

const (
	TriggerTypeMetricRule = "metric_rule"
)

// Threshold captures the threshold condition that triggered a metric event.
type Threshold struct {
	Operator string  `json:"operator"`
	Value    float64 `json:"value"`
}

// Target represents one internal asset matched by a metric rule.
type Target struct {
	InternalID     string            `json:"internal_id"`
	Value          float64           `json:"value"`
	Labels         map[string]string `json:"labels,omitempty"`
	ResourceLabels map[string]string `json:"resource_labels,omitempty"`
}

// Trigger is the payload stored inside metric trigger elements.
type Trigger struct {
	TriggerType    string            `json:"trigger_type"`
	Provider       string            `json:"provider"`
	DataSourceID   string            `json:"data_source_id"`
	DataSourceType string            `json:"data_source_type"`
	RuleID         string            `json:"rule_id"`
	RuleName       string            `json:"rule_name"`
	Query          string            `json:"query,omitempty"`
	Filter         string            `json:"filter,omitempty"`
	Window         string            `json:"window,omitempty"`
	Threshold      Threshold         `json:"threshold"`
	Targets        []Target          `json:"targets"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	TriggeredAt    time.Time         `json:"triggered_at"`
}
