package gcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/coordimap/agent/internal/metrics"
	"google.golang.org/api/monitoring/v3"
)

// Evaluator evaluates metric rules against Google Cloud Monitoring.
type Evaluator struct {
	service   *monitoring.Service
	projectID string
}

// NewEvaluator creates a Cloud Monitoring evaluator.
func NewEvaluator(service *monitoring.Service, projectID string) *Evaluator {
	if service == nil || strings.TrimSpace(projectID) == "" {
		return nil
	}

	return &Evaluator{service: service, projectID: projectID}
}

// Provider returns the provider key this evaluator supports.
func (evaluator *Evaluator) Provider() string {
	return metrics.ProviderGCPMonitoring
}

// Evaluate executes a Cloud Monitoring query and returns normalized samples.
func (evaluator *Evaluator) Evaluate(ctx context.Context, rule metrics.RuleConfig, evalTime time.Time) ([]metrics.Sample, error) {
	if evaluator == nil || evaluator.service == nil {
		return nil, fmt.Errorf("gcp monitoring evaluator is not configured")
	}

	lookbackDuration, errLookback := time.ParseDuration(rule.Lookback)
	if errLookback != nil {
		return nil, fmt.Errorf("invalid lookback duration for gcp metric rule %s: %w", rule.ID, errLookback)
	}

	filter := strings.TrimSpace(rule.Filter)
	if filter == "" {
		filter = fmt.Sprintf(`metric.type="%s"`, strings.TrimSpace(rule.MetricType))
	}

	queryStart := evalTime.UTC().Add(-lookbackDuration)

	call := evaluator.service.Projects.TimeSeries.List(fmt.Sprintf("projects/%s", evaluator.projectID)).
		Filter(filter).
		IntervalStartTime(queryStart.Format(time.RFC3339Nano)).
		IntervalEndTime(evalTime.UTC().Format(time.RFC3339Nano))

	if rule.AlignmentPeriod != "" {
		call = call.AggregationAlignmentPeriod(rule.AlignmentPeriod)
	}
	if rule.PerSeriesAligner != "" {
		call = call.AggregationPerSeriesAligner(rule.PerSeriesAligner)
	}
	if rule.CrossSeriesReducer != "" {
		call = call.AggregationCrossSeriesReducer(rule.CrossSeriesReducer)
	}
	if len(rule.GroupByFields) > 0 {
		call = call.AggregationGroupByFields(rule.GroupByFields...)
	}

	response, errQuery := call.Context(ctx).Do()
	if errQuery != nil {
		return nil, fmt.Errorf("could not evaluate gcp monitoring query for rule %s: %w", rule.ID, errQuery)
	}

	samples := []metrics.Sample{}
	for _, series := range response.TimeSeries {
		value, foundValue := getFirstPointValue(series)
		if !foundValue {
			continue
		}

		metricLabels := map[string]string{}
		if series.Metric != nil {
			for key, val := range series.Metric.Labels {
				metricLabels[key] = val
			}
		}

		resourceLabels := map[string]string{}
		if series.Resource != nil {
			for key, val := range series.Resource.Labels {
				resourceLabels[key] = val
			}
		}

		samples = append(samples, metrics.Sample{
			Value:          value,
			Labels:         metricLabels,
			ResourceLabels: resourceLabels,
		})
	}

	return samples, nil
}

func getFirstPointValue(series *monitoring.TimeSeries) (float64, bool) {
	if series == nil || len(series.Points) == 0 || series.Points[0] == nil || series.Points[0].Value == nil {
		return 0, false
	}

	typedValue := series.Points[0].Value
	if typedValue.Int64Value != nil {
		return float64(*typedValue.Int64Value), true
	}

	if typedValue.BoolValue != nil {
		if *typedValue.BoolValue {
			return 1, true
		}

		return 0, true
	}

	if typedValue.DoubleValue != nil {
		return *typedValue.DoubleValue, true
	}

	if typedValue.StringValue != nil {
		parsedFloat, errParsedFloat := strconv.ParseFloat(*typedValue.StringValue, 64)
		if errParsedFloat != nil {
			return 0, false
		}

		return parsedFloat, true
	}

	return 0, false
}
