package kubernetes

import (
	"time"

	"github.com/coordimap/agent/internal/metrics"
	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/prometheus/client_golang/api"
	"k8s.io/client-go/kubernetes"
)

const defaultCrawlTime = 30 * time.Second

const (
	kubeConfigInCluster            = "in_cluster"
	kubeConfigConfigFile           = "config_file"
	kubeConfigCrawlInterval        = "crawl_interval"
	kubeConfigIstioPrometheusHost  = "prometheus_host"
	kubeConfigClusterName          = "cluster_name"
	kubeConfigRetinaPrometheusHost = "retina_prometheus"
	kubeConfigMetricPrometheusHost = "metrics_prometheus_host"
	kubeConfigMetricRules          = metrics.ConfigMetricRules
	kubeConfigCloudDataSourceID    = "cloud_data_source_id"
	kubeConfigExternalMappings     = "external_mappings"
	kubeConfigScopeID              = "scope_id"
	kubeConfigSendSecretData       = "send_secret_data"
	kubeConfigSendConfigMapData    = "send_configmap_data"
)

type kubernetesCrawler struct {
	retinaCrawler     *prometheusCrawler
	kubeClient        *kubernetes.Clientset
	outputChannel     chan *agent.CloudCrawlData
	dataSource        agent.DataSource
	istioCrawler      prometheusCrawler
	internalNodeNames map[string]string
	externalMappings  map[string]string
	clusterName       string
	clusterUID        string
	cloudDataSourceID string
	crawlInterval     time.Duration
	istioConfigured   bool
	metricRules       []metrics.RuleConfig
	metricPromCrawler *prometheusCrawler
	sendSecretData    bool
	sendConfigMapData bool
}

type prometheusCrawler struct {
	Host          string
	promClient    api.Client
	promQueryTime string
}

type Crawler interface {
	Crawl()
}
