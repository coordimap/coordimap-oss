package kubernetes

const (
	TypeNode                     = "kubernetes.node"
	TypeNamespace                = "kubernetes.namespace"
	TypePod                      = "kubernetes.pod"
	TypeDeployment               = "kubernetes.deployment"
	TypeService                  = "kubernetes.service"
	TypePV                       = "kubernetes.pv"
	TypePVC                      = "kubernetes.pvc"
	TypeStorageClass             = "kubernetes.storage_class"
	TypeEndpoint                 = "kubernetes.endpoint"
	TypeSecret                   = "kubernetes.secret"
	TypeIngress                  = "kubernetes.ingress"
	TypeIngressExtensionBeta1    = "kubernetes.ingress_extension_beta1"
	TypeIngressNetworkingV1      = "kubernetes.ingress_networking_v1"
	TypeIngressNetworkingV1Beta1 = "kubernetes.ingress_v1_beta1"
	TypeJob                      = "kubernetes.job"
	TypeCronJob                  = "kubernetes.cronjob"
	TypeConfigMap                = "kubernetes.config_map"
	TypeStatefulSet              = "kubernetes.stateful_set"
	TypeDaemonSet                = "kubernetes.daemon_set"
	TypeHelmChart                = "kubernetes.helm_chart"
	TypeServiceAccount           = "kubernetes.service_account"
	TypeLabelName                = "kubernetes.label_name"
	TypeLabelComponent           = "kubernetes.label_component"
	TypeLabelVersion             = "kubernetes.label_version"
	TypeLabelPartOf              = "kubernetes.label_part_of"
	TypeLabelInstance            = "kubernetes.label_instance"
	TypeLabelManagedBy           = "kubernetes.label_managed_by"
)

const (
	FlowIstioRelationshipSkipinsert     = "kubernetes.flow.istio.relationship_skipinsert"
	FlowIstioRelationshipTypeService    = "kubernetes.flow.istio.relationship.service"
	FlowIstioRelationshipTypeDeployment = "kubernetes.flow.istio.relationship.deployment"
	FlowIstioRelationshipTypePod        = "kubernetes.flow.istio.relationship.pod"
	RelationshipSkipinsert              = "kubernetes.relationship_skipinsert"
	RelationshipTypeServicePod          = "kubernetes.relationship.service_pod"
	RelationshipTypeDeploymentPod       = "kubernetes.relationship.deployment_pod"
)

type KubernetesLabelName struct {
	Name string `json:"name"`
}

type KubernetesLabelComponent struct {
	Name string `json:"name"`
}

type KubernetesLabelPartOf struct {
	Name string `json:"name"`
}

type KubernetesLabelInstance struct {
	Name string `json:"name"`
}

type KubernetesLabelManagedBy struct {
	Name string `json:"name"`
}

type KubernetesChart struct {
	Name string `json:"name"`
}
