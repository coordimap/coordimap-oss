package kubernetes

import (
	"context"
	"fmt"
	"sort"
	"time"

	cloudutils "github.com/coordimap/agent/internal/cloud/utils"
	"github.com/coordimap/agent/internal/metrics"
	metricsPrometheus "github.com/coordimap/agent/internal/metrics/prometheus"
	"github.com/coordimap/agent/pkg/domain/agent"
	kubeModel "github.com/coordimap/agent/pkg/domain/kubernetes"
	"github.com/coordimap/agent/pkg/domain/metrictrigger"
	"github.com/coordimap/agent/pkg/utils"
	"github.com/rs/zerolog/log"
)

func (kubeCrawler *kubernetesCrawler) getMetricTriggerElements(crawlTime time.Time) ([]*agent.Element, error) {
	triggerElements := []*agent.Element{}
	if len(kubeCrawler.metricRules) == 0 {
		return triggerElements, nil
	}

	if kubeCrawler.metricPromCrawler == nil || kubeCrawler.metricPromCrawler.promClient == nil {
		return triggerElements, fmt.Errorf("metrics_prometheus_host or prometheus_host must be configured when metric_rules are used")
	}

	evaluatorFactory := metrics.NewEvaluatorFactory(
		metricsPrometheus.NewEvaluator(kubeCrawler.metricPromCrawler.promClient),
	)

	for _, rule := range kubeCrawler.metricRules {
		evaluator, errEvaluator := evaluatorFactory.Get(rule.Provider)
		if errEvaluator != nil {
			log.Debug().Err(errEvaluator).Str("MetricRule", rule.ID).Msg("skipping unsupported metric rule provider for kubernetes crawler")
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		samples, errQuery := evaluator.Evaluate(ctx, rule, time.Now().UTC())
		cancel()
		if errQuery != nil {
			log.Error().Err(errQuery).Str("MetricRule", rule.ID).Msg("could not evaluate metric rule")
			continue
		}

		targetByID := map[string]metrictrigger.Target{}
		for _, sample := range samples {
			value := sample.Value
			if !metrics.EvaluateThreshold(value, rule.Threshold) {
				continue
			}

			internalID, found := kubeCrawler.resolveMetricInternalID(rule.Target, sample.Labels)
			if !found {
				continue
			}

			targetByID[internalID] = metrictrigger.Target{
				InternalID: internalID,
				Value:      value,
				Labels:     sample.Labels,
			}
		}

		if len(targetByID) == 0 {
			continue
		}

		targets := make([]metrictrigger.Target, 0, len(targetByID))
		targetIDs := make([]string, 0, len(targetByID))
		for _, target := range targetByID {
			targets = append(targets, target)
			targetIDs = append(targetIDs, target.InternalID)
		}
		sort.Slice(targets, func(i, j int) bool {
			return targets[i].InternalID < targets[j].InternalID
		})

		payload := metrictrigger.Trigger{
			TriggerType:    metrictrigger.TriggerTypeMetricRule,
			Provider:       rule.Provider,
			DataSourceID:   kubeCrawler.dataSource.DataSourceID,
			DataSourceType: kubeCrawler.dataSource.Info.Type,
			RuleID:         rule.ID,
			RuleName:       rule.Name,
			Query:          rule.Query,
			Window:         rule.Lookback,
			Threshold: metrictrigger.Threshold{
				Operator: rule.Threshold.Operator,
				Value:    rule.Threshold.Value,
			},
			Targets:     targets,
			TriggeredAt: crawlTime,
		}

		elementID := metrics.BuildTriggerElementID(kubeCrawler.dataSource.DataSourceID, rule.ID, crawlTime.UTC().Format("200601021504"), targetIDs)
		element, errElement := utils.CreateMetricTriggerElement(payload, rule.Name, elementID, crawlTime)
		if errElement != nil {
			return nil, fmt.Errorf("could not create metric trigger element for rule %s: %w", rule.ID, errElement)
		}

		triggerElements = append(triggerElements, element)
	}

	return triggerElements, nil
}

func (kubeCrawler *kubernetesCrawler) resolveMetricInternalID(target metrics.TargetConfig, labels map[string]string) (string, bool) {
	switch target.Resolver {
	case metrics.ResolverKubernetesService:
		namespace := labels[target.NamespaceLabel]
		name := labels[target.NameLabel]
		if namespace == "" || name == "" {
			return "", false
		}

		return kubeCrawler.kubeInternalName(namespace, kubeModel.TypeService, name), true

	case metrics.ResolverKubernetesDeployment:
		namespace := labels[target.NamespaceLabel]
		name := labels[target.NameLabel]
		if namespace == "" || name == "" {
			return "", false
		}

		return kubeCrawler.kubeInternalName(namespace, kubeModel.TypeDeployment, name), true

	case metrics.ResolverKubernetesPod:
		namespace := labels[target.NamespaceLabel]
		name := labels[target.NameLabel]
		if namespace == "" || name == "" {
			return "", false
		}

		return kubeCrawler.kubeInternalName(namespace, kubeModel.TypePod, name), true

	case metrics.ResolverKubernetesPVC:
		namespace := labels[target.NamespaceLabel]
		name := labels[target.NameLabel]
		if namespace == "" || name == "" {
			return "", false
		}

		return kubeCrawler.kubeInternalName(namespace, kubeModel.TypePVC, name), true

	case metrics.ResolverKubernetesStatefulSet:
		namespace := labels[target.NamespaceLabel]
		name := labels[target.NameLabel]
		if namespace == "" || name == "" {
			return "", false
		}

		return kubeCrawler.kubeInternalName(namespace, kubeModel.TypeStatefulSet, name), true

	case metrics.ResolverExternalMapping:
		mappingKey := metrics.RenderTemplate(target.MappingKeyTemplate, labels, nil)
		if mappingKey == "" {
			return "", false
		}

		mappedValue, errMappedValue := cloudutils.GetMappingValue(kubeCrawler.externalMappings, mappingKey)
		if errMappedValue != nil {
			return "", false
		}

		return mappedValue, true

	default:
		return "", false
	}
}
