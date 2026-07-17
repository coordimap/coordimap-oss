package gcp

import "github.com/coordimap/agent/pkg/domain/agent"

func getComputeStatus(status string) string {
	switch status {
	case "RUNNING", "READY", "RUNNABLE", "ALWAYS":
		return agent.StatusGreen

	case "DEPROVISIONING", "STOPPED", "STOPPING", "SUSPENDED", "SUSPENDING", "TERMINATED", "INVALID", "DRAINING", "ERROR", "FAILED", "UNAVAILABLE", "MAINTENANCE", "NEVER":
		return agent.StatusRed

	case "PROVISIONING", "STAGING", "REPAIRING", "DELETING", "CREATING", "RECONCILING", "DEGRADED", "RUNNING_WITH_ERROR", "PENDING_DELETE", "PENDING_CREATE", "ON_DEMAND":
		return agent.StatusOrange
	}

	return agent.StatusNoStatus
}
