package utils

import (
	"fmt"
	"strings"

	gcpModel "github.com/coordimap/agent/pkg/domain/gcp"
	kube_model "github.com/coordimap/agent/pkg/domain/kubernetes"
)

func CreateGCPInternalName(scopeID, zone, assetType, name string) string {
	return fmt.Sprintf("%s-%s-%s-%s", scopeID, zone, assetType, name)
}

func CreateKubeInternalName(scopeID, namespace, assetType, name string) string {
	return fmt.Sprintf("%s-%s-%s-%s", scopeID, namespace, assetType, name)
}

func CreateAWSInternalID(scopeID string, awsElementID string) string {
	return fmt.Sprintf("%s@%s", scopeID, awsElementID)
}

// CreateSQLInternalName generate the internal name of the SQL server
// Examples:
// gcp:zone:name:dsid
// kube:namespace:podname:cluster_uid
// aws:rdsname:dsid
func CreateSQLInternalName(config string) (string, error) {
	configParts := strings.Split(config, ":")
	internalName := ""

	if configParts[0] == "gcp" && len(configParts) == 4 {
		internalName = CreateGCPInternalName(configParts[3], configParts[1], gcpModel.TypeCloudSQL, configParts[2])
	} else if strings.HasPrefix(config, "aws") {
	} else if configParts[0] == "kube" && len(configParts) == 4 {
		internalName = CreateKubeInternalName(configParts[3], configParts[1], kube_model.TypeNamespace, configParts[2])
	} else {
		return "", fmt.Errorf("wrong config %s", config)
	}

	return internalName, nil
}
