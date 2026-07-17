package gcp

import "github.com/coordimap/agent/internal/metrics"

const (
	PredefinedCloudSQLHighCPU         = "cloudsql_high_cpu"
	PredefinedCloudSQLHighConnections = "cloudsql_high_connections"
	PredefinedVMHighCPU               = "vm_high_cpu"
)

func init() {
	metrics.RegisterPredefinedBuilder(metrics.ProviderGCPMonitoring, PredefinedCloudSQLHighCPU, buildCloudSQLHighCPU)
	metrics.RegisterPredefinedBuilder(metrics.ProviderGCPMonitoring, PredefinedCloudSQLHighConnections, buildCloudSQLHighConnections)
	metrics.RegisterPredefinedBuilder(metrics.ProviderGCPMonitoring, PredefinedVMHighCPU, buildVMHighCPU)
}

func buildCloudSQLHighCPU(params map[string]any) (metrics.RuleConfig, error) {
	lookback := metricsGetStringParam(params, "lookback", "5m")
	threshold := metricsGetFloatParam(params, "threshold", 0.8)

	return metrics.RuleConfig{
		Provider:   metrics.ProviderGCPMonitoring,
		MetricType: "cloudsql.googleapis.com/database/cpu/utilization",
		Lookback:   lookback,
		Threshold: metrics.ThresholdConfig{
			Operator: ">",
			Value:    threshold,
		},
		Target: metrics.TargetConfig{
			Resolver:    metrics.ResolverGCPCloudSQL,
			NameLabel:   "database_id",
			RegionLabel: "region",
		},
	}, nil
}

func buildCloudSQLHighConnections(params map[string]any) (metrics.RuleConfig, error) {
	lookback := metricsGetStringParam(params, "lookback", "5m")
	threshold := metricsGetFloatParam(params, "threshold", 200)
	metricType := metricsGetStringParam(params, "metric_type", "cloudsql.googleapis.com/database/network/connections")

	return metrics.RuleConfig{
		Provider:         metrics.ProviderGCPMonitoring,
		MetricType:       metricType,
		Lookback:         lookback,
		AlignmentPeriod:  metricsGetStringParam(params, "alignment_period", "300s"),
		PerSeriesAligner: metricsGetStringParam(params, "per_series_aligner", "ALIGN_MEAN"),
		Threshold: metrics.ThresholdConfig{
			Operator: ">",
			Value:    threshold,
		},
		Target: metrics.TargetConfig{
			Resolver:    metrics.ResolverGCPCloudSQL,
			NameLabel:   "database_id",
			RegionLabel: "region",
		},
	}, nil
}

func buildVMHighCPU(params map[string]any) (metrics.RuleConfig, error) {
	lookback := metricsGetStringParam(params, "lookback", "5m")
	threshold := metricsGetFloatParam(params, "threshold", 0.8)

	return metrics.RuleConfig{
		Provider:         metrics.ProviderGCPMonitoring,
		MetricType:       "compute.googleapis.com/instance/cpu/utilization",
		Lookback:         lookback,
		AlignmentPeriod:  metricsGetStringParam(params, "alignment_period", "300s"),
		PerSeriesAligner: metricsGetStringParam(params, "per_series_aligner", "ALIGN_MEAN"),
		Threshold: metrics.ThresholdConfig{
			Operator: ">",
			Value:    threshold,
		},
		Target: metrics.TargetConfig{
			Resolver:  metrics.ResolverGCPVMInstance,
			NameLabel: "instance_name",
			ZoneLabel: "zone",
		},
	}, nil
}

func metricsGetStringParam(params map[string]any, key, defaultValue string) string {
	value, ok := params[key]
	if !ok {
		return defaultValue
	}

	asString, ok := value.(string)
	if !ok || asString == "" {
		return defaultValue
	}

	return asString
}

func metricsGetFloatParam(params map[string]any, key string, defaultValue float64) float64 {
	value, ok := params[key]
	if !ok {
		return defaultValue
	}

	switch parsed := value.(type) {
	case int:
		return float64(parsed)
	case int32:
		return float64(parsed)
	case int64:
		return float64(parsed)
	case float32:
		return float64(parsed)
	case float64:
		return parsed
	default:
		return defaultValue
	}
}
