package kubernetes

import (
	"time"

	"github.com/coordimap/agent/pkg/domain/agent"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
)

func getDeploymentStatus(deploymentConditions []appsv1.DeploymentCondition) string {
	var mostRecentStatusTime time.Time
	deploymentStatus := agent.StatusNoStatus

	for _, condition := range deploymentConditions {
		if mostRecentStatusTime.After(condition.LastUpdateTime.Time) {
			continue
		}

		mostRecentStatusTime = condition.LastUpdateTime.Time

		if condition.Type == appsv1.DeploymentProgressing {
			deploymentStatus = agent.StatusNoStatus
		} else if condition.Type == appsv1.DeploymentAvailable {
			deploymentStatus = agent.StatusGreen
		} else if condition.Type == appsv1.DeploymentReplicaFailure {
			deploymentStatus = agent.StatusRed
		}
	}

	return deploymentStatus
}

func getPodStatus(condition v1.PodPhase) string {
	switch condition {
	case v1.PodFailed:
		return agent.StatusRed

	case v1.PodSucceeded, v1.PodRunning:
		return agent.StatusGreen

	case v1.PodPending:
		return agent.StatusOrange

	default:
		return agent.StatusNoStatus
	}
}
