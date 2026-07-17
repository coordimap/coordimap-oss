package prometheus

import (
	"context"
	"fmt"
	"time"

	"github.com/coordimap/agent/internal/metrics"
	"github.com/prometheus/client_golang/api"
	promV1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// Evaluator evaluates metric rules against a Prometheus backend.
type Evaluator struct {
	api promV1.API
}

// NewEvaluator creates a Prometheus evaluator.
func NewEvaluator(client api.Client) *Evaluator {
	if client == nil {
		return nil
	}

	return &Evaluator{api: promV1.NewAPI(client)}
}

// Provider returns the provider key this evaluator supports.
func (evaluator *Evaluator) Provider() string {
	return metrics.ProviderPrometheus
}

// Evaluate executes a PromQL query and returns normalized vector samples.
func (evaluator *Evaluator) Evaluate(ctx context.Context, rule metrics.RuleConfig, evalTime time.Time) ([]metrics.Sample, error) {
	if evaluator == nil || evaluator.api == nil {
		return nil, fmt.Errorf("prometheus evaluator is not configured")
	}

	result, _, errQuery := evaluator.api.Query(ctx, rule.Query, evalTime, promV1.WithTimeout(10*time.Second))
	if errQuery != nil {
		return nil, fmt.Errorf("could not evaluate prometheus query for rule %s: %w", rule.ID, errQuery)
	}

	vectorResult, ok := result.(model.Vector)
	if !ok {
		return []metrics.Sample{}, nil
	}

	samples := make([]metrics.Sample, 0, len(vectorResult))
	for _, sample := range vectorResult {
		labels := make(map[string]string, len(sample.Metric))
		for labelKey, labelValue := range sample.Metric {
			labels[string(labelKey)] = string(labelValue)
		}

		samples = append(samples, metrics.Sample{
			Value:  float64(sample.Value),
			Labels: labels,
		})
	}

	return samples, nil
}
