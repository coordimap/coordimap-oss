// Package crawl starts configured crawler producers exactly once.
package crawl

import (
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	cloudutils "github.com/coordimap/agent/internal/cloud/utils"
	"github.com/coordimap/agent/internal/integrations"
	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/coordimap/agent/pkg/utils"
)

// Runner owns the configured crawler lifecycle and its shared output channel.
type Runner struct {
	dataSources map[string][]*agent.DataSource
	sender      chan *agent.CloudCrawlData

	mu      sync.Mutex
	started bool
	runID   string
}

// NewRunner creates a crawler runner using the existing integration factory.
func NewRunner(dataSources map[string][]*agent.DataSource, sender chan *agent.CloudCrawlData) *Runner {
	return &Runner{dataSources: dataSources, sender: sender}
}

// Start validates mappings and starts every configured crawler once.
func (r *Runner) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return nil
	}
	if err := ValidateKubernetesScopeMappings(r.dataSources); err != nil {
		return err
	}

	r.runID = uuid.NewString()
	r.started = true
	for integrationName, dataSources := range r.dataSources {
		for _, dataSource := range dataSources {
			log.Info().Msgf("Loading crawler for %s:%s", integrationName, dataSource.DataSourceID)
			crawler, err := integrations.IntegrationsFactory(integrationName, dataSource, r.sender)
			if err != nil {
				log.Info().Msgf("Could not create Crawler for integration: %s. The error was: %s", integrationName, err)
				continue
			}
			go crawler.Crawl()
		}
	}
	return nil
}

// Run returns the stable run ID. Existing recurring crawlers are never started twice.
func (r *Runner) Run(dataSourceID string) (runID string, running bool, err error) {
	if dataSourceID != "" && !r.hasDataSource(dataSourceID) {
		return "", false, fmt.Errorf("unknown data source %q", dataSourceID)
	}

	r.mu.Lock()
	alreadyStarted := r.started
	r.mu.Unlock()
	if err := r.Start(); err != nil {
		return "", false, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runID, alreadyStarted, nil
}

func (r *Runner) hasDataSource(id string) bool {
	for _, dataSources := range r.dataSources {
		for _, dataSource := range dataSources {
			if dataSource.DataSourceID == id {
				return true
			}
		}
	}
	return false
}

// ValidateKubernetesScopeMappings validates external mappings against Kubernetes scope IDs.
func ValidateKubernetesScopeMappings(allDataSources map[string][]*agent.DataSource) error {
	kubernetesDataSources := allDataSources[integrations.INTEGRATION_KUBERNETES]
	if len(kubernetesDataSources) == 0 {
		return nil
	}

	kubeDataSourceIDToClusterUID := map[string]string{}
	for _, dataSource := range kubernetesDataSources {
		clusterUID := dataSourceConfigValue(dataSource, "scope_id")
		if clusterUID == "" {
			continue
		}
		kubeDataSourceIDToClusterUID[dataSource.DataSourceID] = clusterUID
	}
	if len(kubeDataSourceIDToClusterUID) == 0 {
		return nil
	}

	validationErrors := validateExternalMappingsForKubernetesScopes(allDataSources[integrations.INTEGRATION_GCP], integrations.INTEGRATION_GCP, kubeDataSourceIDToClusterUID)
	validationErrors = append(validationErrors, validateExternalMappingsForKubernetesScopes(allDataSources[integrations.INTEGRATION_EBPF_FLOWS], integrations.INTEGRATION_EBPF_FLOWS, kubeDataSourceIDToClusterUID)...)
	if len(validationErrors) == 0 {
		return nil
	}
	return fmt.Errorf("invalid external_mappings for kubernetes cluster scope:\n- %s", strings.Join(validationErrors, "\n- "))
}

func validateExternalMappingsForKubernetesScopes(dataSources []*agent.DataSource, integrationName string, kubeDataSourceIDToClusterUID map[string]string) []string {
	validationErrors := []string{}
	for _, dataSource := range dataSources {
		if integrationName == integrations.INTEGRATION_GCP && dataSourceConfigValue(dataSource, "gcp_flows") != "true" {
			continue
		}
		if integrationName == integrations.INTEGRATION_EBPF_FLOWS && dataSourceConfigValue(dataSource, "deployedAt") != "kubernetes" {
			continue
		}
		rawMappings := dataSourceConfigValue(dataSource, "external_mappings")
		if rawMappings == "" {
			continue
		}
		parsedMappings, err := cloudutils.SplitConfiguredMappings(rawMappings)
		if err != nil {
			continue
		}
		for mappingKey, mappingValue := range parsedMappings {
			expectedClusterUID, found := kubeDataSourceIDToClusterUID[mappingValue]
			if !found {
				continue
			}
			validationErrors = append(validationErrors, fmt.Sprintf("integration=%s data_source_id=%s mapping=%s@%s expected_cluster_uid=%s", integrationName, dataSource.DataSourceID, mappingKey, mappingValue, expectedClusterUID))
		}
	}
	return validationErrors
}

func dataSourceConfigValue(dataSource *agent.DataSource, key string) string {
	for _, valuePair := range dataSource.Config.ValuePairs {
		if valuePair.Key != key {
			continue
		}
		value, err := utils.LoadValueFromEnvConfig(valuePair.Value)
		if err == nil {
			return value
		}
		return valuePair.Value
	}
	return ""
}
