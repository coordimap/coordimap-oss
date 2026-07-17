package kubernetes

import (
	"errors"
	"fmt"
	"strings"

	cloudutils "github.com/coordimap/agent/internal/cloud/utils"
	gcpModel "github.com/coordimap/agent/pkg/domain/gcp"
	"github.com/prometheus/client_golang/api"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const gkeServiceAccountAnnotation = "iam.gke.io/gcp-service-account"

func makePrometheusCrawler(prometheusHost string) (prometheusCrawler, error) {
	crawler := prometheusCrawler{
		promClient: nil,
		Host:       prometheusHost,
	}

	client, err := api.NewClient(api.Config{
		Address: prometheusHost,
	})
	if err != nil {
		fmt.Printf("Error creating client: %v\n", err)
		return crawler, fmt.Errorf("could not connect to the prometheus client because %w", err)
	}

	crawler.promClient = client

	return crawler, nil
}

func connectToK8sFromConfigFile(configFilePath string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", configFilePath)
	if err != nil {
		return nil, fmt.Errorf("could not create Kubernetes config from file. %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("could not create Kubernetes clientset. %w", err)
	}

	return clientset, nil
}

func connectoToK8sInCluster() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("could not create inClusterConfig.%w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("could not create Kubernetes clientSet. %w", err)
	}

	return clientset, nil
}

func clearManagedFields(item *metav1.ObjectMeta) {
	item.ManagedFields = []metav1.ManagedFieldsEntry{}
}

func (kubeCrawler *kubernetesCrawler) sanitizeSecret(secret v1.Secret) v1.Secret {
	if kubeCrawler.sendSecretData {
		return secret
	}

	secret.Data = nil
	secret.StringData = nil
	return secret
}

func (kubeCrawler *kubernetesCrawler) sanitizeConfigMap(configMap v1.ConfigMap) v1.ConfigMap {
	if kubeCrawler.sendConfigMapData {
		return configMap
	}

	configMap.Data = nil
	configMap.BinaryData = nil
	return configMap
}

func getNodeCloud(labels map[string]string, annotations map[string]string, addresses []v1.NodeAddress) (string, error) {
	for _, address := range addresses {
		if strings.Contains(address.Address, "compute.internal") || strings.Contains(address.Address, "amazonaws") {
			return "aws", nil
		}
	}

	for key, value := range labels {
		if strings.Contains(key, "aws") || strings.Contains(value, "aws") {
			return "aws", nil
		}

		if strings.Contains(value, "google") || strings.Contains(key, "gke") || strings.Contains(value, "google") {
			return "gcp", nil
		}
	}

	for key, value := range annotations {
		if strings.Contains(key, "aws") || strings.Contains(value, "aws") {
			return "aws", nil
		}

		if strings.Contains(key, "cloud.google.com") || strings.Contains(value, "google") || strings.Contains(key, "gke") || strings.Contains(value, "google") {
			return "gcp", nil
		}
	}

	return "", errors.New("no cloud found")
}

func getNodeLocationLabel(labels map[string]string, primaryLabel, fallbackLabel string) string {
	if value := labels[primaryLabel]; value != "" {
		return value
	}

	return labels[fallbackLabel]
}

func resolveExternalNodeInternalName(node v1.Node, externalMappings map[string]string) (string, bool) {
	cloudName, errCloudName := getNodeCloud(node.Labels, node.Annotations, node.Status.Addresses)
	if errCloudName != nil || cloudName != "gcp" {
		return "", false
	}

	region := getNodeLocationLabel(node.Labels, "topology.kubernetes.io/region", "failure-domain.beta.kubernetes.io/region")
	zone := getNodeLocationLabel(node.Labels, "topology.kubernetes.io/zone", "failure-domain.beta.kubernetes.io/zone")
	if region == "" || zone == "" {
		return "", false
	}

	gcpDataSourceID, errGcpDataSourceID := cloudutils.GetMappingDataSourceID(externalMappings, fmt.Sprintf("%s-%s", region, node.Name))
	if errGcpDataSourceID != nil {
		return "", false
	}

	return cloudutils.CreateGCPInternalName(gcpDataSourceID, zone, gcpModel.TypeVMInstance, node.Name), true
}

func getGCPServiceAccountInternalID(gcpScopeID, gsaEmail string) string {
	return cloudutils.CreateGCPInternalName(gcpScopeID, "", gcpModel.TypeServiceAccount, gsaEmail)
}

func _sanitizeIAMName(value string) string {
	replacer := strings.NewReplacer("/", "_", ":", "_", "@", "_", " ", "_")
	return replacer.Replace(value)
}

const AppVersionLabel = "app.kubernetes.io/version"

// GetAppVersionFromLabels extracts the application version from the standard Kubernetes label.
// It returns the version string and a boolean indicating if the label was found.
func GetAppVersionFromLabels(labels map[string]string) (string, bool) {
	if labels == nil {
		return "", false
	}
	version, ok := labels[AppVersionLabel]
	return version, ok
}
