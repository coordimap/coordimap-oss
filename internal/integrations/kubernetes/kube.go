package kubernetes

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	cloudutils "github.com/coordimap/agent/internal/cloud/utils"
	"github.com/coordimap/agent/internal/metrics"
	"github.com/coordimap/agent/pkg/utils"

	"github.com/coordimap/agent/pkg/domain/agent"
	gcpModel "github.com/coordimap/agent/pkg/domain/gcp"
	kube_model "github.com/coordimap/agent/pkg/domain/kubernetes"
	"github.com/rs/zerolog/log"
)

func MakeKubernetesCrawler(dataSource *agent.DataSource, outChannel chan *agent.CloudCrawlData) (Crawler, error) {
	clientInitialzed := false

	// Create initial kubernetesCrawler object
	crawler := &kubernetesCrawler{
		kubeClient:        nil,
		crawlInterval:     defaultCrawlTime,
		dataSource:        *dataSource,
		outputChannel:     outChannel,
		istioConfigured:   false,
		istioCrawler:      prometheusCrawler{},
		clusterName:       "",
		clusterUID:        "",
		retinaCrawler:     nil,
		internalNodeNames: map[string]string{},
		externalMappings:  map[string]string{},
		metricRules:       []metrics.RuleConfig{},
		metricPromCrawler: nil,
		sendSecretData:    true,
		sendConfigMapData: true,
	}

	promQueryTime := ""

	// Assign values from the config
	for _, dsConfig := range dataSource.Config.ValuePairs {
		value, errLoadValue := utils.LoadValueFromEnvConfig(dsConfig.Value)
		if errLoadValue != nil {
			log.Info().Msgf("Error loading value of db_pass for value: %s. The error returned was: %s", dsConfig.Value, errLoadValue.Error())
			return crawler, errLoadValue
		}

		switch dsConfig.Key {

		case kubeConfigInCluster:
			if strings.Compare(value, "true") != 0 || clientInitialzed {
				continue
			}

			clientSet, errClientSet := connectoToK8sInCluster()
			if errClientSet != nil {
				return crawler, errClientSet
			}
			crawler.kubeClient = clientSet

			clientInitialzed = true

		case kubeConfigConfigFile:
			if clientInitialzed {
				continue
			}

			clientSet, errClientSet := connectToK8sFromConfigFile(value)
			if errClientSet != nil {
				return crawler, errClientSet
			}

			crawler.kubeClient = clientSet

			clientInitialzed = true

		case kubeConfigCloudDataSourceID:
			if dsConfig.Value == "" {
				continue
			}
			crawler.cloudDataSourceID = dsConfig.Value

		case kubeConfigIstioPrometheusHost:
			istioCrawler, err := makePrometheusCrawler(value)
			if err != nil {
				return crawler, err
			}

			crawler.istioCrawler = istioCrawler
			crawler.istioConfigured = true

		case kubeConfigRetinaPrometheusHost:
			retinaCrawler, err := makePrometheusCrawler(value)
			if err != nil {
				return crawler, err
			}

			crawler.retinaCrawler = &retinaCrawler

		case kubeConfigMetricPrometheusHost:
			metricCrawler, err := makePrometheusCrawler(value)
			if err != nil {
				return crawler, err
			}

			crawler.metricPromCrawler = &metricCrawler

		case kubeConfigExternalMappings:
			mappings, errMappings := cloudutils.SplitConfiguredMappings(dsConfig.Value)
			if errMappings != nil {
				log.Error().Str("ConfiguredMappings", dsConfig.Value).Msg("Could not generate and use mapping configs.")
				continue
			}

			crawler.externalMappings = mappings

		case kubeConfigClusterName:
			crawler.clusterName = value

		case kubeConfigScopeID:
			if value == "" {
				continue
			}

			crawler.clusterUID = value

		case kubeConfigSendSecretData:
			crawler.sendSecretData = strings.Compare(value, "false") != 0

		case kubeConfigSendConfigMapData:
			crawler.sendConfigMapData = strings.Compare(value, "false") != 0

		case kubeConfigCrawlInterval:
			amountStr := string(dsConfig.Value[:len(dsConfig.Value)-1])
			durationStr := string(dsConfig.Value[len(dsConfig.Value)-1])
			promQueryTime = value

			amount, errConv := strconv.ParseInt(amountStr, 10, 32)
			if errConv != nil {
				return crawler, errConv
			}

			switch durationStr {
			case "s":
				crawler.crawlInterval = time.Duration(amount) * time.Second

			case "m":
				crawler.crawlInterval = time.Duration(amount) * time.Minute

			default:
				crawler.crawlInterval = defaultCrawlTime
			}
		}
	}
	crawler.metricRules = append(crawler.metricRules, dataSource.Config.MetricRules...)

	if crawler.clusterUID == "" {
		return nil, fmt.Errorf("Kubernetes crawler config error: scope_id must be provided for data source %s", crawler.dataSource.DataSourceID)
	}

	crawler.istioCrawler.promQueryTime = promQueryTime
	if crawler.metricPromCrawler == nil && crawler.istioCrawler.promClient != nil {
		crawler.metricPromCrawler = &crawler.istioCrawler
	}

	return crawler, nil
}

func (kubeCrawler *kubernetesCrawler) kubeInternalName(namespace, assetType, name string) string {
	return cloudutils.CreateKubeInternalName(kubeCrawler.clusterUID, namespace, assetType, name)
}

func (kubeCrawler *kubernetesCrawler) Crawl() {
	crawlTicker := time.NewTicker(kubeCrawler.crawlInterval)

	log.Info().Msgf("Starting ticker for: %s", kubeCrawler.dataSource.DataSourceID)
	for range crawlTicker.C {
		_, errCrawl := kubeCrawler.crawl()
		log.Info().Msgf("Crawling Kubernetes cluster for %s", kubeCrawler.dataSource.DataSourceID)
		if errCrawl != nil {
			// do not ship any data
			log.Info().Msg(errCrawl.Error())
			continue
		}
		// ship the crawledData to the backend
	}
}

func (kubeCrawler *kubernetesCrawler) crawl() (*agent.CloudCrawlData, error) {
	crawlTime := time.Now().UTC()
	globalCrawledElements := []*agent.Element{}
	createdElementsFromLabels := []string{}

	nodes, errNodes := kubeCrawler.getNodes()
	if errNodes != nil {
		log.Warn().Msgf("Could not get the kubernetes nodes of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errNodes.Error())
	}

	for _, node := range nodes {
		nodeInternalName := kubeCrawler.kubeInternalName("", kube_model.TypeNode, node.Name)
		if mappedInternalName, ok := resolveExternalNodeInternalName(node, kubeCrawler.externalMappings); ok {
			nodeInternalName = mappedInternalName
			kubeCrawler.internalNodeNames[node.Name] = nodeInternalName
			continue
		}

		kubeCrawler.internalNodeNames[node.Name] = nodeInternalName

		nodeElement, errNodeElement := utils.CreateElement(node, node.Name, nodeInternalName, kube_model.TypeNode, agent.StatusNoStatus, "", crawlTime)
		if errNodeElement != nil {
			continue
		}

		globalCrawledElements = append(globalCrawledElements, nodeElement)
	}

	pvs, errPvs := kubeCrawler.listPersistentVolumes()
	if errPvs != nil {
		log.Warn().Msgf("Could not get the kubernetes persistenvolumes of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errPvs.Error())
	} else {
		for _, pv := range pvs {
			pvInternalID := kubeCrawler.kubeInternalName("", kube_model.TypePV, pv.Name)
			nodeElement, errNodeElement := utils.CreateElement(pv, pv.Name, pvInternalID, kube_model.TypePV, agent.StatusNoStatus, "", crawlTime)
			if errNodeElement != nil {
				continue
			}

			elems, createdElems := kubeCrawler.getLabelElementsAndRelationships(pvInternalID, "", pv.Labels, createdElementsFromLabels, crawlTime)
			createdElementsFromLabels = append(createdElementsFromLabels, createdElems...)
			globalCrawledElements = append(globalCrawledElements, elems...)

			rel, errRel := utils.CreateRelationship(pvInternalID, kubeCrawler.kubeInternalName("", kube_model.TypeStorageClass, pv.Spec.StorageClassName), agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				globalCrawledElements = append(globalCrawledElements, rel)
			}

			globalCrawledElements = append(globalCrawledElements, nodeElement)
		}
	}

	storageClasses, errStorageClasses := kubeCrawler.listStorageClasses()
	if errStorageClasses != nil {
		log.Warn().Msgf("Could not get the kubernetes storageclasses of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errStorageClasses.Error())
	} else {
		for _, storageClass := range storageClasses {
			storageClassInternalID := kubeCrawler.kubeInternalName("", kube_model.TypeStorageClass, storageClass.Name)
			nodeElement, errNodeElement := utils.CreateElement(storageClass, storageClass.Name, storageClassInternalID, kube_model.TypeStorageClass, agent.StatusNoStatus, "", crawlTime)
			if errNodeElement != nil {
				continue
			}

			elems, createdElems := kubeCrawler.getLabelElementsAndRelationships(storageClassInternalID, "", storageClass.Labels, createdElementsFromLabels, crawlTime)
			createdElementsFromLabels = append(createdElementsFromLabels, createdElems...)
			globalCrawledElements = append(globalCrawledElements, elems...)

			globalCrawledElements = append(globalCrawledElements, nodeElement)
		}
	}

	kubeNamespaces, errNamespaces := kubeCrawler.listNamespaces()
	if errNamespaces != nil {
		log.Warn().Msgf("Could not get the kubernetes namespaces of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errNamespaces.Error())
	}

	for _, namespace := range kubeNamespaces {
		namespaceInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeNamespace, "")
		allCrawledElements := []*agent.Element{}
		allCrawledElements = append(allCrawledElements, globalCrawledElements...)

		namespaceElement, errNodeElement := utils.CreateElement(namespace, namespace.Name, namespaceInternalID, kube_model.TypeNamespace, agent.StatusNoStatus, "", crawlTime)
		if errNodeElement != nil {
			continue
		}

		elems, createdElems := kubeCrawler.getLabelElementsAndRelationships(namespaceInternalID, namespace.Name, namespace.Labels, createdElementsFromLabels, crawlTime)
		createdElementsFromLabels = append(createdElementsFromLabels, createdElems...)
		allCrawledElements = append(allCrawledElements, elems...)

		allCrawledElements = append(allCrawledElements, namespaceElement)

		// add the relevant namespace - storageClass relationship
		for _, storageClass := range storageClasses {
			if storageClass.Namespace == namespace.Name {
				storageClassInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeStorageClass, storageClass.Name)
				rel, errRel := utils.CreateRelationship(namespaceInternalID, storageClassInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					allCrawledElements = append(allCrawledElements, rel)
				}

				break
			}
		}

		// add the relevant namespace - persistenVolume relationship
		for _, pv := range pvs {
			if pv.Namespace == namespace.Name {
				pvInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypePV, pv.Name)
				rel, errRel := utils.CreateRelationship(namespaceInternalID, pvInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					allCrawledElements = append(allCrawledElements, rel)
				}

				break
			}
		}

		// get the deployments
		deployments, errDeployments := kubeCrawler.listDeplyments(namespace.Name)
		if errDeployments != nil {
			log.Warn().Msgf("Could not get the kubernetes deployments of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errDeployments.Error())
		} else {
			for _, deployment := range deployments {
				deploymentStatus := getDeploymentStatus(deployment.Status.Conditions)
				deploymentInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeDeployment, deployment.Name)
				version, _ := GetAppVersionFromLabels(deployment.Labels)
				nodeElement, errNodeElement := utils.CreateElement(deployment, deployment.Name, deploymentInternalID, kube_model.TypeDeployment, deploymentStatus, version, crawlTime)
				if errNodeElement != nil {
					continue
				}

				elems, createdElems := kubeCrawler.getLabelElementsAndRelationships(deploymentInternalID, namespace.Name, deployment.Labels, createdElementsFromLabels, crawlTime)
				createdElementsFromLabels = append(createdElementsFromLabels, createdElems...)
				allCrawledElements = append(allCrawledElements, elems...)

				// create namespace - deployment relationship
				rel, errRel := utils.CreateRelationship(namespaceInternalID, deploymentInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					allCrawledElements = append(allCrawledElements, rel)
				}

				// get deployment pods
				deploymentPods, errDeploymentPods := kubeCrawler.listDeplymentPods(&deployment, namespace.Name)
				if errDeploymentPods != nil {
					continue
				}

				for _, deploymentPodRelationship := range deploymentPods {
					nameAndID := fmt.Sprintf("%s.%s", deploymentPodRelationship.SourceID, deploymentPodRelationship.DestinationID)
					servicePodRelationshipElem, errSerservicePodRelationshipElem := utils.CreateElement(
						deploymentPodRelationship,
						nameAndID,
						nameAndID,
						agent.RelationshipType,
						agent.StatusNoStatus, "",
						crawlTime,
					)

					if errSerservicePodRelationshipElem != nil {
						continue
					}

					allCrawledElements = append(allCrawledElements, servicePodRelationshipElem)
				}

				allCrawledElements = append(allCrawledElements, nodeElement)
			}
		}

		serviceAccounts, errServiceAccounts := kubeCrawler.listServiceAccounts(namespace.Name)
		if errServiceAccounts != nil {
			log.Warn().Msgf("Could not get the kubernetes service accounts of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errServiceAccounts.Error())
		} else {
			for _, serviceAccount := range serviceAccounts {
				serviceAccountInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeServiceAccount, serviceAccount.Name)
				serviceAccountElement, errServiceAccountElement := utils.CreateElement(serviceAccount, serviceAccount.Name, serviceAccountInternalID, kube_model.TypeServiceAccount, agent.StatusNoStatus, "", crawlTime)
				if errServiceAccountElement != nil {
					continue
				}

				allCrawledElements = append(allCrawledElements, serviceAccountElement)

				rel, errRel := utils.CreateRelationship(namespaceInternalID, serviceAccountInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					allCrawledElements = append(allCrawledElements, rel)
				}

				gcpServiceAccount, ok := serviceAccount.Annotations[gkeServiceAccountAnnotation]
				if ok && gcpServiceAccount != "" {
					gcpScopeID, errGCPScopeID := cloudutils.GetMappingValue(kubeCrawler.externalMappings, gcpServiceAccount)
					if errGCPScopeID == nil {
						gcpServiceAccountInternalID := cloudutils.CreateGCPInternalName(gcpScopeID, "", gcpModel.TypeServiceAccount, gcpServiceAccount)
						rel, errRel = utils.CreateRelationship(serviceAccountInternalID, gcpServiceAccountInternalID, agent.RelationshipExternalDestinationSideType, agent.ParentChildTypeRelation, crawlTime)
						if errRel == nil {
							allCrawledElements = append(allCrawledElements, rel)
						}
					}
				}
			}
		}

		// get the services
		services, errServices := kubeCrawler.listServices(namespace.Name)
		if errServices != nil {
			log.Warn().Msgf("Could not get the kubernetes services of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errServices.Error())
		} else {
			for _, service := range services {
				serviceInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeService, service.Name)
				version, _ := GetAppVersionFromLabels(service.Labels)
				nodeElement, errNodeElement := utils.CreateElement(service, service.Name, serviceInternalID, kube_model.TypeService, agent.StatusNoStatus, version, crawlTime)
				if errNodeElement != nil {
					continue
				}

				elems, createdElems := kubeCrawler.getLabelElementsAndRelationships(serviceInternalID, namespace.Name, service.Labels, createdElementsFromLabels, crawlTime)
				createdElementsFromLabels = append(createdElementsFromLabels, createdElems...)
				allCrawledElements = append(allCrawledElements, elems...)

				// create namespace - service relationship
				rel, errRel := utils.CreateRelationship(namespaceInternalID, serviceInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					allCrawledElements = append(allCrawledElements, rel)
				}

				// get the service pods
				servicePods, errServicePods := kubeCrawler.listServicePods(&service, namespace.Name)
				if errServicePods != nil {
					continue
				}

				for _, servicePodRelationship := range servicePods {
					nameAndID := fmt.Sprintf("%s.%s", servicePodRelationship.SourceID, servicePodRelationship.DestinationID)
					servicePodRelationshipElem, errSerservicePodRelationshipElem := utils.CreateElement(
						servicePodRelationship,
						nameAndID,
						nameAndID,
						agent.RelationshipType,
						agent.StatusNoStatus, "",
						crawlTime,
					)

					if errSerservicePodRelationshipElem != nil {
						continue
					}

					allCrawledElements = append(allCrawledElements, servicePodRelationshipElem)
				}

				allCrawledElements = append(allCrawledElements, nodeElement)
			}
		}

		// get pods
		pods, errPods := kubeCrawler.listPods(namespace.Name)
		if errPods != nil {
			log.Warn().Msgf("Could not get the kubernetes pods of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errPods.Error())
		} else {
			for _, pod := range pods {
				podStatus := getPodStatus(pod.Status.Phase)
				podInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypePod, pod.Name)
				version, _ := GetAppVersionFromLabels(pod.Labels)
				nodeElement, errNodeElement := utils.CreateElement(pod, pod.Name, podInternalID, kube_model.TypePod, podStatus, version, crawlTime)
				if errNodeElement != nil {
					continue
				}

				elems, createdElems := kubeCrawler.getLabelElementsAndRelationships(podInternalID, namespace.Name, pod.Labels, createdElementsFromLabels, crawlTime)
				createdElementsFromLabels = append(createdElementsFromLabels, createdElems...)
				allCrawledElements = append(allCrawledElements, elems...)

				rel, errRel := utils.CreateRelationship(namespaceInternalID, podInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					allCrawledElements = append(allCrawledElements, rel)
				}

				if pod.Spec.NodeName != "" {
					if internalName, ok := kubeCrawler.internalNodeNames[pod.Spec.NodeName]; ok {
						rel, errRel := utils.CreateRelationship(internalName, podInternalID, agent.RelationshipExternalSourceSideType, agent.ParentChildTypeRelation, crawlTime)
						if errRel == nil {
							allCrawledElements = append(allCrawledElements, rel)
						}
					}
				}

				for _, podContainer := range pod.Spec.Containers {
					for _, containerEnv := range podContainer.Env {
						if containerEnv.ValueFrom != nil {
							if containerEnv.ValueFrom.ConfigMapKeyRef != nil {
								configMapInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeConfigMap, containerEnv.ValueFrom.ConfigMapKeyRef.Name)
								rel, errRel := utils.CreateRelationship(podInternalID, configMapInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
								if errRel == nil {
									allCrawledElements = append(allCrawledElements, rel)
								}
							}

							if containerEnv.ValueFrom.SecretKeyRef != nil {
								podSecretInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeSecret, containerEnv.ValueFrom.SecretKeyRef.Name)
								rel, errRel := utils.CreateRelationship(podInternalID, podSecretInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
								if errRel == nil {
									allCrawledElements = append(allCrawledElements, rel)
								}
							}
						}
					}
				}

				for _, podVolume := range pod.Spec.Volumes {
					podVolumeInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypePV, podVolume.Name)
					rel, errRel := utils.CreateRelationship(podInternalID, podVolumeInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
					if errRel == nil {
						allCrawledElements = append(allCrawledElements, rel)
					}

					if podVolume.ConfigMap != nil {
						podConfigMapInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeConfigMap, podVolume.ConfigMap.Name)
						rel, errRel := utils.CreateRelationship(podInternalID, podConfigMapInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
						if errRel == nil {
							allCrawledElements = append(allCrawledElements, rel)
						}
					}

					if podVolume.PersistentVolumeClaim != nil {
						podPersisternVolumeClaim := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypePVC, podVolume.PersistentVolumeClaim.ClaimName)
						rel, errRel := utils.CreateRelationship(podInternalID, podPersisternVolumeClaim, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
						if errRel == nil {
							allCrawledElements = append(allCrawledElements, rel)
						}
					}

					if podVolume.Secret != nil {
						podSecretInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeSecret, podVolume.Secret.SecretName)
						rel, errRel := utils.CreateRelationship(podInternalID, podSecretInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
						if errRel == nil {
							allCrawledElements = append(allCrawledElements, rel)
						}
					}
				}

				allCrawledElements = append(allCrawledElements, nodeElement)
			}
		}

		// list secrets
		secrets, errSecrets := kubeCrawler.listSecrets(namespace.Name)
		if errSecrets != nil {
			log.Warn().Msgf("Could not get the kubernetes secrets of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errSecrets.Error())
		} else {
			for _, secret := range secrets {
				secretInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeSecret, secret.Name)
				version, _ := GetAppVersionFromLabels(secret.Labels)
				nodeElement, errNodeElement := utils.CreateElement(kubeCrawler.sanitizeSecret(secret), secret.Name, secretInternalID, kube_model.TypeSecret, agent.StatusNoStatus, version, crawlTime)
				if errNodeElement != nil {
					continue
				}

				elems, createdElems := kubeCrawler.getLabelElementsAndRelationships(secretInternalID, namespace.Name, secret.Labels, createdElementsFromLabels, crawlTime)
				createdElementsFromLabels = append(createdElementsFromLabels, createdElems...)
				allCrawledElements = append(allCrawledElements, elems...)

				rel, errRel := utils.CreateRelationship(namespaceInternalID, secretInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					allCrawledElements = append(allCrawledElements, rel)
				}

				allCrawledElements = append(allCrawledElements, nodeElement)
			}
		}

		// list endpoints
		endpoints, errEndpoints := kubeCrawler.listEndpoints(namespace.Name)
		if errEndpoints != nil {
			log.Warn().Msgf("Could not get the kubernetes endpoints of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errEndpoints.Error())
		} else {
			for _, endpoint := range endpoints {
				endpointInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeEndpoint, endpoint.Name)
				version, _ := GetAppVersionFromLabels(endpoint.Labels)
				nodeElement, errNodeElement := utils.CreateElement(endpoint, endpoint.Name, endpointInternalID, kube_model.TypeEndpoint, agent.StatusNoStatus, version, crawlTime)
				if errNodeElement != nil {
					continue
				}

				elems, createdElems := kubeCrawler.getLabelElementsAndRelationships(endpointInternalID, namespace.Name, endpoint.Labels, createdElementsFromLabels, crawlTime)
				createdElementsFromLabels = append(createdElementsFromLabels, createdElems...)
				allCrawledElements = append(allCrawledElements, elems...)

				rel, errRel := utils.CreateRelationship(namespaceInternalID, endpointInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					allCrawledElements = append(allCrawledElements, rel)
				}

				// TODO: add relationship to the target
				// endpoint.Subsets[0].Addresses[0].TargetRef

				allCrawledElements = append(allCrawledElements, nodeElement)
			}
		}

		// list jobs
		jobs, errJobs := kubeCrawler.listJobs(namespace.Name)
		if errJobs != nil {
			log.Warn().Msgf("Could not get the kubernetes jobs of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errJobs.Error())
		} else {
			for _, job := range jobs {
				jobInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeJob, job.Name)
				version, _ := GetAppVersionFromLabels(job.Labels)
				nodeElement, errNodeElement := utils.CreateElement(job, job.Name, jobInternalID, kube_model.TypeJob, agent.StatusNoStatus, version, crawlTime)
				if errNodeElement != nil {
					continue
				}

				elems, createdElems := kubeCrawler.getLabelElementsAndRelationships(jobInternalID, namespace.Name, job.Labels, createdElementsFromLabels, crawlTime)
				createdElementsFromLabels = append(createdElementsFromLabels, createdElems...)
				allCrawledElements = append(allCrawledElements, elems...)

				rel, errRel := utils.CreateRelationship(namespaceInternalID, jobInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					allCrawledElements = append(allCrawledElements, rel)
				}

				if job.Spec.Template.Spec.NodeName != "" {
					if internalName, ok := kubeCrawler.internalNodeNames[job.Spec.Template.Spec.NodeName]; ok {
						rel, errRel := utils.CreateRelationship(internalName, jobInternalID, agent.RelationshipExternalSourceSideType, agent.ParentChildTypeRelation, crawlTime)
						if errRel == nil {
							allCrawledElements = append(allCrawledElements, rel)
						}
					}
				}

				allCrawledElements = append(allCrawledElements, nodeElement)
			}
		}

		// list cronjobs
		cronJobs, errCronJobs := kubeCrawler.listCronJobs(namespace.Name)
		if errCronJobs != nil {
			log.Warn().Msgf("Could not get the kubernetes cronjobs of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errCronJobs.Error())
		} else {
			for _, cronJob := range cronJobs {
				cronJobInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeCronJob, cronJob.Name)
				nodeElement, errNodeElement := utils.CreateElement(cronJob, cronJob.Name, cronJobInternalID, kube_model.TypeCronJob, agent.StatusNoStatus, "", crawlTime)
				if errNodeElement != nil {
					continue
				}

				elems, createdElems := kubeCrawler.getLabelElementsAndRelationships(cronJobInternalID, namespace.Name, cronJob.Labels, createdElementsFromLabels, crawlTime)
				createdElementsFromLabels = append(createdElementsFromLabels, createdElems...)
				allCrawledElements = append(allCrawledElements, elems...)

				rel, errRel := utils.CreateRelationship(namespaceInternalID, cronJobInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					allCrawledElements = append(allCrawledElements, rel)
				}

				if cronJob.Spec.JobTemplate.Spec.Template.Spec.NodeName != "" {
					if internalName, ok := kubeCrawler.internalNodeNames[cronJob.Spec.JobTemplate.Spec.Template.Spec.NodeName]; ok {
						rel, errRel := utils.CreateRelationship(internalName, cronJobInternalID, agent.RelationshipExternalSourceSideType, agent.ParentChildTypeRelation, crawlTime)
						if errRel == nil {
							allCrawledElements = append(allCrawledElements, rel)
						}
					}
				}

				allCrawledElements = append(allCrawledElements, nodeElement)
			}
		}

		// list configmaps
		configMaps, errConfigMaps := kubeCrawler.listConfigMaps(namespace.Name)
		if errConfigMaps != nil {
			log.Warn().Msgf("Could not get the kubernetes configmaps of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errConfigMaps.Error())
		} else {
			for _, configMap := range configMaps {
				configMapInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeConfigMap, configMap.Name)
				version, _ := GetAppVersionFromLabels(configMap.Labels)
				nodeElement, errNodeElement := utils.CreateElement(kubeCrawler.sanitizeConfigMap(configMap), configMap.Name, configMapInternalID, kube_model.TypeConfigMap, agent.StatusNoStatus, version, crawlTime)
				if errNodeElement != nil {
					continue
				}

				elems, createdElems := kubeCrawler.getLabelElementsAndRelationships(configMapInternalID, namespace.Name, configMap.Labels, createdElementsFromLabels, crawlTime)
				createdElementsFromLabels = append(createdElementsFromLabels, createdElems...)
				allCrawledElements = append(allCrawledElements, elems...)

				rel, errRel := utils.CreateRelationship(namespaceInternalID, configMapInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					allCrawledElements = append(allCrawledElements, rel)
				}

				allCrawledElements = append(allCrawledElements, nodeElement)
			}
		}

		// list statefulsets
		statefulSets, errStatefulSets := kubeCrawler.listStatefulSets(namespace.Name)
		if errStatefulSets != nil {
			log.Warn().Msgf("Could not get the kubernetes statefulsets of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errStatefulSets.Error())
		} else {
			for _, statefulSet := range statefulSets {
				statefulSetInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeStatefulSet, statefulSet.Name)
				nodeElement, errNodeElement := utils.CreateElement(statefulSet, statefulSet.Name, statefulSetInternalID, kube_model.TypeStatefulSet, agent.StatusNoStatus, "", crawlTime)
				if errNodeElement != nil {
					continue
				}

				elems, createdElems := kubeCrawler.getLabelElementsAndRelationships(statefulSetInternalID, namespace.Name, statefulSet.Labels, createdElementsFromLabels, crawlTime)
				createdElementsFromLabels = append(createdElementsFromLabels, createdElems...)
				allCrawledElements = append(allCrawledElements, elems...)

				rel, errRel := utils.CreateRelationship(namespaceInternalID, statefulSetInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					allCrawledElements = append(allCrawledElements, rel)
				}

				if statefulSet.Spec.Template.Spec.NodeName != "" {
					if internalName, ok := kubeCrawler.internalNodeNames[statefulSet.Spec.Template.Spec.NodeName]; ok {
						rel, errRel := utils.CreateRelationship(internalName, statefulSetInternalID, agent.RelationshipExternalSourceSideType, agent.ParentChildTypeRelation, crawlTime)
						if errRel == nil {
							allCrawledElements = append(allCrawledElements, rel)
						}
					}
				}

				// add volume details
				for _, pvc := range statefulSet.Spec.VolumeClaimTemplates {
					volInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypePV, pvc.Spec.VolumeName)

					rel, errRel := utils.CreateRelationship(statefulSetInternalID, volInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
					if errRel == nil {
						allCrawledElements = append(allCrawledElements, rel)
					}
				}

				allCrawledElements = append(allCrawledElements, nodeElement)
			}
		}

		// list daemonsets
		daemonSets, errDaemonSets := kubeCrawler.listDaemonSets(namespace.Name)
		if errDaemonSets != nil {
			log.Warn().Msgf("Could not get the kubernetes daemonsets of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errDaemonSets.Error())
		} else {
			for _, daemonSet := range daemonSets {
				daemonSetInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeDaemonSet, daemonSet.Name)
				version, _ := GetAppVersionFromLabels(daemonSet.Labels)
				nodeElement, errNodeElement := utils.CreateElement(daemonSet, daemonSet.Name, daemonSetInternalID, kube_model.TypeDaemonSet, agent.StatusNoStatus, version, crawlTime)
				if errNodeElement != nil {
					continue
				}

				elems, createdElems := kubeCrawler.getLabelElementsAndRelationships(daemonSetInternalID, namespace.Name, daemonSet.Labels, createdElementsFromLabels, crawlTime)
				createdElementsFromLabels = append(createdElementsFromLabels, createdElems...)
				allCrawledElements = append(allCrawledElements, elems...)

				rel, errRel := utils.CreateRelationship(namespaceInternalID, daemonSetInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					allCrawledElements = append(allCrawledElements, rel)
				}

				allCrawledElements = append(allCrawledElements, nodeElement)
			}
		}

		// list pvcs
		pvcs, errPVCs := kubeCrawler.listPersistentVolumeClaims(namespace.Name)
		if errPVCs != nil {
			log.Warn().Msgf("Could not get the kubernetes persistenvolumeclaims of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errPVCs.Error())
		} else {
			for _, pvc := range pvcs {
				pvcInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypePVC, pvc.Name)
				version, _ := GetAppVersionFromLabels(pvc.Labels)
				nodeElement, errNodeElement := utils.CreateElement(pvc, pvc.Name, pvcInternalID, kube_model.TypePVC, agent.StatusNoStatus, version, crawlTime)
				if errNodeElement != nil {
					continue
				}

				elems, createdElems := kubeCrawler.getLabelElementsAndRelationships(pvcInternalID, namespace.Name, pvc.Labels, createdElementsFromLabels, crawlTime)
				createdElementsFromLabels = append(createdElementsFromLabels, createdElems...)
				allCrawledElements = append(allCrawledElements, elems...)

				rel, errRel := utils.CreateRelationship(namespaceInternalID, pvcInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					allCrawledElements = append(allCrawledElements, rel)
				}

				allCrawledElements = append(allCrawledElements, nodeElement)
			}
		}

		// list ingressesExtensionsBeta1
		ingressesExtensionsBeta1, errIngressesExtensionsBeta1 := kubeCrawler.listIngressesExtensionsBeta1(namespace.Name)
		if errIngressesExtensionsBeta1 != nil {
			log.Warn().Msgf("Could not get the kubernetes ingresses extensions beta1 of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errIngressesExtensionsBeta1.Error())
		} else {
			for _, ingress := range ingressesExtensionsBeta1 {
				ingressInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeIngressExtensionBeta1, ingress.Name)
				version, _ := GetAppVersionFromLabels(ingress.Labels)
				nodeElement, errNodeElement := utils.CreateElement(ingress, ingress.Name, ingressInternalID, kube_model.TypeIngressExtensionBeta1, agent.StatusNoStatus, version, crawlTime)
				if errNodeElement != nil {
					continue
				}

				for _, rules := range ingress.Spec.Rules {
					for _, path := range rules.HTTP.Paths {
						internalServiceName := kubeCrawler.kubeInternalName(ingress.Namespace, kube_model.TypeService, path.Backend.ServiceName)
						rel, errRel := utils.CreateRelationship(ingressInternalID, internalServiceName, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
						if errRel == nil {
							allCrawledElements = append(allCrawledElements, rel)
						}
					}
				}

				elems, createdElems := kubeCrawler.getLabelElementsAndRelationships(ingressInternalID, namespace.Name, ingress.Labels, createdElementsFromLabels, crawlTime)
				createdElementsFromLabels = append(createdElementsFromLabels, createdElems...)
				allCrawledElements = append(allCrawledElements, elems...)

				rel, errRel := utils.CreateRelationship(namespaceInternalID, ingressInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					allCrawledElements = append(allCrawledElements, rel)
				}

				allCrawledElements = append(allCrawledElements, nodeElement)
			}
		}

		ingressesNetworkingV1, errIngressesNetworkingV1 := kubeCrawler.listIngressesNetworkingV1(namespace.Name)
		if errIngressesNetworkingV1 != nil {
			log.Warn().Msgf("Could not get the kubernetes ingresses extensions beta1 of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errIngressesExtensionsBeta1.Error())
		} else {
			for _, ingress := range ingressesNetworkingV1 {
				ingressInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeIngressNetworkingV1, ingress.Name)
				version, _ := GetAppVersionFromLabels(ingress.Labels)
				nodeElement, errNodeElement := utils.CreateElement(ingress, ingress.Name, ingressInternalID, kube_model.TypeIngressNetworkingV1, agent.StatusNoStatus, version, crawlTime)
				if errNodeElement != nil {
					continue
				}

				for _, rules := range ingress.Spec.Rules {
					for _, path := range rules.HTTP.Paths {
						internalServiceName := kubeCrawler.kubeInternalName(ingress.Namespace, kube_model.TypeService, path.Backend.Service.Name)
						rel, errRel := utils.CreateRelationship(ingressInternalID, internalServiceName, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
						if errRel == nil {
							allCrawledElements = append(allCrawledElements, rel)
						}
					}
				}

				elems, createdElems := kubeCrawler.getLabelElementsAndRelationships(ingressInternalID, namespace.Name, ingress.Labels, createdElementsFromLabels, crawlTime)
				createdElementsFromLabels = append(createdElementsFromLabels, createdElems...)
				allCrawledElements = append(allCrawledElements, elems...)

				rel, errRel := utils.CreateRelationship(namespaceInternalID, ingressInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					allCrawledElements = append(allCrawledElements, rel)
				}

				allCrawledElements = append(allCrawledElements, nodeElement)
			}
		}

		ingressesNetworkingV1Beta1, errIngressesNetworkingV1Beta1 := kubeCrawler.listIngressesNetworkingV1Beta1(namespace.Name)
		if errIngressesNetworkingV1Beta1 != nil {
			log.Warn().Msgf("Could not get the kubernetes ingresses extensions beta1 of data source name: %s because %s", kubeCrawler.dataSource.DataSourceID, errIngressesExtensionsBeta1.Error())
		} else {
			for _, ingress := range ingressesNetworkingV1Beta1 {
				ingressInternalID := kubeCrawler.kubeInternalName(namespace.Name, kube_model.TypeIngressExtensionBeta1, ingress.Name)
				version, _ := GetAppVersionFromLabels(ingress.Labels)
				nodeElement, errNodeElement := utils.CreateElement(ingress, ingress.Name, ingressInternalID, kube_model.TypeIngressNetworkingV1Beta1, agent.StatusNoStatus, version, crawlTime)
				if errNodeElement != nil {
					continue
				}

				for _, rules := range ingress.Spec.Rules {
					for _, path := range rules.HTTP.Paths {
						internalServiceName := kubeCrawler.kubeInternalName(ingress.Namespace, kube_model.TypeService, path.Backend.ServiceName)
						rel, errRel := utils.CreateRelationship(ingressInternalID, internalServiceName, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
						if errRel == nil {
							allCrawledElements = append(allCrawledElements, rel)
						}
					}
				}

				elems, createdElems := kubeCrawler.getLabelElementsAndRelationships(ingressInternalID, namespace.Name, ingress.Labels, createdElementsFromLabels, crawlTime)
				createdElementsFromLabels = append(createdElementsFromLabels, createdElems...)
				allCrawledElements = append(allCrawledElements, elems...)

				rel, errRel := utils.CreateRelationship(namespaceInternalID, ingressInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					allCrawledElements = append(allCrawledElements, rel)
				}

				allCrawledElements = append(allCrawledElements, nodeElement)
			}
		}

		if kubeCrawler.retinaCrawler != nil {
			if retinaElems, errRetina := kubeCrawler.getRetinaFlowsRelationships(crawlTime); errRetina == nil {
				allCrawledElements = append(allCrawledElements, retinaElems...)
			}
		}

		crawledData := agent.CrawledData{
			Data: allCrawledElements,
		}

		log.Info().Msgf("Crawled %d Kubernetes elements for connection %s and namespace %s", len(allCrawledElements), kubeCrawler.dataSource.DataSourceID, namespace.Name)

		kubeCrawler.outputChannel <- &agent.CloudCrawlData{
			Timestamp:       crawlTime,
			DataSource:      kubeCrawler.dataSource,
			CrawledData:     crawledData,
			CrawlInternalID: fmt.Sprintf("%s.%s", namespace.Name, kube_model.TypeNamespace),
		}
	}

	metricTriggerElems, errMetricTriggerElems := kubeCrawler.getMetricTriggerElements(crawlTime)
	if errMetricTriggerElems != nil {
		log.Info().Msgf("There was an error finding metric trigger elements for kubernetes connection %s because %s", kubeCrawler.dataSource.DataSourceID, errMetricTriggerElems.Error())
	} else if len(metricTriggerElems) > 0 {
		kubeCrawler.outputChannel <- &agent.CloudCrawlData{
			Timestamp:  crawlTime,
			DataSource: kubeCrawler.dataSource,
			CrawledData: agent.CrawledData{
				Data: metricTriggerElems,
			},
		}
	}

	if !kubeCrawler.istioConfigured {
		return nil, nil
	}

	istioRelationships, errIstioRelationships := kubeCrawler.getIstioRelationships()
	if errIstioRelationships != nil {
		log.Info().Msgf("There was an error finding the istio relationships for kubernetes connection %s because %s", kubeCrawler.dataSource.DataSourceID, errIstioRelationships.Error())
		return nil, errIstioRelationships
	}

	istioElements := []*agent.Element{}
	for _, istioRelaitonship := range istioRelationships {
		istioElem, errIstioElem := utils.CreateElement(
			istioRelaitonship,
			fmt.Sprintf("%s-%s", istioRelaitonship.SourceID, istioRelaitonship.DestinationID),
			fmt.Sprintf("%s-%s", istioRelaitonship.SourceID, istioRelaitonship.DestinationID),
			kube_model.FlowIstioRelationshipSkipinsert,
			agent.StatusNoStatus, "",
			crawlTime,
		)
		if errIstioElem != nil {
			continue
		}

		istioElements = append(istioElements, istioElem)
	}

	crawledData := agent.CrawledData{
		Data: istioElements,
	}

	kubeCrawler.outputChannel <- &agent.CloudCrawlData{
		Timestamp:   crawlTime,
		DataSource:  kubeCrawler.dataSource,
		CrawledData: crawledData,
	}

	return nil, nil
}
