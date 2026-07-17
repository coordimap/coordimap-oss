package configuration_test

import (
	"os"
	"reflect"
	"testing"

	configuration "github.com/coordimap/agent/internal/config"
	"github.com/coordimap/agent/internal/metrics"
	"github.com/coordimap/agent/pkg/domain/agent"
)

func TestNewYamlFileConfig(t *testing.T) {
	// Create a temporary file for testing
	tmpfile, err := os.CreateTemp("", "config_test_*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name()) // clean up

	content := []byte(`
coordimap:
  api_key: test_key
  data_sources: []
`)
	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	type args struct {
		filePath string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "valid file",
			args:    args{filePath: tmpfile.Name()},
			wantErr: false,
		},
		{
			name:    "non-existent file",
			args:    args{filePath: "non_existent_file.yaml"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := configuration.NewYamlFileConfig(tt.args.filePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewYamlFileConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestNewYamlStringConfig(t *testing.T) {
	type args struct {
		yamlContent string
	}
	tests := []struct {
		name    string
		args    args
		want    *configuration.CoordimapConfig
		wantErr bool
	}{
		{
			name: "valid config",
			want: &configuration.CoordimapConfig{
				Coordimap: configuration.Coordimap{
					APIKey: "123",
					DataSources: []configuration.CoordimapConfigDataSource{
						{
							Type: "aws",
							ID:   "aws1",
							Config: []configuration.CoordimapConfigNameValueConfig{
								{
									Name:  "policy_config",
									Value: "true",
								},
							},
						},
						{
							Type: "postgres",
							ID:   "post1",
							Config: []configuration.CoordimapConfigNameValueConfig{
								{
									Name:  "db_name",
									Value: "dbname1",
								},
								{
									Name:  "db_host",
									Value: "host1",
								},
								{
									Name:  "db_user",
									Value: "user1",
								},
								{
									Name:  "db_pass",
									Value: "pass1",
								},
							},
						},
					},
				},
			},
			wantErr: false,
			args: args{yamlContent: `coordimap:
  api_key: 123
  data_sources:
    - type: aws
      id: aws1
      config:
      - name: policy_config
        value: "true"
    - type: postgres
      id: post1
      config:
        - name: db_name
          value: dbname1
        - name: db_host
          value: host1
        - name: db_user
          value: user1
        - name: db_pass
          value: pass1`},
		},
		{
			name:    "missing api key",
			want:    nil,
			wantErr: true,
			args: args{yamlContent: `coordimap:
  data_sources: []`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := configuration.NewYamlStringConfig(tt.args.yamlContent)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewYamlStringConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewYamlStringConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAllDataSourcesDatasourceLocalMetricRules(t *testing.T) {
	runtimeConfig, errRuntimeConfig := configuration.NewYamlFileConfig(createTempConfigFile(t, `coordimap:
  api_key: 123
  data_sources:
    - type: kubernetes
      id: kube-1
      config: []
      metric_rules:
        - id: k8s-errors
          provider: prometheus
          mode: custom
          custom:
            query: up
          threshold:
            operator: ">"
            value: 1
          target:
            resolver: kubernetes_service
    - type: gcp
      id: gcp-1
      config: []
      metric_rules:
        - id: gcp-cpu
          provider: gcp_monitoring
          mode: predefined
          predefined:
            name: cloudsql_high_cpu
            params:
              lookback: 6m
              threshold: 0.9
`))
	if errRuntimeConfig != nil {
		t.Fatalf("NewYamlFileConfig() unexpected error: %v", errRuntimeConfig)
	}

	allDataSources := runtimeConfig.GetAllDataSources()
	kubeDS := allDataSources["kubernetes"][0]
	gcpDS := allDataSources["gcp"][0]

	kubeRules := kubeDS.Config.MetricRules
	if len(kubeRules) != 1 {
		t.Fatalf("kubernetes metric rules len = %d, want 1", len(kubeRules))
	}

	if kubeRules[0].Provider != metrics.ProviderPrometheus {
		t.Fatalf("kubernetes provider = %q, want %q", kubeRules[0].Provider, metrics.ProviderPrometheus)
	}

	gcpRules := gcpDS.Config.MetricRules
	if len(gcpRules) != 1 {
		t.Fatalf("gcp metric rules len = %d, want 1", len(gcpRules))
	}

	if gcpRules[0].Provider != metrics.ProviderGCPMonitoring {
		t.Fatalf("gcp provider = %q, want %q", gcpRules[0].Provider, metrics.ProviderGCPMonitoring)
	}

	if gcpRules[0].MetricType != "cloudsql.googleapis.com/database/cpu/utilization" {
		t.Fatalf("gcp metric type = %q", gcpRules[0].MetricType)
	}

	if gcpRules[0].Lookback != "6m" {
		t.Fatalf("gcp lookback = %q, want 6m", gcpRules[0].Lookback)
	}

	for _, ds := range []*agent.DataSource{kubeDS, gcpDS} {
		for _, kv := range ds.Config.ValuePairs {
			if kv.Key == metrics.ConfigMetricRules {
				t.Fatalf("datasource %s has metric_rules in value_pairs; want typed data_source_config.metric_rules only", ds.DataSourceID)
			}
		}
	}
}

func TestNewYamlStringConfigMetricRulesUnsupportedDataSource(t *testing.T) {
	_, errConfig := configuration.NewYamlStringConfig(`coordimap:
  api_key: 123
  data_sources:
    - type: postgres
      id: pg-1
      config: []
      metric_rules:
        - id: invalid-rule
          provider: prometheus
          mode: custom
          custom:
            query: up
          threshold:
            operator: ">"
            value: 1
          target:
            resolver: kubernetes_service
`)
	if errConfig == nil {
		t.Fatalf("NewYamlStringConfig() expected unsupported data source metric rule error")
	}
}

func TestNewYamlStringConfigMetricRulesModeValidation(t *testing.T) {
	_, errConfig := configuration.NewYamlStringConfig(`coordimap:
  api_key: 123
  data_sources:
    - type: kubernetes
      id: kube-1
      config: []
      metric_rules:
        - id: invalid-mode-rule
          provider: prometheus
          mode: unknown
          custom:
            query: up
          threshold:
            operator: ">"
            value: 1
          target:
            resolver: kubernetes_service
`)
	if errConfig == nil {
		t.Fatalf("NewYamlStringConfig() expected metric rule mode validation error")
	}
}

func createTempConfigFile(t *testing.T, content string) string {
	t.Helper()

	tmpFile, errTempFile := os.CreateTemp("", "config_metric_rules_*.yaml")
	if errTempFile != nil {
		t.Fatalf("CreateTemp() error = %v", errTempFile)
	}

	if _, errWrite := tmpFile.WriteString(content); errWrite != nil {
		t.Fatalf("WriteString() error = %v", errWrite)
	}

	if errClose := tmpFile.Close(); errClose != nil {
		t.Fatalf("Close() error = %v", errClose)
	}

	t.Cleanup(func() {
		_ = os.Remove(tmpFile.Name())
	})

	return tmpFile.Name()
}
