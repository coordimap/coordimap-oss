package flows

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func getPodInfo(clientset *kubernetes.Clientset, cache *PodCache, ipAddress string) (PodInfo, error) {
	// Check cache first
	if info, found := cache.Get(ipAddress); found {
		return info, nil
	}

	// Get pod info from Kubernetes API
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to find pod with this IP
	pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("status.podIP=%s", ipAddress),
	})

	if err != nil || len(pods.Items) == 0 {
		// IP doesn't belong to a pod or API error
		info := PodInfo{
			Name:      "external",
			Namespace: "external",
			Workload:  "external",
			IP:        ipAddress,
		}
		cache.Set(ipAddress, info)
		return info, fmt.Errorf("Could not find pod with IP: %s", ipAddress)
	}

	pod := pods.Items[0]

	// Determine workload type and name
	workloadType := ""
	workloadName := ""

	if len(pod.OwnerReferences) > 0 {
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == "ReplicaSet" {
				rs, err := clientset.AppsV1().ReplicaSets(pod.Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
				if err == nil && len(rs.OwnerReferences) > 0 {
					for _, rsOwner := range rs.OwnerReferences {
						if rsOwner.Kind == "Deployment" {
							workloadType = "Deployment"
							workloadName = rsOwner.Name
							break
						}
					}
				} else {
					workloadType = "ReplicaSet"
					workloadName = owner.Name
				}
			} else if owner.Kind == "StatefulSet" {
				workloadType = "StatefulSet"
				workloadName = owner.Name
			} else if owner.Kind == "DaemonSet" {
				workloadType = "DaemonSet"
				workloadName = owner.Name
			} else if owner.Kind == "Job" {
				job, err := clientset.BatchV1().Jobs(pod.Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
				if err == nil && len(job.OwnerReferences) > 0 {
					for _, jobOwner := range job.OwnerReferences {
						if jobOwner.Kind == "CronJob" {
							workloadType = "CronJob"
							workloadName = jobOwner.Name
							break
						}
					}
				} else {
					workloadType = "Job"
					workloadName = owner.Name
				}
			}

			if workloadType != "" {
				break
			}
		}
	} else {
		workloadType = "Pod"
		workloadName = pod.Name
	}

	info := PodInfo{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Workload:  fmt.Sprintf("%s/%s", workloadType, workloadName),
		IP:        ipAddress,
	}

	cache.Set(ipAddress, info)
	return info, nil
}
