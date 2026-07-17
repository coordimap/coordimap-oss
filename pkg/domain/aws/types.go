package aws

const (
	AwsTypeInstance                      = "aws.instance"
	AwsTypeVpc                           = "aws.vpc"
	AwsTypeRegion                        = "aws.region"
	AwsTypeSubnet                        = "aws.subnet"
	AwsTypeNetworkACL                    = "aws.network_acl"
	AwsTypeDHCPOptions                   = "aws.dhcp_options"
	AwsTypeNatGw                         = "aws.natgw"
	AwsTypeAvailabilityZone              = "aws.availability_zone"
	AwsTypeAMI                           = "aws.ami"
	AwsTypeSecGroup                      = "aws.security_group"
	AwsTypeVolume                        = "aws.volume"
	AwsTypeClassicalLB                   = "aws.classical_lb"
	AwsTypeLoadBalancer                  = "aws.load_balancer"
	AwsTypeRouteTable                    = "aws.route_table"
	AwsTypeS3Bucket                      = "aws.s3_bucket"
	AwsTypeOwner                         = "aws.owner"
	AwsTypeLambda                        = "aws.lambda"
	AwsTypeApplicationLoadBalancer       = "aws.application_load_balancer"
	AwsTypeNetworkLoadBalancer           = "aws.network_load_balancer"
	AwsTypeGatewayLoadBalancer           = "aws.gateway_load_balancer"
	AwsTypeLoadBalancerTargetsSkipinsert = "aws.load_balancer_targets_skipinsert"
	AwsTypeRDS                           = "aws.rds"
	AwsTypeEKS                           = "aws.eks"
	AwsTypeEKSNodeGroup                  = "aws.eks_nodegroup"
	AwsTypeECRRepository                 = "aws.ecr_repo"
	AwsTypeECRRepositoryImage            = "aws.ecr_repo_image"
	AwsTypeAutoscalingGroup              = "aws.auto_scaling_group"
)

const (
	AwsRelationshipTypeLoadBalancerV2Targets = "aws.relationships.load_balancer_v2_targets"
	AwsRelationshipSkipinsert                = "aws.relationship_skipinsert"
	AwsRelationshipEKSClusterNodegroup       = "aws.relationship.eks_cluster_nodegroups"
	AwsRelationshipAutoscalingGroupNodegroup = "aws.relationships.autoscaling_group_nodegroups"
	AwsRelationshipEKSAutoscalingGroup       = "aws.relationships.eks_autoscaling_group"
)
