package gcp

import (
	"time"

	"github.com/coordimap/agent/internal/metrics"
	"github.com/coordimap/agent/pkg/domain/agent"
	"google.golang.org/api/logging/v2"
	"google.golang.org/api/monitoring/v3"
	"google.golang.org/api/option"
)

const (
	gcpConfigInGoogleCloud    = "in_cloud"
	gcpConfigCredentialsFile  = "credentials_file"
	gcpConfigCrawlInterval    = "crawl_interval"
	gcpProjectID              = "project_id"
	gcpConfigFlows            = "gcp_flows"
	gcpConfigExternalMappings = "external_mappings"
	gcpConfigIncludeRegions   = "include_regions"
	gcpConfigScopeID          = "scope_id"
	gcpConfigMetricRules      = metrics.ConfigMetricRules
)

// ServiceAccountKey represents the complete structure of a Google Cloud service account key JSON file
type ServiceAccountKey struct {
	// Required fields that are always present in service account keys
	Type         string `json:"type"`           // Always "service_account"
	ProjectID    string `json:"project_id"`     // The GCP project ID
	PrivateKeyID string `json:"private_key_id"` // Unique identifier for the private key
	PrivateKey   string `json:"private_key"`    // The PEM-encoded private key
	ClientEmail  string `json:"client_email"`   // Service account email address
	ClientID     string `json:"client_id"`      // Unique identifier for the service account

	// Optional fields that might be present
	AuthURI                 string `json:"auth_uri"`                    // OAuth2 auth URI (usually https://accounts.google.com/o/oauth2/auth)
	TokenURI                string `json:"token_uri"`                   // OAuth2 token URI (usually https://oauth2.googleapis.com/token)
	AuthProviderX509CertURL string `json:"auth_provider_x509_cert_url"` // X509 cert URL for auth provider
	ClientX509CertURL       string `json:"client_x509_cert_url"`        // X509 cert URL for this service account
}

type gcpCrawler struct {
	outputChan          chan *agent.CloudCrawlData
	logClient           *logging.Service
	monitoringClient    *monitoring.Service
	clientOpts          []option.ClientOption
	dataSource          agent.DataSource
	externalMappings    map[string]string
	metricRules         []metrics.RuleConfig
	internalIDMapper    map[string]string
	cloudSQLZones       map[string]string
	ConfiguredProjectID string
	credentialsFile     string
	includedRegions     []string
	crawlInterval       time.Duration
	scopeID             string
	InGCPEnvironment    bool
}

type Crawler interface {
	Crawl()
	validateConfig() bool
}

type IpConnection struct {
	DstIP    string `json:"dest_ip"`
	SrcIP    string `json:"src_ip"`
	Protocol int    `json:"protocol"`
	SrcPort  int    `json:"src_port"`
	DstPort  int    `json:"dest_port"`
}

type GatewayDetails struct {
	ProjectID string     `json:"project_id"`
	Location  string     `json:"location"`
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	Vpc       VpcDetails `json:"vpc"`
}

type ClusterDetails struct {
	ClusterLocation string `json:"cluster_location"`
	ClusterName     string `json:"cluster_name"`
}

type PodDetails struct {
	Name      string          `json:"pod_name"`
	Namespace string          `json:"pod_namespace"`
	Workload  WorkloadDetails `json:"workload"`
}

type WorkloadDetails struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type ServiceDetails struct {
	Name      string `json:"service_name"`
	Namespace string `json:"service_namespace"`
}

type GkeDetails struct {
	Cluster ClusterDetails   `json:"cluster"`
	Pod     PodDetails       `json:"pod"`
	Service []ServiceDetails `json:"service"`
}

type GoogleService struct {
	Type string `json:"type"`
}

type InstanceDetails struct {
	ProjectID string `json:"project_id"`
	Region    string `json:"region"`
	VmName    string `json:"vm_name"`
	Zone      string `json:"zone"`
}

type VpcDetails struct {
	ProjectID    string `json:"project_id"`
	SubnetName   string `json:"subnetwork_name"`
	SubnetRegion string `json:"subnetwork_region"`
	VpcName      string `json:"vpc_name"`
}

type GeographicalDetails struct {
	City      string `json:"city"`
	Continent string `json:"continent"`
	Country   string `json:"country"`
	Region    string `json:"region"`
	Asn       int    `json:"asn"`
}

type flowJSONStructure struct {
	BytesSent        string              `json:"bytes_sent,omitempty"`
	PacketsSent      string              `json:"packets_sent,omitempty"`
	StartTime        string              `json:"start_time,omitempty"`
	EndTime          string              `json:"end_time,omitempty"`
	Connection       IpConnection        `json:"connection,omitempty"`
	SrcGateway       GatewayDetails      `json:"src_gateway,omitempty"`
	DstGateway       GatewayDetails      `json:"dest_gateway,omitempty"`
	SrcGkeDetails    GkeDetails          `json:"src_gke_details,omitempty"`
	DstGkeDetails    GkeDetails          `json:"dest_gke_details,omitempty"`
	SrcGoogleService GoogleService       `json:"src_google_service,omitempty"`
	DstGoogleService GoogleService       `json:"dest_google_service,omitempty"`
	SrcInstance      InstanceDetails     `json:"src_instance,omitempty"`
	DstInstance      InstanceDetails     `json:"dest_instance,omitempty"`
	SrcVpc           VpcDetails          `json:"src_vpc,omitempty"`
	DstVpc           VpcDetails          `json:"dest_vpc,omitempty"`
	SrcLocation      GeographicalDetails `json:"src_location,omitempty"`
	DstLocation      GeographicalDetails `json:"dest_location,omitempty"`
}
