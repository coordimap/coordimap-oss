package agent

func GetFlowTypeRelationLabel(flowType int) string {
	switch flowType {
	case ParentChildTypeRelation:
		return "parent_child"

	case ErTypeRelation:
		return "er"

	case GenericFlowTypeRelation:
		return "generic_flow"

	case GCPNetworkFlowTypeRelation:
		return "gcp_network_flow"

	case KubernetesRetinaFlowTypeRelation:
		return "kubernetes_retina_flow"

	case KubernetesIstioFlowTypeRelation:
		return "kubernetes_istio_flow"

	case EBPFFlowTypeRelation:
		return "ebpf_flow"

	default:
		return "unknown_relation"
	}
}
