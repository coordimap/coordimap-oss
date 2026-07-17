package gcp

import (
	"testing"

	cloudutils "github.com/coordimap/agent/internal/cloud/utils"
	"github.com/coordimap/agent/internal/metrics"
	gcpModel "github.com/coordimap/agent/pkg/domain/gcp"
)

func TestResolveMetricInternalID(t *testing.T) {
	crawler := &gcpCrawler{
		scopeID:          "123456789012",
		externalMappings: map[string]string{"cloudsql:my-project:europe-west1:orders": "postgres/orders-public@"},
		cloudSQLZones: map[string]string{
			"orders":          "europe-west1-b",
			"ermal-test:prod": "europe-west3-b",
		},
	}

	tests := []struct {
		name           string
		target         metrics.TargetConfig
		metricLabels   map[string]string
		resourceLabels map[string]string
		wantID         string
		wantFound      bool
	}{
		{
			name: "gcp cloudsql resolver",
			target: metrics.TargetConfig{
				Resolver:    metrics.ResolverGCPCloudSQL,
				NameLabel:   "database_id",
				RegionLabel: "region",
			},
			resourceLabels: map[string]string{"database_id": "orders", "region": "europe-west1"},
			wantID:         cloudutils.CreateGCPInternalName("123456789012", "europe-west1-b", gcpModel.TypeCloudSQL, "orders"),
			wantFound:      true,
		},
		{
			name: "gcp cloudsql resolver normalizes project database id",
			target: metrics.TargetConfig{
				Resolver:    metrics.ResolverGCPCloudSQL,
				NameLabel:   "database_id",
				RegionLabel: "region",
			},
			resourceLabels: map[string]string{"database_id": "ermal-test:prod", "region": "europe-west3", "project_id": "ermal-test"},
			wantID:         cloudutils.CreateGCPInternalName("123456789012", "europe-west3-b", gcpModel.TypeCloudSQL, "prod"),
			wantFound:      true,
		},
		{
			name: "gcp cloudsql resolver missing zone mapping",
			target: metrics.TargetConfig{
				Resolver:    metrics.ResolverGCPCloudSQL,
				NameLabel:   "database_id",
				RegionLabel: "region",
			},
			resourceLabels: map[string]string{"database_id": "missing", "region": "europe-west1"},
			wantID:         "",
			wantFound:      false,
		},
		{
			name: "gcp vm instance resolver",
			target: metrics.TargetConfig{
				Resolver:  metrics.ResolverGCPVMInstance,
				NameLabel: "instance_name",
				ZoneLabel: "zone",
			},
			resourceLabels: map[string]string{"instance_name": "vm-1", "zone": "europe-west1-b"},
			wantID:         cloudutils.CreateGCPInternalName("123456789012", "europe-west1-b", gcpModel.TypeVMInstance, "vm-1"),
			wantFound:      true,
		},
		{
			name: "external mapping resolver",
			target: metrics.TargetConfig{
				Resolver:           metrics.ResolverExternalMapping,
				MappingKeyTemplate: "cloudsql:${resource.project_id}:${resource.region}:${resource.database_id}",
			},
			resourceLabels: map[string]string{"project_id": "my-project", "region": "europe-west1", "database_id": "orders"},
			wantID:         "postgres/orders-public@",
			wantFound:      true,
		},
		{
			name: "external mapping resolver missing key",
			target: metrics.TargetConfig{
				Resolver:           metrics.ResolverExternalMapping,
				MappingKeyTemplate: "cloudsql:${resource.project_id}:${resource.region}:${resource.database_id}",
			},
			resourceLabels: map[string]string{"project_id": "my-project", "region": "europe-west1", "database_id": "unknown"},
			wantID:         "",
			wantFound:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, found := crawler.resolveMetricInternalID(tt.target, tt.metricLabels, tt.resourceLabels)
			if found != tt.wantFound {
				t.Fatalf("resolveMetricInternalID() found = %v, want %v", found, tt.wantFound)
			}

			if gotID != tt.wantID {
				t.Fatalf("resolveMetricInternalID() id = %q, want %q", gotID, tt.wantID)
			}
		})
	}
}
