package metrics

import (
	"testing"

	"github.com/coordimap/agent/pkg/domain/agent"
)

func TestParseRules(t *testing.T) {
	t.Run("parses array and applies defaults", func(t *testing.T) {
		rules, err := ParseRules(`[
			{"id":"r1","provider":"prometheus","query":"up","target":{"resolver":"kubernetes_service","namespace_label":"namespace","name_label":"service"},"threshold":{"value":1}},
			{"id":"r2","provider":"gcp_monitoring","metric_type":"cloudsql.googleapis.com/database/cpu/utilization","target":{"resolver":"gcp_cloudsql","name_label":"database_id","region_label":"region"},"threshold":{"operator":">=","value":0.5}}
		]`)
		if err != nil {
			t.Fatalf("ParseRules() unexpected error: %v", err)
		}

		if len(rules) != 2 {
			t.Fatalf("ParseRules() length = %d, want 2", len(rules))
		}

		if rules[0].Lookback != "5m" {
			t.Fatalf("rule[0].Lookback = %q, want 5m", rules[0].Lookback)
		}

		if rules[0].Threshold.Operator != ">" {
			t.Fatalf("rule[0].Threshold.Operator = %q, want >", rules[0].Threshold.Operator)
		}

		if rules[0].Target.MappingValueType != MappingValueTypeInternalID {
			t.Fatalf("rule[0].Target.MappingValueType = %q, want %q", rules[0].Target.MappingValueType, MappingValueTypeInternalID)
		}
	})

	t.Run("fails for duplicate IDs", func(t *testing.T) {
		_, err := ParseRules(`[
			{"id":"same","provider":"prometheus","query":"up","target":{"resolver":"kubernetes_service"},"threshold":{"value":1}},
			{"id":"same","provider":"prometheus","query":"up","target":{"resolver":"kubernetes_service"},"threshold":{"value":1}}
		]`)
		if err == nil {
			t.Fatalf("ParseRules() expected duplicate id error")
		}
	})
}

func TestParseRulesFromDataSource(t *testing.T) {
	dataSource := agent.DataSource{
		DataSourceID: "kube-prod",
		Config: agent.DataSourceConfig{
			ValuePairs: []agent.KeyValue{{
				Key:   ConfigMetricRules,
				Value: `{"id":"r1","provider":"prometheus","query":"up","target":{"resolver":"kubernetes_service"},"threshold":{"value":0}}`,
			}},
		},
	}

	rules, err := ParseRulesFromDataSource(dataSource)
	if err != nil {
		t.Fatalf("ParseRulesFromDataSource() unexpected error: %v", err)
	}

	if len(rules) != 1 {
		t.Fatalf("len(ParseRulesFromDataSource()) = %d, want 1", len(rules))
	}
}

func TestEvaluateThreshold(t *testing.T) {
	tests := []struct {
		name      string
		operator  string
		value     float64
		threshold float64
		want      bool
	}{
		{name: "greater", operator: ">", value: 10, threshold: 5, want: true},
		{name: "greater or equal", operator: ">=", value: 5, threshold: 5, want: true},
		{name: "lower", operator: "<", value: 1, threshold: 2, want: true},
		{name: "lower or equal", operator: "<=", value: 2, threshold: 2, want: true},
		{name: "equal", operator: "==", value: 2, threshold: 2, want: true},
		{name: "not equal", operator: "!=", value: 3, threshold: 2, want: true},
		{name: "unsupported", operator: "~~", value: 3, threshold: 2, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateThreshold(tt.value, ThresholdConfig{Operator: tt.operator, Value: tt.threshold})
			if got != tt.want {
				t.Fatalf("EvaluateThreshold() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenderTemplate(t *testing.T) {
	labels := map[string]string{"namespace": "payments", "service": "checkout"}
	resourceLabels := map[string]string{"database_id": "orders", "region": "europe-west1"}

	got := RenderTemplate("${label.namespace}/${label.service}:${resource.database_id}@${resource.region}", labels, resourceLabels)
	if got != "payments/checkout:orders@europe-west1" {
		t.Fatalf("RenderTemplate() = %q", got)
	}
}
