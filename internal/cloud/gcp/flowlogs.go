package gcp

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	cloudutils "github.com/coordimap/agent/internal/cloud/utils"
	"github.com/coordimap/agent/pkg/utils"

	"github.com/coordimap/agent/pkg/domain/agent"
	gcpModel "github.com/coordimap/agent/pkg/domain/gcp"
	kube_model "github.com/coordimap/agent/pkg/domain/kubernetes"

	"google.golang.org/api/logging/v2"
)

func (crawler *gcpCrawler) getFlowLogsRelationships() ([]*agent.Element, error) {
	allFoundRelationships := []*agent.Element{}
	startTime := time.Now().UTC().Add(-4 * crawler.crawlInterval)
	endTime := startTime.Add(crawler.crawlInterval - 5*time.Second)

	timeFilter := fmt.Sprintf(`timestamp >= "%s" AND timestamp <= "%s"`,
		startTime.Format(time.RFC3339),
		endTime.Format(time.RFC3339))

	filter := fmt.Sprintf("logName=~\"compute.googleapis.com%%2Fvpc_flows\" AND %s", timeFilter)

	entries, errEntries := crawler.logClient.Entries.List(&logging.ListLogEntriesRequest{
		ResourceNames: []string{fmt.Sprintf("projects/%s", crawler.ConfiguredProjectID)},
		Filter:        filter,
	}).Do()
	if errEntries != nil {
		return nil, errEntries
	}

	for _, logEntry := range entries.Entries {
		var jsonPayload flowJSONStructure
		errUnmarshal := json.Unmarshal(logEntry.JsonPayload, &jsonPayload)

		if errUnmarshal != nil {
			continue
		}

		crawlTime, errCrawlTime := time.Parse(time.RFC3339, jsonPayload.StartTime)
		if errCrawlTime != nil {
			crawlTime = time.Now().UTC()
		}

		if jsonPayload.SrcInstance.VmName != "" && jsonPayload.DstInstance.VmName != "" {

			srcVmInternalID := cloudutils.CreateGCPInternalName(crawler.scopeID, jsonPayload.DstInstance.Zone, gcpModel.TypeVMInstance, jsonPayload.DstInstance.VmName)
			dstVmInternalID := cloudutils.CreateGCPInternalName(crawler.scopeID, jsonPayload.DstInstance.Zone, gcpModel.TypeVMInstance, jsonPayload.DstInstance.VmName)

			vmRel, errVmRel := utils.CreateRelationship(srcVmInternalID, dstVmInternalID, agent.RelationshipType, agent.GCPNetworkFlowTypeRelation, crawlTime)
			if errVmRel == nil {
				allFoundRelationships = append(allFoundRelationships, vmRel)
			}
		}

		// check for gke src and dest
		if jsonPayload.SrcGkeDetails.Cluster.ClusterName != "" && jsonPayload.DstGkeDetails.Cluster.ClusterName != "" {
			srcClusterUID, errSrcClusterUID := cloudutils.GetMappingValue(crawler.externalMappings, fmt.Sprintf("%s-%s", jsonPayload.SrcGkeDetails.Cluster.ClusterLocation, jsonPayload.SrcGkeDetails.Cluster.ClusterName))
			if errSrcClusterUID != nil {
				continue
			}
			dstClusterUID, errDstClusterUID := cloudutils.GetMappingValue(crawler.externalMappings, fmt.Sprintf("%s-%s", jsonPayload.DstGkeDetails.Cluster.ClusterLocation, jsonPayload.DstGkeDetails.Cluster.ClusterName))
			if errDstClusterUID != nil {
				continue
			}

			if jsonPayload.SrcGkeDetails.Pod.Name != "" && jsonPayload.SrcGkeDetails.Pod.Namespace != "" && jsonPayload.DstGkeDetails.Pod.Namespace != "" && jsonPayload.DstGkeDetails.Pod.Name != "" {
				srcPodInternalName := cloudutils.CreateKubeInternalName(srcClusterUID, jsonPayload.SrcGkeDetails.Pod.Namespace, kube_model.TypePod, jsonPayload.SrcGkeDetails.Pod.Name)
				dstPodInternalName := cloudutils.CreateKubeInternalName(dstClusterUID, jsonPayload.DstGkeDetails.Pod.Namespace, kube_model.TypePod, jsonPayload.DstGkeDetails.Pod.Name)

				rel, errRel := utils.CreateRelationship(srcPodInternalName, dstPodInternalName, agent.RelationshipExternalBothSidesType, agent.GCPNetworkFlowTypeRelation, crawlTime)
				if errRel == nil {
					allFoundRelationships = append(allFoundRelationships, rel)
				}
			}

			if jsonPayload.SrcGkeDetails.Pod.Workload.Type == "DEPLOYMENT" && jsonPayload.DstGkeDetails.Pod.Workload.Type == "DEPLOYMENT" {
				srcDeployment := cloudutils.CreateKubeInternalName(srcClusterUID, jsonPayload.SrcGkeDetails.Pod.Namespace, kube_model.TypeDeployment, jsonPayload.SrcGkeDetails.Pod.Workload.Name)
				dstDeployment := cloudutils.CreateKubeInternalName(dstClusterUID, jsonPayload.DstGkeDetails.Pod.Namespace, kube_model.TypeDeployment, jsonPayload.DstGkeDetails.Pod.Workload.Name)

				rel, errRel := utils.CreateRelationship(srcDeployment, dstDeployment, agent.RelationshipExternalBothSidesType, agent.GCPNetworkFlowTypeRelation, crawlTime)
				if errRel == nil {
					allFoundRelationships = append(allFoundRelationships, rel)
				}
			}
			// TODO: check for service relationships
		}

		// check for the SQL mapping
		sqlPorts := []int{5432, 3306}
		if (slices.Contains(sqlPorts, jsonPayload.Connection.SrcPort) || slices.Contains(sqlPorts, jsonPayload.Connection.DstPort)) && (jsonPayload.DstGkeDetails.Cluster.ClusterName == "" || jsonPayload.SrcGkeDetails.Cluster.ClusterName == "") {
			sqlIP := jsonPayload.Connection.SrcIP
			if slices.Contains(sqlPorts, jsonPayload.Connection.DstPort) {
				sqlIP = jsonPayload.Connection.DstIP
			}
			sqlInternalName, existsSqlInternalIP := crawler.internalIDMapper[sqlIP]
			if !existsSqlInternalIP {
				continue
			}

			// relationship between the deployment and pod and the sql instance
			gkeDetails := []GkeDetails{jsonPayload.SrcGkeDetails, jsonPayload.DstGkeDetails}
			for index, gke := range gkeDetails {
				if gke.Pod.Name == "" || gke.Pod.Workload.Type != "DEPLOYMENT" {
					continue
				}
				clusterUID, errClusterUID := cloudutils.GetMappingValue(crawler.externalMappings, fmt.Sprintf("%s-%s", gke.Cluster.ClusterLocation, gke.Cluster.ClusterName))
				if errClusterUID != nil {
					continue
				}

				podInternalName := cloudutils.CreateKubeInternalName(clusterUID, gke.Pod.Namespace, kube_model.TypePod, gke.Pod.Name)
				deplomentInternalName := cloudutils.CreateKubeInternalName(clusterUID, gke.Pod.Namespace, kube_model.TypeDeployment, gke.Pod.Workload.Name)
				if index == 0 {
					relPodSql, errRelPodSql := utils.CreateRelationship(podInternalName, sqlInternalName, agent.RelationshipExternalSourceSideType, agent.GCPNetworkFlowTypeRelation, crawlTime)
					if errRelPodSql == nil {
						allFoundRelationships = append(allFoundRelationships, relPodSql)
					}
					relDeploymentSql, errRelDeploymentSql := utils.CreateRelationship(deplomentInternalName, sqlInternalName, agent.RelationshipExternalSourceSideType, agent.GCPNetworkFlowTypeRelation, crawlTime)
					if errRelDeploymentSql == nil {
						allFoundRelationships = append(allFoundRelationships, relDeploymentSql)
					}
				} else {
					relPodSql, errRelPodSql := utils.CreateRelationship(sqlInternalName, podInternalName, agent.RelationshipExternalDestinationSideType, agent.GCPNetworkFlowTypeRelation, crawlTime)
					if errRelPodSql == nil {
						allFoundRelationships = append(allFoundRelationships, relPodSql)
					}
					relDeploymentSql, errRelDeploymentSql := utils.CreateRelationship(sqlInternalName, deplomentInternalName, agent.RelationshipExternalDestinationSideType, agent.GCPNetworkFlowTypeRelation, crawlTime)
					if errRelDeploymentSql == nil {
						allFoundRelationships = append(allFoundRelationships, relDeploymentSql)
					}
				}
			}

			// relationship between the node and the sql instance
			instancesDetails := []InstanceDetails{jsonPayload.SrcInstance, jsonPayload.DstInstance}
			for index, instance := range instancesDetails {
				if instance.VmName == "" {
					continue
				}

				instanceInternalName := cloudutils.CreateGCPInternalName(crawler.scopeID, instance.Zone, gcpModel.TypeVMInstance, instance.VmName)
				if index == 0 {
					utils.AddRelationship(&allFoundRelationships, sqlInternalName, instanceInternalName, agent.GCPNetworkFlowTypeRelation, crawlTime)
				} else if index == 1 {
					utils.AddRelationship(&allFoundRelationships, instanceInternalName, sqlInternalName, agent.GCPNetworkFlowTypeRelation, crawlTime)
				}
			}
		}
	}

	return allFoundRelationships, nil
}
