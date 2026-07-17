package kubernetes

import (
	"testing"

	cloudutils "github.com/coordimap/agent/internal/cloud/utils"
	"github.com/coordimap/agent/internal/metrics"
	kubeModel "github.com/coordimap/agent/pkg/domain/kubernetes"
)

func TestResolveMetricInternalID(t *testing.T) {
	crawler := &kubernetesCrawler{
		clusterUID:       "cluster-uid-1",
		externalMappings: map[string]string{"sql:orders": "postgres/orders-public@"},
	}

	tests := []struct {
		name      string
		target    metrics.TargetConfig
		labels    map[string]string
		wantID    string
		wantFound bool
	}{
		{
			name: "service resolver",
			target: metrics.TargetConfig{
				Resolver:       metrics.ResolverKubernetesService,
				NamespaceLabel: "namespace",
				NameLabel:      "service",
			},
			labels:    map[string]string{"namespace": "payments", "service": "checkout"},
			wantID:    cloudutils.CreateKubeInternalName("cluster-uid-1", "payments", kubeModel.TypeService, "checkout"),
			wantFound: true,
		},
		{
			name: "deployment resolver",
			target: metrics.TargetConfig{
				Resolver:       metrics.ResolverKubernetesDeployment,
				NamespaceLabel: "namespace",
				NameLabel:      "deployment",
			},
			labels:    map[string]string{"namespace": "payments", "deployment": "api"},
			wantID:    cloudutils.CreateKubeInternalName("cluster-uid-1", "payments", kubeModel.TypeDeployment, "api"),
			wantFound: true,
		},
		{
			name: "pod resolver",
			target: metrics.TargetConfig{
				Resolver:       metrics.ResolverKubernetesPod,
				NamespaceLabel: "namespace",
				NameLabel:      "pod",
			},
			labels:    map[string]string{"namespace": "payments", "pod": "api-123"},
			wantID:    cloudutils.CreateKubeInternalName("cluster-uid-1", "payments", kubeModel.TypePod, "api-123"),
			wantFound: true,
		},
		{
			name: "pvc resolver",
			target: metrics.TargetConfig{
				Resolver:       metrics.ResolverKubernetesPVC,
				NamespaceLabel: "namespace",
				NameLabel:      "persistentvolumeclaim",
			},
			labels:    map[string]string{"namespace": "payments", "persistentvolumeclaim": "data-es-0"},
			wantID:    cloudutils.CreateKubeInternalName("cluster-uid-1", "payments", kubeModel.TypePVC, "data-es-0"),
			wantFound: true,
		},
		{
			name: "statefulset resolver",
			target: metrics.TargetConfig{
				Resolver:       metrics.ResolverKubernetesStatefulSet,
				NamespaceLabel: "namespace",
				NameLabel:      "statefulset",
			},
			labels:    map[string]string{"namespace": "payments", "statefulset": "elasticsearch"},
			wantID:    cloudutils.CreateKubeInternalName("cluster-uid-1", "payments", kubeModel.TypeStatefulSet, "elasticsearch"),
			wantFound: true,
		},
		{
			name: "external mapping resolver",
			target: metrics.TargetConfig{
				Resolver:           metrics.ResolverExternalMapping,
				MappingKeyTemplate: "sql:${label.instance}",
			},
			labels:    map[string]string{"instance": "orders"},
			wantID:    "postgres/orders-public@",
			wantFound: true,
		},
		{
			name: "missing mapping",
			target: metrics.TargetConfig{
				Resolver:           metrics.ResolverExternalMapping,
				MappingKeyTemplate: "sql:${label.instance}",
			},
			labels:    map[string]string{"instance": "unknown"},
			wantID:    "",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotFound := crawler.resolveMetricInternalID(tt.target, tt.labels)
			if gotFound != tt.wantFound {
				t.Fatalf("resolveMetricInternalID() found = %v, want %v", gotFound, tt.wantFound)
			}

			if gotID != tt.wantID {
				t.Fatalf("resolveMetricInternalID() id = %q, want %q", gotID, tt.wantID)
			}
		})
	}
}
