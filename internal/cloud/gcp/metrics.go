package gcp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	cloudutils "github.com/coordimap/agent/internal/cloud/utils"
	"github.com/coordimap/agent/internal/metrics"
	metricsGCP "github.com/coordimap/agent/internal/metrics/gcp"
	"github.com/coordimap/agent/pkg/domain/agent"
	gcpModel "github.com/coordimap/agent/pkg/domain/gcp"
	"github.com/coordimap/agent/pkg/domain/metrictrigger"
	"github.com/coordimap/agent/pkg/utils"
	"github.com/rs/zerolog/log"
)

func (gcpCrawler *gcpCrawler) getMetricTriggerElements(crawlTime time.Time) ([]*agent.Element, error) {
	triggerElements := []*agent.Element{}
	if len(gcpCrawler.metricRules) == 0 {
		return triggerElements, nil
	}

	if gcpCrawler.monitoringClient == nil {
		return triggerElements, fmt.Errorf("gcp monitoring client is not configured but metric_rules exist")
	}

	evaluatorFactory := metrics.NewEvaluatorFactory(
		metricsGCP.NewEvaluator(gcpCrawler.monitoringClient, gcpCrawler.ConfiguredProjectID),
	)

	for _, rule := range gcpCrawler.metricRules {
		evaluator, errEvaluator := evaluatorFactory.Get(rule.Provider)
		if errEvaluator != nil {
			log.Debug().Err(errEvaluator).Str("MetricRule", rule.ID).Msg("skipping unsupported metric rule provider for gcp crawler")
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		samples, errQuery := evaluator.Evaluate(ctx, rule, time.Now().UTC())
		cancel()
		if errQuery != nil {
			log.Error().Err(errQuery).Str("MetricRule", rule.ID).Msg("could not evaluate gcp metric rule")
			continue
		}

		targetByID := map[string]metrictrigger.Target{}
		for _, sample := range samples {
			value := sample.Value
			if !metrics.EvaluateThreshold(value, rule.Threshold) {
				continue
			}

			internalID, foundID := gcpCrawler.resolveMetricInternalID(rule.Target, sample.Labels, sample.ResourceLabels)
			if !foundID {
				continue
			}

			targetByID[internalID] = metrictrigger.Target{
				InternalID:     internalID,
				Value:          value,
				Labels:         sample.Labels,
				ResourceLabels: sample.ResourceLabels,
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
			DataSourceID:   gcpCrawler.dataSource.DataSourceID,
			DataSourceType: gcpCrawler.dataSource.Info.Type,
			RuleID:         rule.ID,
			RuleName:       rule.Name,
			Filter:         rule.Filter,
			Window:         rule.Lookback,
			Threshold: metrictrigger.Threshold{
				Operator: rule.Threshold.Operator,
				Value:    rule.Threshold.Value,
			},
			Targets:     targets,
			TriggeredAt: crawlTime,
		}
		if rule.MetricType != "" {
			payload.Metadata = map[string]string{"metric_type": rule.MetricType}
		}

		elementID := metrics.BuildTriggerElementID(gcpCrawler.dataSource.DataSourceID, rule.ID, crawlTime.UTC().Format("200601021504"), targetIDs)
		element, errElement := utils.CreateMetricTriggerElement(payload, rule.Name, elementID, crawlTime)
		if errElement != nil {
			return nil, fmt.Errorf("could not create gcp metric trigger element for rule %s: %w", rule.ID, errElement)
		}

		triggerElements = append(triggerElements, element)
	}

	return triggerElements, nil
}

func (gcpCrawler *gcpCrawler) resolveMetricInternalID(target metrics.TargetConfig, metricLabels, resourceLabels map[string]string) (string, bool) {
	getValue := func(labelName string) string {
		if labelName == "" {
			return ""
		}

		if value, exists := metricLabels[labelName]; exists {
			return value
		}

		return resourceLabels[labelName]
	}

	switch target.Resolver {
	case metrics.ResolverGCPCloudSQL:
		nameLabel := target.NameLabel
		if nameLabel == "" {
			nameLabel = "database_id"
		}

		databaseID := getValue(nameLabel)
		if databaseID == "" {
			return "", false
		}

		name := normalizeCloudSQLDatabaseID(databaseID)
		zone, hasZone := gcpCrawler.cloudSQLZones[databaseID]
		if !hasZone {
			zone, hasZone = gcpCrawler.cloudSQLZones[name]
		}
		if !hasZone || zone == "" {
			return "", false
		}

		return cloudutils.CreateGCPInternalName(gcpCrawler.scopeID, zone, gcpModel.TypeCloudSQL, name), true

	case metrics.ResolverGCPVMInstance:
		nameLabel := target.NameLabel
		if nameLabel == "" {
			nameLabel = "instance_name"
		}
		zoneLabel := target.ZoneLabel
		if zoneLabel == "" {
			zoneLabel = "zone"
		}

		name := getValue(nameLabel)
		if name == "" {
			return "", false
		}

		zone := getValue(zoneLabel)
		return cloudutils.CreateGCPInternalName(gcpCrawler.scopeID, zone, gcpModel.TypeVMInstance, name), true

	case metrics.ResolverExternalMapping:
		mappingKey := metrics.RenderTemplate(target.MappingKeyTemplate, metricLabels, resourceLabels)
		if mappingKey == "" {
			return "", false
		}

		mappedValue, errMappedValue := cloudutils.GetMappingValue(gcpCrawler.externalMappings, mappingKey)
		if errMappedValue != nil {
			return "", false
		}

		return mappedValue, true

	default:
		return "", false
	}
}

func (gcpCrawler *gcpCrawler) rememberCloudSQLZone(databaseID, zone string) {
	if databaseID == "" || zone == "" {
		return
	}
	if gcpCrawler.cloudSQLZones == nil {
		gcpCrawler.cloudSQLZones = map[string]string{}
	}

	gcpCrawler.cloudSQLZones[databaseID] = zone
}

func normalizeCloudSQLDatabaseID(databaseID string) string {
	if before, after, found := strings.Cut(databaseID, ":"); found && before != "" && after != "" {
		return after
	}

	return databaseID
}
