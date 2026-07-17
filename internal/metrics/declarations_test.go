package metrics_test

import (
	"strings"
	"testing"

	"github.com/coordimap/agent/internal/metrics"
	_ "github.com/coordimap/agent/internal/metrics/gcp"
	_ "github.com/coordimap/agent/internal/metrics/prometheus"
)

func TestResolveRuleDeclarationCustom(t *testing.T) {
	decl := metrics.RuleDeclaration{
		DataSourceID: "kube-1",
		ID:           "k8s-errors",
		Provider:     metrics.ProviderPrometheus,
		Mode:         metrics.RuleModeCustom,
		Custom: metrics.CustomRuleConfig{
			Query: "up",
		},
		Threshold: &metrics.ThresholdConfig{Operator: ">", Value: 1},
		Target: &metrics.TargetConfig{
			Resolver: metrics.ResolverKubernetesService,
		},
	}

	rule, errResolve := metrics.ResolveRuleDeclaration(decl)
	if errResolve != nil {
		t.Fatalf("ResolveRuleDeclaration() unexpected error: %v", errResolve)
	}

	if rule.Query != "up" {
		t.Fatalf("ResolveRuleDeclaration() query = %q, want up", rule.Query)
	}
}

func TestResolveRuleDeclarationPredefined(t *testing.T) {
	decl := metrics.RuleDeclaration{
		DataSourceType: "gcp",
		ID:             "gcp-cpu",
		Provider:       metrics.ProviderGCPMonitoring,
		Mode:           metrics.RuleModePredefined,
		Predefined: metrics.PredefinedRuleConfig{
			Name: "cloudsql_high_cpu",
			Params: map[string]any{
				"lookback":  "6m",
				"threshold": 0.9,
			},
		},
	}

	rule, errResolve := metrics.ResolveRuleDeclaration(decl)
	if errResolve != nil {
		t.Fatalf("ResolveRuleDeclaration() unexpected error: %v", errResolve)
	}

	if rule.MetricType != "cloudsql.googleapis.com/database/cpu/utilization" {
		t.Fatalf("ResolveRuleDeclaration() metric type = %q", rule.MetricType)
	}

	if rule.Lookback != "6m" {
		t.Fatalf("ResolveRuleDeclaration() lookback = %q, want 6m", rule.Lookback)
	}

	if rule.Threshold.Value != 0.9 {
		t.Fatalf("ResolveRuleDeclaration() threshold value = %v, want 0.9", rule.Threshold.Value)
	}
}

func TestResolveRuleDeclarationPredefinedPrometheusTemplates(t *testing.T) {
	tests := []struct {
		name             string
		template         string
		expectedQuerySub string
		expectedResolver string
	}{
		{
			name:             "deployment high 5xx",
			template:         "kubernetes_deployment_high_5xx",
			expectedQuerySub: "destination_workload",
			expectedResolver: metrics.ResolverKubernetesDeployment,
		},
		{
			name:             "service high latency",
			template:         "kubernetes_service_high_latency",
			expectedQuerySub: "histogram_quantile",
			expectedResolver: metrics.ResolverKubernetesService,
		},
		{
			name:             "pod high restart rate",
			template:         "kubernetes_pod_high_restart_rate",
			expectedQuerySub: "kube_pod_container_status_restarts_total",
			expectedResolver: metrics.ResolverKubernetesPod,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl := metrics.RuleDeclaration{
				DataSourceType: "kubernetes",
				ID:             "tmpl-" + tt.template,
				Provider:       metrics.ProviderPrometheus,
				Mode:           metrics.RuleModePredefined,
				Predefined: metrics.PredefinedRuleConfig{
					Name: tt.template,
				},
			}

			rule, errResolve := metrics.ResolveRuleDeclaration(decl)
			if errResolve != nil {
				t.Fatalf("ResolveRuleDeclaration() unexpected error: %v", errResolve)
			}

			if !strings.Contains(rule.Query, tt.expectedQuerySub) {
				t.Fatalf("ResolveRuleDeclaration() query = %q does not contain %q", rule.Query, tt.expectedQuerySub)
			}

			if rule.Target.Resolver != tt.expectedResolver {
				t.Fatalf("ResolveRuleDeclaration() resolver = %q, want %q", rule.Target.Resolver, tt.expectedResolver)
			}
		})
	}
}

func TestResolveRuleDeclarationPredefinedPrometheusKubernetesCoreTemplates(t *testing.T) {
	tests := []struct {
		name             string
		template         string
		expectedQuerySub string
		expectedResolver string
		invalidQuerySub  string
	}{
		{name: "crashloop imagepull", template: "kubernetes_pod_crashloop_or_imagepull_error", expectedQuerySub: "kube_pod_container_status_waiting_reason", expectedResolver: metrics.ResolverKubernetesPod},
		{name: "pod not ready", template: "kubernetes_pod_not_ready", expectedQuerySub: `max_over_time(kube_pod_status_ready{condition="true"}[5m])`, expectedResolver: metrics.ResolverKubernetesPod, invalidQuerySub: "max_over_time((1 -"},
		{name: "deployment unavailable replicas", template: "kubernetes_deployment_unavailable_replicas", expectedQuerySub: "kube_deployment_status_replicas_unavailable", expectedResolver: metrics.ResolverKubernetesDeployment},
		{name: "deployment availability gap", template: "kubernetes_deployment_availability_gap", expectedQuerySub: "kube_deployment_spec_replicas", expectedResolver: metrics.ResolverKubernetesDeployment},
		{name: "pod high cpu", template: "kubernetes_pod_high_cpu_usage", expectedQuerySub: "container_cpu_usage_seconds_total", expectedResolver: metrics.ResolverKubernetesPod},
		{name: "pod high memory", template: "kubernetes_pod_high_memory_workingset", expectedQuerySub: "container_memory_working_set_bytes", expectedResolver: metrics.ResolverKubernetesPod},
		{name: "pod cpu throttling", template: "kubernetes_pod_cpu_throttling_high", expectedQuerySub: "container_cpu_cfs_throttled_periods_total", expectedResolver: metrics.ResolverKubernetesPod},
		{name: "pod oom events", template: "kubernetes_pod_oom_events", expectedQuerySub: "container_oom_events_total", expectedResolver: metrics.ResolverKubernetesPod},
		{name: "pod unschedulable", template: "kubernetes_pod_unschedulable", expectedQuerySub: "kube_pod_status_unschedulable", expectedResolver: metrics.ResolverKubernetesPod},
		{name: "pvc low free space", template: "kubernetes_pvc_low_free_space", expectedQuerySub: "kubelet_volume_stats_available_bytes", expectedResolver: metrics.ResolverKubernetesPVC},
		{name: "pvc free space burn rate", template: "kubernetes_pvc_free_space_burn_rate", expectedQuerySub: "predict_linear", expectedResolver: metrics.ResolverKubernetesPVC},
		{name: "pvc inode low free", template: "kubernetes_inode_low_free", expectedQuerySub: "kubelet_volume_stats_inodes_free", expectedResolver: metrics.ResolverKubernetesPVC},
		{name: "statefulset pvc low free", template: "kubernetes_statefulset_pvc_low_free_space", expectedQuerySub: "kubelet_volume_stats_capacity_bytes", expectedResolver: metrics.ResolverKubernetesPVC},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl := metrics.RuleDeclaration{
				DataSourceType: "kubernetes",
				ID:             "tmpl-" + tt.template,
				Provider:       metrics.ProviderPrometheus,
				Mode:           metrics.RuleModePredefined,
				Predefined: metrics.PredefinedRuleConfig{
					Name: tt.template,
				},
			}

			rule, errResolve := metrics.ResolveRuleDeclaration(decl)
			if errResolve != nil {
				t.Fatalf("ResolveRuleDeclaration() unexpected error: %v", errResolve)
			}

			if !strings.Contains(rule.Query, tt.expectedQuerySub) {
				t.Fatalf("ResolveRuleDeclaration() query = %q does not contain %q", rule.Query, tt.expectedQuerySub)
			}

			if tt.invalidQuerySub != "" && strings.Contains(rule.Query, tt.invalidQuerySub) {
				t.Fatalf("ResolveRuleDeclaration() query = %q contains invalid PromQL pattern %q", rule.Query, tt.invalidQuerySub)
			}

			if rule.Target.Resolver != tt.expectedResolver {
				t.Fatalf("ResolveRuleDeclaration() resolver = %q, want %q", rule.Target.Resolver, tt.expectedResolver)
			}
		})
	}
}

func TestResolveRuleDeclarationPredefinedGCPTemplates(t *testing.T) {
	tests := []struct {
		name               string
		template           string
		expectedMetricType string
		expectedResolver   string
	}{
		{
			name:               "cloudsql high connections",
			template:           "cloudsql_high_connections",
			expectedMetricType: "cloudsql.googleapis.com/database/network/connections",
			expectedResolver:   metrics.ResolverGCPCloudSQL,
		},
		{
			name:               "vm high cpu",
			template:           "vm_high_cpu",
			expectedMetricType: "compute.googleapis.com/instance/cpu/utilization",
			expectedResolver:   metrics.ResolverGCPVMInstance,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl := metrics.RuleDeclaration{
				DataSourceType: "gcp",
				ID:             "tmpl-" + tt.template,
				Provider:       metrics.ProviderGCPMonitoring,
				Mode:           metrics.RuleModePredefined,
				Predefined: metrics.PredefinedRuleConfig{
					Name: tt.template,
				},
			}

			rule, errResolve := metrics.ResolveRuleDeclaration(decl)
			if errResolve != nil {
				t.Fatalf("ResolveRuleDeclaration() unexpected error: %v", errResolve)
			}

			if rule.MetricType != tt.expectedMetricType {
				t.Fatalf("ResolveRuleDeclaration() metric_type = %q, want %q", rule.MetricType, tt.expectedMetricType)
			}

			if rule.Target.Resolver != tt.expectedResolver {
				t.Fatalf("ResolveRuleDeclaration() resolver = %q, want %q", rule.Target.Resolver, tt.expectedResolver)
			}
		})
	}
}
