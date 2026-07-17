package gcp

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	cloudutils "github.com/coordimap/agent/internal/cloud/utils"
	"github.com/coordimap/agent/pkg/utils"

	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/coordimap/agent/pkg/domain/gcp"
	gcpModel "github.com/coordimap/agent/pkg/domain/gcp"
	"github.com/rs/zerolog/log"
	"google.golang.org/api/compute/v1"
	run "google.golang.org/api/run/v1"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"
	"google.golang.org/api/storage/v1"
)

func (gcpCrawler *gcpCrawler) GetLocationsAndZones(client *compute.Service, crawlTime time.Time) ([]*agent.Element, error) {
	allElems := []*agent.Element{}

	regionsCall := client.Regions.List(gcpCrawler.ConfiguredProjectID)
	regionList, err := regionsCall.Do()
	if err != nil {
		return allElems, fmt.Errorf("could not list regions becaue %v", err)
	}

	// Iterate through regions and their zones
	for _, region := range regionList.Items {
		if !slices.Contains(gcpCrawler.includedRegions, region.Name) {
			continue
		}

		regionInternalName := cloudutils.CreateGCPInternalName(gcpCrawler.scopeID, "", gcpModel.TypeRegion, region.Name)
		regionElem, errRegionElem := utils.CreateElement(region, region.Name, regionInternalName, gcpModel.TypeRegion, agent.StatusNoStatus, "", crawlTime)
		if errRegionElem == nil {
			allElems = append(allElems, regionElem)
		}

		zonesCall := client.Zones.List(gcpCrawler.ConfiguredProjectID)
		zoneList, err := zonesCall.Filter(fmt.Sprintf("name=%s*", region.Name)).Do()
		if err != nil {
			log.Printf("Error getting zones for region %s: %v", region.Name, err)
			continue
		}

		for _, zone := range zoneList.Items {
			zoneInternalName := cloudutils.CreateGCPInternalName(gcpCrawler.scopeID, "", gcpModel.TypeZone, zone.Name)
			zoneElem, errZoneElem := utils.CreateElement(zone, zone.Name, zoneInternalName, gcpModel.TypeZone, agent.StatusNoStatus, "", crawlTime)
			if errZoneElem == nil {
				allElems = append(allElems, zoneElem)
			}

			rel, errRel := utils.CreateRelationship(regionInternalName, zoneInternalName, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				allElems = append(allElems, rel)
			}
		}
	}
	return allElems, nil
}

func (gcpCrawler *gcpCrawler) GetBuckets(crawlTime time.Time) ([]*agent.Element, error) {
	allBucketElements := []*agent.Element{}

	client, err := storage.NewService(context.Background(), gcpCrawler.clientOpts...)
	if err != nil {
		return allBucketElements, fmt.Errorf("could not create storage client because %v", err)
	}

	buckets, errBuckets := client.Buckets.List(gcpCrawler.ConfiguredProjectID).Do()
	if errBuckets != nil {
		return allBucketElements, fmt.Errorf("could not retrieve all buckets because %v", errBuckets)
	}

	for _, bucket := range buckets.Items {
		zone := strings.ToLower(bucket.Location)
		zoneInternalName := cloudutils.CreateGCPInternalName(gcpCrawler.scopeID, "", gcpModel.TypeRegion, zone)
		bucketInternalName := cloudutils.CreateGCPInternalName(gcpCrawler.scopeID, zone, gcpModel.TypeBucket, bucket.Name)
		elem, errElem := utils.CreateElement(bucket, bucket.Name, bucketInternalName, gcpModel.TypeBucket, agent.StatusNoStatus, "", crawlTime)
		if errElem == nil {
			allBucketElements = append(allBucketElements, elem)
		}

		rel, errRel := utils.CreateRelationship(zoneInternalName, bucketInternalName, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			allBucketElements = append(allBucketElements, rel)
		}
	}

	return allBucketElements, nil
}

func (gcpCrawler *gcpCrawler) GetCloudRuns(crawlTime time.Time) ([]*agent.Element, error) {
	allCloudRuns := []*agent.Element{}

	client, errClient := run.NewService(context.Background(), gcpCrawler.clientOpts...)
	if errClient != nil {
		return allCloudRuns, fmt.Errorf("could not create a cloud run client because %v", errClient)
	}

	parent := fmt.Sprintf("projects/%s/locations/-", gcpCrawler.ConfiguredProjectID)
	services, errServices := client.Projects.Locations.Services.List(parent).Do()
	if errServices != nil {
		return allCloudRuns, fmt.Errorf("failed to list Cloud Run services: %v", errServices)
	}

	for _, service := range services.Items {
		cloudRunInternalID := cloudutils.CreateGCPInternalName(gcpCrawler.scopeID, "", gcp.TypeCloudRun, service.Metadata.Name)
		elem, errElem := utils.CreateElement(service, service.Metadata.Name, cloudRunInternalID, gcpModel.TypeCloudRun, agent.StatusNoStatus, service.Metadata.ResourceVersion, crawlTime)
		if errElem == nil {
			allCloudRuns = append(allCloudRuns, elem)
		}
	}

	return allCloudRuns, nil
}

func (gcpCrawler *gcpCrawler) GetComputeElems(crawlTime time.Time) ([]*agent.Element, error) {
	logger := log.With().Str("DataSourceType", "gcp").Str("ProjectID", gcpCrawler.ConfiguredProjectID).Str("DataSourceID", gcpCrawler.dataSource.DataSourceID).Logger()
	allComputeElems := []*agent.Element{}
	client, errClient := createComputeClient(gcpCrawler.clientOpts)
	if errClient != nil {
		return allComputeElems, fmt.Errorf("could not create a compute instance because %v", errClient)
	}

	regionsAndZones, errRegionsAndZones := gcpCrawler.GetLocationsAndZones(client, crawlTime)
	if errRegionsAndZones == nil {
		allComputeElems = append(allComputeElems, regionsAndZones...)
	}

	vmInstanceElems, errVMInstanceElems := gcpCrawler.GetVMInstances(client, crawlTime)
	if errVMInstanceElems != nil {
		logger.Err(errVMInstanceElems).Msg("could not retrieve VM instances")
	} else {
		allComputeElems = append(allComputeElems, vmInstanceElems...)
	}

	nodeGroupElems, errNodeGroupElems := gcpCrawler.getNodeGroups(client, crawlTime)
	if errNodeGroupElems != nil {
		logger.Err(errNodeGroupElems).Msg("could not retrieve node group")
	} else {
		allComputeElems = append(allComputeElems, nodeGroupElems...)
	}

	instanceGroupElems, errInstanceGroupElems := gcpCrawler.getInstanceGroups(client, crawlTime)
	if errInstanceGroupElems != nil {
		logger.Err(errInstanceGroupElems).Msg("could not retrieve instance groups")
	} else {
		allComputeElems = append(allComputeElems, instanceGroupElems...)
	}

	diskElems, errDiskElems := gcpCrawler.getDisks(client, crawlTime)
	if errDiskElems != nil {
		logger.Err(errDiskElems).Msg("could not retrieve disks")
	} else {
		allComputeElems = append(allComputeElems, diskElems...)
	}

	networkElems, errNetworkElems := gcpCrawler.getNetworks(client, crawlTime)
	if errNetworkElems != nil {
		logger.Err(errNetworkElems).Msg("could not retrieve networks")
	} else {
		allComputeElems = append(allComputeElems, networkElems...)
	}

	subnetworkElems, errSubnetworkElems := gcpCrawler.getSubNetworks(client, crawlTime)
	if errSubnetworkElems != nil {
		logger.Err(errSubnetworkElems).Msg("could not retrieve networks")
	} else {
		allComputeElems = append(allComputeElems, subnetworkElems...)
	}

	return allComputeElems, nil
}

func (gcpCrawler *gcpCrawler) GetVMInstances(client *compute.Service, crawlTime time.Time) ([]*agent.Element, error) {
	allVMInstanceElems := []*agent.Element{}

	instances, errInstances := client.Instances.AggregatedList(gcpCrawler.ConfiguredProjectID).Do()
	if errInstances != nil {
		return allVMInstanceElems, fmt.Errorf("could not retrieve the instances because %v", errInstances)
	}

	for scopedZone, list := range instances.Items {
		for _, instance := range list.Instances {
			zone := getZoneFromScopedZone(scopedZone)
			zoneInternalName := cloudutils.CreateGCPInternalName(gcpCrawler.scopeID, "", gcpModel.TypeZone, zone)
			instanceInternalID := cloudutils.CreateGCPInternalName(gcpCrawler.scopeID, zone, gcpModel.TypeVMInstance, instance.Name)

			utils.AddRelationship(&allVMInstanceElems, zoneInternalName, instanceInternalID, agent.ParentChildTypeRelation, crawlTime)

			instanceElem, errInstanceElem := utils.CreateElement(instance, instance.Name, instanceInternalID, gcpModel.TypeVMInstance, getComputeStatus(instance.Status), "", crawlTime)
			if errInstanceElem == nil {
				allVMInstanceElems = append(allVMInstanceElems, instanceElem)
			}

			for _, disk := range instance.Disks {
				split := strings.Split(disk.Source, "/")
				diskInternalID := cloudutils.CreateGCPInternalName(gcpCrawler.scopeID, zone, gcpModel.TypeDisk, split[len(split)-1])

				diskRel, errDiskRel := utils.CreateRelationship(instanceInternalID, diskInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errDiskRel == nil {
					allVMInstanceElems = append(allVMInstanceElems, diskRel)
				}
			}
		}
	}

	return allVMInstanceElems, nil
}

func (gcpCrawler *gcpCrawler) getNodeGroups(client *compute.Service, crawlTime time.Time) ([]*agent.Element, error) {
	allNodeGroups := []*agent.Element{}

	nodeGroups, errNodeGroups := client.NodeGroups.AggregatedList(gcpCrawler.ConfiguredProjectID).Do()
	if errNodeGroups != nil {
		return allNodeGroups, fmt.Errorf("could not get all node groups because %s", errNodeGroups)
	}

	for zone, list := range nodeGroups.Items {
		for _, nodeGroup := range list.NodeGroups {
			zoneInternalName := cloudutils.CreateGCPInternalName(gcpCrawler.scopeID, "", gcpModel.TypeZone, zone)
			nodeGroupInternalName := cloudutils.CreateGCPInternalName(gcpCrawler.scopeID, zone, gcpModel.TypeNodeGroup, nodeGroup.Name)
			nodeGroupElem, errNodeGroupElem := utils.CreateElement(nodeGroup, nodeGroup.Name, nodeGroupInternalName, gcpModel.TypeNodeGroup, getComputeStatus(nodeGroup.Status), "", crawlTime)
			if errNodeGroupElem == nil {
				allNodeGroups = append(allNodeGroups, nodeGroupElem)
			}

			utils.AddRelationship(&allNodeGroups, zoneInternalName, nodeGroupInternalName, agent.ParentChildTypeRelation, crawlTime)

			nodeGroupNodes, errNodeGroupNodes := client.NodeGroups.ListNodes(gcpCrawler.ConfiguredProjectID, zone, nodeGroup.Name).Do()
			if errNodeGroupNodes != nil {
				continue
			}

			for _, nodeGroupNode := range nodeGroupNodes.Items {
				nodeGroupNodeInternalName := cloudutils.CreateGCPInternalName(gcpCrawler.scopeID, zone, gcpModel.TypeNodeGroup, nodeGroupNode.Name)
				fmt.Println(nodeGroupNode.Name, nodeGroupNodeInternalName)
			}
		}
	}

	return allNodeGroups, nil
}

func (gcpCrawler *gcpCrawler) getInstanceGroups(client *compute.Service, crawlTime time.Time) ([]*agent.Element, error) {
	allInstanceGroupElems := []*agent.Element{}

	instanceGroups, errInstanceGroups := client.InstanceGroups.AggregatedList(gcpCrawler.ConfiguredProjectID).Do()
	if errInstanceGroups != nil {
		return allInstanceGroupElems, errInstanceGroups
	}

	for scopedZone, list := range instanceGroups.Items {
		for _, instanceGroup := range list.InstanceGroups {
			zone := getZoneFromScopedZone(scopedZone)
			zoneInternalName := cloudutils.CreateGCPInternalName(gcpCrawler.scopeID, "", gcpModel.TypeZone, zone)
			instanceGroupInternalID := cloudutils.CreateGCPInternalName(gcpCrawler.scopeID, zone, gcpModel.TypeInstanceGroup, instanceGroup.Name)

			instanceGroupElem, errInstanceGroupElem := utils.CreateElement(instanceGroup, instanceGroup.Name, instanceGroupInternalID, gcpModel.TypeInstanceGroup, agent.StatusNoStatus, "", crawlTime)
			if errInstanceGroupElem == nil {
				allInstanceGroupElems = append(allInstanceGroupElems, instanceGroupElem)
			}

			utils.AddRelationship(&allInstanceGroupElems, zoneInternalName, instanceGroupInternalID, agent.ParentChildTypeRelation, crawlTime)

			instanceGroupInstanceList, errInstanceGroupInstance := client.InstanceGroups.ListInstances(gcpCrawler.ConfiguredProjectID, zone, instanceGroup.Name, &compute.InstanceGroupsListInstancesRequest{}).Do()
			if errInstanceGroupInstance != nil {
				continue
			}

			for _, instanceGroupInstance := range instanceGroupInstanceList.Items {
				split := strings.Split(instanceGroupInstance.Instance, "/")
				instanceInternalID := cloudutils.CreateGCPInternalName(gcpCrawler.scopeID, zone, gcpModel.TypeVMInstance, split[len(split)-1])

				rel, errRel := utils.CreateRelationship(instanceGroupInternalID, instanceInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					allInstanceGroupElems = append(allInstanceGroupElems, rel)
				}
			}
		}
	}

	return allInstanceGroupElems, nil
}

func (gcp *gcpCrawler) getDisks(client *compute.Service, crawlTime time.Time) ([]*agent.Element, error) {
	allDisks := []*agent.Element{}

	disksAggregatedList, errDisksAggList := client.Disks.AggregatedList(gcp.ConfiguredProjectID).Do()
	if errDisksAggList != nil {
		return allDisks, errDisksAggList
	}

	for scopedZone, diskList := range disksAggregatedList.Items {
		zone := getZoneFromScopedZone(scopedZone)
		zoneInternalName := cloudutils.CreateGCPInternalName(gcp.scopeID, "", gcpModel.TypeZone, zone)

		for _, disk := range diskList.Disks {
			diskInternalID := cloudutils.CreateGCPInternalName(gcp.scopeID, zone, gcpModel.TypeDisk, disk.Name)
			diskElem, errDiskElem := utils.CreateElement(disk, disk.Name, diskInternalID, gcpModel.TypeDisk, getComputeStatus(disk.Status), "", crawlTime)
			if errDiskElem == nil {
				allDisks = append(allDisks, diskElem)
			}

			utils.AddRelationship(&allDisks, zoneInternalName, diskInternalID, agent.ParentChildTypeRelation, crawlTime)
		}
	}

	return allDisks, nil
}

func (gcp *gcpCrawler) getNetworks(client *compute.Service, crawlTime time.Time) ([]*agent.Element, error) {
	allNetworkElems := []*agent.Element{}

	networks, errNetworks := client.Networks.List(gcp.ConfiguredProjectID).Do()
	if errNetworks != nil {
		return allNetworkElems, errNetworks
	}

	for _, network := range networks.Items {
		networkInternalID := cloudutils.CreateGCPInternalName(gcp.scopeID, "", gcpModel.TypeNetwork, network.Name)

		networkElem, errNetworkElem := utils.CreateElement(network, network.Name, networkInternalID, gcpModel.TypeNetwork, agent.StatusNoStatus, "", crawlTime)
		if errNetworkElem == nil {
			allNetworkElems = append(allNetworkElems, networkElem)
		}

		for _, subNet := range network.Subnetworks {
			split := strings.Split(subNet, "/")
			subnetInternalID := cloudutils.CreateGCPInternalName(gcp.scopeID, split[len(split)-3], gcpModel.TypeSubnetwork, split[len(split)-1])

			rel, errRel := utils.CreateRelationship(networkInternalID, subnetInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				allNetworkElems = append(allNetworkElems, rel)
			}
		}
	}

	return allNetworkElems, nil
}

func (gcp *gcpCrawler) getSubNetworks(client *compute.Service, crawlTime time.Time) ([]*agent.Element, error) {
	allSubnets := []*agent.Element{}

	subnetworks, errSubnetworks := client.Subnetworks.AggregatedList(gcp.ConfiguredProjectID).Do()
	if errSubnetworks != nil {
		return allSubnets, errSubnetworks
	}

	for scopedZone, list := range subnetworks.Items {
		zone := getZoneFromScopedZone(scopedZone)
		zoneInternalName := cloudutils.CreateGCPInternalName(gcp.scopeID, "", gcpModel.TypeRegion, zone)

		for _, subnet := range list.Subnetworks {
			subnetInternalID := cloudutils.CreateGCPInternalName(gcp.scopeID, zone, gcpModel.TypeSubnetwork, subnet.Name)
			subnetElem, errSubnetElem := utils.CreateElement(subnet, subnet.Name, subnetInternalID, gcpModel.TypeSubnetwork, getComputeStatus(subnet.State), "", crawlTime)
			if errSubnetElem == nil {
				allSubnets = append(allSubnets, subnetElem)
			}

			utils.AddRelationship(&allSubnets, zoneInternalName, subnetInternalID, agent.ParentChildTypeRelation, crawlTime)
		}
	}

	return allSubnets, nil
}

func (gcp *gcpCrawler) getGKEClusters(crawlTime time.Time) ([]*agent.Element, error) {
	allGKEClusterElems := []*agent.Element{}

	client, errClient := createContainerClient(gcp.clientOpts)
	if errClient != nil {
		return nil, errClient
	}

	clusters, errClusters := client.Projects.Locations.Clusters.List(fmt.Sprintf("projects/%s/locations/-", gcp.ConfiguredProjectID)).Do()
	if errClusters != nil {
		return nil, errClusters
	}

	for _, cluster := range clusters.Clusters {
		zoneInternalName := cloudutils.CreateGCPInternalName(gcp.scopeID, "", gcpModel.TypeRegion, cluster.Zone)
		clusterInternalID := cloudutils.CreateGCPInternalName(gcp.scopeID, cluster.Location, gcpModel.TypeGKE, cluster.Name)
		networkInternalID := cloudutils.CreateGCPInternalName(gcp.scopeID, "", gcpModel.TypeNetwork, cluster.Network)
		subnetInternalID := cloudutils.CreateGCPInternalName(gcp.scopeID, cluster.Zone, gcpModel.TypeSubnetwork, cluster.Subnetwork)
		clusterElem, errClusterElem := utils.CreateElement(cluster, cluster.Name, clusterInternalID, gcpModel.TypeGKE, getComputeStatus(cluster.Status), cluster.CurrentMasterVersion, crawlTime)
		if errClusterElem != nil {
			continue
		}

		utils.AddRelationship(&allGKEClusterElems, zoneInternalName, clusterInternalID, agent.ParentChildTypeRelation, crawlTime)

		allGKEClusterElems = append(allGKEClusterElems, clusterElem)

		networkRel, errNetworkRel := utils.CreateRelationship(networkInternalID, clusterInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errNetworkRel == nil {
			allGKEClusterElems = append(allGKEClusterElems, networkRel)
		}

		subnetRel, errSubnetRel := utils.CreateRelationship(subnetInternalID, clusterInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errSubnetRel == nil {
			allGKEClusterElems = append(allGKEClusterElems, subnetRel)
		}

		for _, nodePool := range cluster.NodePools {
			nodePoolInternalID := cloudutils.CreateGCPInternalName(gcp.scopeID, "", gcpModel.TypeNodePool, nodePool.Name)
			nodePoolElem, errNodePoolElem := utils.CreateElement(nodePool, nodePool.Name, nodePoolInternalID, gcpModel.TypeNodePool, getComputeStatus(nodePool.Status), "", crawlTime)
			if errNodePoolElem != nil {
				continue
			}

			utils.AddRelationship(&allGKEClusterElems, clusterInternalID, nodePoolInternalID, agent.ParentChildTypeRelation, crawlTime)

			allGKEClusterElems = append(allGKEClusterElems, nodePoolElem)

			for _, instanceGroupUrl := range nodePool.InstanceGroupUrls {
				split := strings.Split(instanceGroupUrl, "/")
				instanceGroupName := split[len(split)-1]
				zone := split[len(split)-3]

				instanceGroupInternalID := cloudutils.CreateGCPInternalName(gcp.scopeID, zone, gcpModel.TypeInstanceGroup, instanceGroupName)
				utils.AddRelationship(&allGKEClusterElems, nodePoolInternalID, instanceGroupInternalID, agent.ParentChildTypeRelation, crawlTime)

				computeClient, errComputeClient := createComputeClient(gcp.clientOpts)
				if errComputeClient != nil {
					continue
				}
				instances, err := computeClient.InstanceGroups.ListInstances(
					gcp.ConfiguredProjectID,
					zone,
					instanceGroupName,
					&compute.InstanceGroupsListInstancesRequest{},
				).Do()
				if err != nil {
					continue
				}

				// Print instance details
				for _, instance := range instances.Items {
					instanceSplit := strings.Split(instance.Instance, "/")
					instanceZone := instanceSplit[len(instanceSplit)-3]
					instanceName := instanceSplit[len(instanceSplit)-1]
					instanceInternalName := cloudutils.CreateGCPInternalName(gcp.scopeID, instanceZone, gcpModel.TypeVMInstance, instanceName)

					utils.AddRelationship(&allGKEClusterElems, clusterInternalID, instanceInternalName, agent.ParentChildTypeRelation, crawlTime)
					utils.AddRelationship(&allGKEClusterElems, nodePoolInternalID, instanceInternalName, agent.ParentChildTypeRelation, crawlTime)
					utils.AddRelationship(&allGKEClusterElems, subnetInternalID, instanceInternalName, agent.ParentChildTypeRelation, crawlTime)
					utils.AddRelationship(&allGKEClusterElems, networkInternalID, instanceInternalName, agent.ParentChildTypeRelation, crawlTime)
				}
			}

			relNetwork, errRelNetwork := utils.CreateRelationship(networkInternalID, nodePoolInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRelNetwork == nil {
				allGKEClusterElems = append(allGKEClusterElems, relNetwork)
			}

			relSubnet, errRelSubnet := utils.CreateRelationship(subnetInternalID, nodePoolInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRelSubnet == nil {
				allGKEClusterElems = append(allGKEClusterElems, relSubnet)
			}
		}
	}

	return allGKEClusterElems, nil
}

func (gcp *gcpCrawler) getSqlInstances(crawlTime time.Time) ([]*agent.Element, error) {
	allCrawledSqlInstances := []*agent.Element{}

	client, errClient := sqladmin.NewService(context.Background(), gcp.clientOpts...)
	if errClient != nil {
		return nil, errClient
	}

	sqlInstancesList, errSqlInstancesList := client.Instances.List(gcp.ConfiguredProjectID).Do()
	if errSqlInstancesList != nil {
		return nil, errSqlInstancesList
	}

	for _, sqlInstance := range sqlInstancesList.Items {
		sqlInternalName := cloudutils.CreateGCPInternalName(gcp.scopeID, sqlInstance.GceZone, gcpModel.TypeCloudSQL, sqlInstance.Name)
		gcp.rememberCloudSQLZone(sqlInstance.Name, sqlInstance.GceZone)
		gcp.rememberCloudSQLZone(fmt.Sprintf("%s:%s", gcp.ConfiguredProjectID, sqlInstance.Name), sqlInstance.GceZone)
		status := getComputeStatus(sqlInstance.State)

		if status == agent.StatusGreen && status != getComputeStatus(sqlInstance.Settings.ActivationPolicy) {
			status = getComputeStatus(sqlInstance.Settings.ActivationPolicy)
		}

		dbVer, errDbVer := ParseCloudSqlVersionToSemver(sqlInstance.DatabaseInstalledVersion)
		if errDbVer != nil {
			dbVer = ""
		}

		elem, errElem := utils.CreateElement(sqlInstance, sqlInstance.Name, sqlInternalName, gcpModel.TypeCloudSQL, status, dbVer, crawlTime)
		if errElem == nil {
			allCrawledSqlInstances = append(allCrawledSqlInstances, elem)
		}

		if sqlInstance.Settings == nil || sqlInstance.Settings.IpConfiguration == nil || sqlInstance.Settings.IpConfiguration.PrivateNetwork == "" {
			continue
		}

		split := strings.Split(sqlInstance.Settings.IpConfiguration.PrivateNetwork, "/")
		vpcInternalID := cloudutils.CreateGCPInternalName(gcp.scopeID, "", gcpModel.TypeNetwork, split[4])
		rel, errRel := utils.CreateRelationship(vpcInternalID, sqlInternalName, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			allCrawledSqlInstances = append(allCrawledSqlInstances, rel)
		}

		// add sql IP addresses in the internal cache
		for _, ipv4 := range sqlInstance.IpAddresses {
			if _, exists := gcp.internalIDMapper[ipv4.IpAddress]; !exists {
				gcp.internalIDMapper[ipv4.IpAddress] = sqlInternalName
			}
		}
	}

	return allCrawledSqlInstances, nil
}
