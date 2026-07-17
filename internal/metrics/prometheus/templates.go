package prometheus

import (
	"fmt"
	"strings"

	"github.com/coordimap/agent/internal/metrics"
)

const (
	PredefinedKubernetesServiceHigh5xx                = "kubernetes_service_high_5xx"
	PredefinedKubernetesDeploymentHigh5xx             = "kubernetes_deployment_high_5xx"
	PredefinedKubernetesServiceHighLatency            = "kubernetes_service_high_latency"
	PredefinedKubernetesPodHighRestartRate            = "kubernetes_pod_high_restart_rate"
	PredefinedKubernetesPodCrashloopOrImagePullError  = "kubernetes_pod_crashloop_or_imagepull_error"
	PredefinedKubernetesPodNotReady                   = "kubernetes_pod_not_ready"
	PredefinedKubernetesDeploymentUnavailableReplicas = "kubernetes_deployment_unavailable_replicas"
	PredefinedKubernetesDeploymentAvailabilityGap     = "kubernetes_deployment_availability_gap"
	PredefinedKubernetesPodHighCPUUsage               = "kubernetes_pod_high_cpu_usage"
	PredefinedKubernetesPodHighMemoryWorkingSet       = "kubernetes_pod_high_memory_workingset"
	PredefinedKubernetesPodCPUThrottlingHigh          = "kubernetes_pod_cpu_throttling_high"
	PredefinedKubernetesPodOOMEvents                  = "kubernetes_pod_oom_events"
	PredefinedKubernetesPodUnschedulable              = "kubernetes_pod_unschedulable"
	PredefinedKubernetesPVCLowFreeSpace               = "kubernetes_pvc_low_free_space"
	PredefinedKubernetesPVCFreeSpaceBurnRate          = "kubernetes_pvc_free_space_burn_rate"
	PredefinedKubernetesPVCInodeLowFree               = "kubernetes_inode_low_free"
	PredefinedKubernetesStatefulSetPVCLowFreeSpace    = "kubernetes_statefulset_pvc_low_free_space"
)

func init() {
	metrics.RegisterPredefinedBuilder(metrics.ProviderPrometheus, PredefinedKubernetesServiceHigh5xx, buildKubernetesServiceHigh5xx)
	metrics.RegisterPredefinedBuilder(metrics.ProviderPrometheus, PredefinedKubernetesDeploymentHigh5xx, buildKubernetesDeploymentHigh5xx)
	metrics.RegisterPredefinedBuilder(metrics.ProviderPrometheus, PredefinedKubernetesServiceHighLatency, buildKubernetesServiceHighLatency)
	metrics.RegisterPredefinedBuilder(metrics.ProviderPrometheus, PredefinedKubernetesPodHighRestartRate, buildKubernetesPodHighRestartRate)
	metrics.RegisterPredefinedBuilder(metrics.ProviderPrometheus, PredefinedKubernetesPodCrashloopOrImagePullError, buildKubernetesPodCrashloopOrImagePullError)
	metrics.RegisterPredefinedBuilder(metrics.ProviderPrometheus, PredefinedKubernetesPodNotReady, buildKubernetesPodNotReady)
	metrics.RegisterPredefinedBuilder(metrics.ProviderPrometheus, PredefinedKubernetesDeploymentUnavailableReplicas, buildKubernetesDeploymentUnavailableReplicas)
	metrics.RegisterPredefinedBuilder(metrics.ProviderPrometheus, PredefinedKubernetesDeploymentAvailabilityGap, buildKubernetesDeploymentAvailabilityGap)
	metrics.RegisterPredefinedBuilder(metrics.ProviderPrometheus, PredefinedKubernetesPodHighCPUUsage, buildKubernetesPodHighCPUUsage)
	metrics.RegisterPredefinedBuilder(metrics.ProviderPrometheus, PredefinedKubernetesPodHighMemoryWorkingSet, buildKubernetesPodHighMemoryWorkingSet)
	metrics.RegisterPredefinedBuilder(metrics.ProviderPrometheus, PredefinedKubernetesPodCPUThrottlingHigh, buildKubernetesPodCPUThrottlingHigh)
	metrics.RegisterPredefinedBuilder(metrics.ProviderPrometheus, PredefinedKubernetesPodOOMEvents, buildKubernetesPodOOMEvents)
	metrics.RegisterPredefinedBuilder(metrics.ProviderPrometheus, PredefinedKubernetesPodUnschedulable, buildKubernetesPodUnschedulable)
	metrics.RegisterPredefinedBuilder(metrics.ProviderPrometheus, PredefinedKubernetesPVCLowFreeSpace, buildKubernetesPVCLowFreeSpace)
	metrics.RegisterPredefinedBuilder(metrics.ProviderPrometheus, PredefinedKubernetesPVCFreeSpaceBurnRate, buildKubernetesPVCFreeSpaceBurnRate)
	metrics.RegisterPredefinedBuilder(metrics.ProviderPrometheus, PredefinedKubernetesPVCInodeLowFree, buildKubernetesPVCInodeLowFree)
	metrics.RegisterPredefinedBuilder(metrics.ProviderPrometheus, PredefinedKubernetesStatefulSetPVCLowFreeSpace, buildKubernetesStatefulSetPVCLowFreeSpace)
}

func buildKubernetesServiceHigh5xx(params map[string]any) (metrics.RuleConfig, error) {
	window := metricsGetStringParam(params, "window", "5m")
	threshold := metricsGetFloatParam(params, "threshold", 1)

	query := fmt.Sprintf(`sum(rate(istio_requests_total{response_code=~"5.."}[%s])) by (destination_workload_namespace, destination_canonical_service)`, window)

	return metrics.RuleConfig{
		Provider: metrics.ProviderPrometheus,
		Query:    query,
		Lookback: window,
		Threshold: metrics.ThresholdConfig{
			Operator: ">",
			Value:    threshold,
		},
		Target: metrics.TargetConfig{
			Resolver:       metrics.ResolverKubernetesService,
			NamespaceLabel: "destination_workload_namespace",
			NameLabel:      "destination_canonical_service",
		},
	}, nil
}

func buildKubernetesDeploymentHigh5xx(params map[string]any) (metrics.RuleConfig, error) {
	window := metricsGetStringParam(params, "window", "5m")
	threshold := metricsGetFloatParam(params, "threshold", 1)

	query := fmt.Sprintf(`sum(rate(istio_requests_total{response_code=~"5.."}[%s])) by (destination_workload_namespace, destination_workload)`, window)

	return metrics.RuleConfig{
		Provider: metrics.ProviderPrometheus,
		Query:    query,
		Lookback: window,
		Threshold: metrics.ThresholdConfig{
			Operator: ">",
			Value:    threshold,
		},
		Target: metrics.TargetConfig{
			Resolver:       metrics.ResolverKubernetesDeployment,
			NamespaceLabel: "destination_workload_namespace",
			NameLabel:      "destination_workload",
		},
	}, nil
}

func buildKubernetesServiceHighLatency(params map[string]any) (metrics.RuleConfig, error) {
	window := metricsGetStringParam(params, "window", "5m")
	quantile := metricsGetFloatParam(params, "quantile", 0.95)
	threshold := metricsGetFloatParam(params, "threshold", 500)

	query := fmt.Sprintf(`histogram_quantile(%v, sum(rate(istio_request_duration_milliseconds_bucket[%s])) by (le, destination_workload_namespace, destination_canonical_service))`, quantile, window)

	return metrics.RuleConfig{
		Provider: metrics.ProviderPrometheus,
		Query:    query,
		Lookback: window,
		Threshold: metrics.ThresholdConfig{
			Operator: ">",
			Value:    threshold,
		},
		Target: metrics.TargetConfig{
			Resolver:       metrics.ResolverKubernetesService,
			NamespaceLabel: "destination_workload_namespace",
			NameLabel:      "destination_canonical_service",
		},
	}, nil
}

func buildKubernetesPodHighRestartRate(params map[string]any) (metrics.RuleConfig, error) {
	window := metricsGetStringParam(params, "window", "10m")
	threshold := metricsGetFloatParam(params, "threshold", 3)

	query := fmt.Sprintf(`sum(increase(kube_pod_container_status_restarts_total[%s])) by (namespace, pod)`, window)

	return metrics.RuleConfig{
		Provider: metrics.ProviderPrometheus,
		Query:    query,
		Lookback: window,
		Threshold: metrics.ThresholdConfig{
			Operator: ">",
			Value:    threshold,
		},
		Target: metrics.TargetConfig{
			Resolver:       metrics.ResolverKubernetesPod,
			NamespaceLabel: "namespace",
			NameLabel:      "pod",
		},
	}, nil
}

func buildKubernetesPodCrashloopOrImagePullError(params map[string]any) (metrics.RuleConfig, error) {
	lookback := metricsGetStringParam(params, "lookback", "5m")
	reasonRegex := metricsGetStringParam(params, "reason_regex", "CrashLoopBackOff|ImagePullBackOff|ErrImagePull")

	query := fmt.Sprintf(`max by (namespace, pod) (max_over_time(kube_pod_container_status_waiting_reason{reason=~"%s"}[%s]))`, reasonRegex, lookback)

	return metrics.RuleConfig{
		Provider: metrics.ProviderPrometheus,
		Query:    query,
		Lookback: lookback,
		Threshold: metrics.ThresholdConfig{
			Operator: ">",
			Value:    0,
		},
		Target: metrics.TargetConfig{
			Resolver:       metrics.ResolverKubernetesPod,
			NamespaceLabel: "namespace",
			NameLabel:      "pod",
		},
	}, nil
}

func buildKubernetesPodNotReady(params map[string]any) (metrics.RuleConfig, error) {
	lookback := metricsGetStringParam(params, "lookback", "5m")

	query := fmt.Sprintf(`max by (namespace, pod) (1 - max_over_time(kube_pod_status_ready{condition="true"}[%s]))`, lookback)

	return metrics.RuleConfig{
		Provider: metrics.ProviderPrometheus,
		Query:    query,
		Lookback: lookback,
		Threshold: metrics.ThresholdConfig{
			Operator: ">",
			Value:    0,
		},
		Target: metrics.TargetConfig{
			Resolver:       metrics.ResolverKubernetesPod,
			NamespaceLabel: "namespace",
			NameLabel:      "pod",
		},
	}, nil
}

func buildKubernetesDeploymentUnavailableReplicas(params map[string]any) (metrics.RuleConfig, error) {
	threshold := metricsGetFloatParam(params, "threshold", 0)

	return metrics.RuleConfig{
		Provider: metrics.ProviderPrometheus,
		Query:    `max by (namespace, deployment) (kube_deployment_status_replicas_unavailable)`,
		Lookback: "5m",
		Threshold: metrics.ThresholdConfig{
			Operator: ">",
			Value:    threshold,
		},
		Target: metrics.TargetConfig{
			Resolver:       metrics.ResolverKubernetesDeployment,
			NamespaceLabel: "namespace",
			NameLabel:      "deployment",
		},
	}, nil
}

func buildKubernetesDeploymentAvailabilityGap(params map[string]any) (metrics.RuleConfig, error) {
	threshold := metricsGetFloatParam(params, "threshold", 1)

	return metrics.RuleConfig{
		Provider: metrics.ProviderPrometheus,
		Query:    `max by (namespace, deployment) (kube_deployment_spec_replicas - kube_deployment_status_replicas_available)`,
		Lookback: "5m",
		Threshold: metrics.ThresholdConfig{
			Operator: ">",
			Value:    threshold,
		},
		Target: metrics.TargetConfig{
			Resolver:       metrics.ResolverKubernetesDeployment,
			NamespaceLabel: "namespace",
			NameLabel:      "deployment",
		},
	}, nil
}

func buildKubernetesPodHighCPUUsage(params map[string]any) (metrics.RuleConfig, error) {
	window := metricsGetStringParam(params, "window", "5m")
	threshold := metricsGetFloatParam(params, "threshold", 0.8)

	query := fmt.Sprintf(`sum by (namespace, pod) (rate(container_cpu_usage_seconds_total{pod!="",container!="",container!="POD"}[%s]))`, window)

	return metrics.RuleConfig{
		Provider: metrics.ProviderPrometheus,
		Query:    query,
		Lookback: window,
		Threshold: metrics.ThresholdConfig{
			Operator: ">",
			Value:    threshold,
		},
		Target: metrics.TargetConfig{
			Resolver:       metrics.ResolverKubernetesPod,
			NamespaceLabel: "namespace",
			NameLabel:      "pod",
		},
	}, nil
}

func buildKubernetesPodHighMemoryWorkingSet(params map[string]any) (metrics.RuleConfig, error) {
	threshold := metricsGetFloatParam(params, "threshold", 1500000000)

	return metrics.RuleConfig{
		Provider: metrics.ProviderPrometheus,
		Query:    `sum by (namespace, pod) (container_memory_working_set_bytes{pod!="",container!="",container!="POD"})`,
		Lookback: "5m",
		Threshold: metrics.ThresholdConfig{
			Operator: ">",
			Value:    threshold,
		},
		Target: metrics.TargetConfig{
			Resolver:       metrics.ResolverKubernetesPod,
			NamespaceLabel: "namespace",
			NameLabel:      "pod",
		},
	}, nil
}

func buildKubernetesPodCPUThrottlingHigh(params map[string]any) (metrics.RuleConfig, error) {
	window := metricsGetStringParam(params, "window", "5m")
	threshold := metricsGetFloatParam(params, "threshold", 0.2)

	query := fmt.Sprintf(`sum by (namespace, pod) (rate(container_cpu_cfs_throttled_periods_total{pod!="",container!="",container!="POD"}[%s])) / clamp_min(sum by (namespace, pod) (rate(container_cpu_cfs_periods_total{pod!="",container!="",container!="POD"}[%s])), 1e-9)`, window, window)

	return metrics.RuleConfig{
		Provider: metrics.ProviderPrometheus,
		Query:    query,
		Lookback: window,
		Threshold: metrics.ThresholdConfig{
			Operator: ">",
			Value:    threshold,
		},
		Target: metrics.TargetConfig{
			Resolver:       metrics.ResolverKubernetesPod,
			NamespaceLabel: "namespace",
			NameLabel:      "pod",
		},
	}, nil
}

func buildKubernetesPodOOMEvents(params map[string]any) (metrics.RuleConfig, error) {
	window := metricsGetStringParam(params, "window", "10m")

	query := fmt.Sprintf(`sum by (namespace, pod) (increase(container_oom_events_total{pod!=""}[%s]))`, window)

	return metrics.RuleConfig{
		Provider: metrics.ProviderPrometheus,
		Query:    query,
		Lookback: window,
		Threshold: metrics.ThresholdConfig{
			Operator: ">",
			Value:    0,
		},
		Target: metrics.TargetConfig{
			Resolver:       metrics.ResolverKubernetesPod,
			NamespaceLabel: "namespace",
			NameLabel:      "pod",
		},
	}, nil
}

func buildKubernetesPodUnschedulable(params map[string]any) (metrics.RuleConfig, error) {
	lookback := metricsGetStringParam(params, "lookback", "5m")

	query := fmt.Sprintf(`max by (namespace, pod) (max_over_time(kube_pod_status_unschedulable[%s]))`, lookback)

	return metrics.RuleConfig{
		Provider: metrics.ProviderPrometheus,
		Query:    query,
		Lookback: lookback,
		Threshold: metrics.ThresholdConfig{
			Operator: ">",
			Value:    0,
		},
		Target: metrics.TargetConfig{
			Resolver:       metrics.ResolverKubernetesPod,
			NamespaceLabel: "namespace",
			NameLabel:      "pod",
		},
	}, nil
}

func buildKubernetesPVCLowFreeSpace(params map[string]any) (metrics.RuleConfig, error) {
	threshold := metricsGetFloatParam(params, "threshold", 0.10)
	selector := buildPVCSelector(params)

	query := fmt.Sprintf(`(kubelet_volume_stats_available_bytes%s / clamp_min(kubelet_volume_stats_capacity_bytes%s, 1))`, selector, selector)

	return metrics.RuleConfig{
		Provider: metrics.ProviderPrometheus,
		Query:    query,
		Lookback: "5m",
		Threshold: metrics.ThresholdConfig{
			Operator: "<",
			Value:    threshold,
		},
		Target: metrics.TargetConfig{
			Resolver:       metrics.ResolverKubernetesPVC,
			NamespaceLabel: "namespace",
			NameLabel:      "persistentvolumeclaim",
		},
	}, nil
}

func buildKubernetesPVCFreeSpaceBurnRate(params map[string]any) (metrics.RuleConfig, error) {
	threshold := metricsGetFloatParam(params, "threshold", 0.10)
	window := metricsGetStringParam(params, "window", "6h")
	horizonSeconds := metricsGetFloatParam(params, "horizon_seconds", 86400)
	selector := buildPVCSelector(params)

	query := fmt.Sprintf(`(predict_linear(kubelet_volume_stats_available_bytes%s[%s], %v) / clamp_min(kubelet_volume_stats_capacity_bytes%s, 1))`, selector, window, horizonSeconds, selector)

	return metrics.RuleConfig{
		Provider: metrics.ProviderPrometheus,
		Query:    query,
		Lookback: window,
		Threshold: metrics.ThresholdConfig{
			Operator: "<",
			Value:    threshold,
		},
		Target: metrics.TargetConfig{
			Resolver:       metrics.ResolverKubernetesPVC,
			NamespaceLabel: "namespace",
			NameLabel:      "persistentvolumeclaim",
		},
	}, nil
}

func buildKubernetesPVCInodeLowFree(params map[string]any) (metrics.RuleConfig, error) {
	threshold := metricsGetFloatParam(params, "threshold", 0.10)
	selector := buildPVCSelector(params)

	query := fmt.Sprintf(`(kubelet_volume_stats_inodes_free%s / clamp_min(kubelet_volume_stats_inodes%s, 1))`, selector, selector)

	return metrics.RuleConfig{
		Provider: metrics.ProviderPrometheus,
		Query:    query,
		Lookback: "5m",
		Threshold: metrics.ThresholdConfig{
			Operator: "<",
			Value:    threshold,
		},
		Target: metrics.TargetConfig{
			Resolver:       metrics.ResolverKubernetesPVC,
			NamespaceLabel: "namespace",
			NameLabel:      "persistentvolumeclaim",
		},
	}, nil
}

func buildKubernetesStatefulSetPVCLowFreeSpace(params map[string]any) (metrics.RuleConfig, error) {
	threshold := metricsGetFloatParam(params, "threshold", 0.10)
	namespace := metricsGetStringParam(params, "namespace", "")
	statefulSet := metricsGetStringParam(params, "statefulset", "")
	volumeClaimPrefix := metricsGetStringParam(params, "volume_claim_prefix", "")

	selectorParts := []string{}
	if namespace != "" {
		selectorParts = append(selectorParts, fmt.Sprintf(`namespace="%s"`, namespace))
	}
	if statefulSet != "" {
		if volumeClaimPrefix != "" {
			selectorParts = append(selectorParts, fmt.Sprintf(`persistentvolumeclaim=~"%s-%s-[0-9]+"`, volumeClaimPrefix, statefulSet))
		} else {
			selectorParts = append(selectorParts, fmt.Sprintf(`persistentvolumeclaim=~".*-%s-[0-9]+"`, statefulSet))
		}
	}

	selector := ""
	if len(selectorParts) > 0 {
		selector = fmt.Sprintf("{%s}", strings.Join(selectorParts, ","))
	}

	query := fmt.Sprintf(`(kubelet_volume_stats_available_bytes%s / clamp_min(kubelet_volume_stats_capacity_bytes%s, 1))`, selector, selector)

	return metrics.RuleConfig{
		Provider: metrics.ProviderPrometheus,
		Query:    query,
		Lookback: "5m",
		Threshold: metrics.ThresholdConfig{
			Operator: "<",
			Value:    threshold,
		},
		Target: metrics.TargetConfig{
			Resolver:       metrics.ResolverKubernetesPVC,
			NamespaceLabel: "namespace",
			NameLabel:      "persistentvolumeclaim",
		},
	}, nil
}

func buildPVCSelector(params map[string]any) string {
	selectorParts := []string{}
	namespace := metricsGetStringParam(params, "namespace", "")
	if namespace != "" {
		selectorParts = append(selectorParts, fmt.Sprintf(`namespace="%s"`, namespace))
	}

	pvcRegex := metricsGetStringParam(params, "pvc_regex", "")
	if pvcRegex != "" {
		selectorParts = append(selectorParts, fmt.Sprintf(`persistentvolumeclaim=~"%s"`, pvcRegex))
	}

	if len(selectorParts) == 0 {
		return ""
	}

	return fmt.Sprintf("{%s}", strings.Join(selectorParts, ","))
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
