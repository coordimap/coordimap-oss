package aws

import (
	"fmt"
	"slices"
	"strings"
	"time"

	cloudutils "github.com/coordimap/agent/internal/cloud/utils"
	"github.com/coordimap/agent/pkg/utils"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/coordimap/agent/pkg/domain/agent"
	aws_shared_model "github.com/coordimap/agent/pkg/domain/aws"
)

func getAwsAccountID(session *session.Session) (*string, error) {
	svc := sts.New(session)
	input := &sts.GetCallerIdentityInput{}

	result, err := svc.GetCallerIdentity(input)
	if err != nil {
		return nil, err
	}

	return result.Account, nil
}

func describeAllVPCs(session *session.Session, owner []*string, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	var returnedElems []*agent.Element

	svc := ec2.New(session)
	input := &ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("owner-id"),
				Values: owner,
			},
		},
	}

	result, err := svc.DescribeVpcs(input)
	if err != nil {
		return nil, err
	}

	for _, elem := range result.Vpcs {
		vpcInternalID := cloudutils.CreateAWSInternalID(scopeID, *elem.VpcId)
		agentElement, _ := utils.CreateElement(elem, vpcInternalID, *elem.VpcId, aws_shared_model.AwsTypeVpc, agent.StatusNoStatus, "", crawlTime)
		returnedElems = append(returnedElems, agentElement)

		rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *elem.OwnerId), vpcInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			returnedElems = append(returnedElems, rel)
		}
	}

	return returnedElems, nil
}

func describeAllRegions(session *session.Session, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	var returnedElems []*agent.Element

	svc := ec2.New(session)
	input := &ec2.DescribeRegionsInput{}

	result, err := svc.DescribeRegions(input)
	if err != nil {
		return nil, err
	}

	for _, elem := range result.Regions {
		regionInternalID := cloudutils.CreateAWSInternalID(scopeID, *elem.RegionName)
		agentElem, _ := utils.CreateElement(elem, *elem.RegionName, regionInternalID, aws_shared_model.AwsTypeRegion, agent.StatusNoStatus, "", crawlTime)

		returnedElems = append(returnedElems, agentElem)
	}

	return returnedElems, nil
}

func describeAllRouteTables(session *session.Session, owner []*string, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	var returnedElems []*agent.Element

	svc := ec2.New(session)
	input := &ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("owner-id"),
				Values: owner,
			},
		},
	}

	result, err := svc.DescribeRouteTables(input)
	if err != nil {
		return nil, err
	}

	for _, elem := range result.RouteTables {
		routeTableInternalID := cloudutils.CreateAWSInternalID(scopeID, *elem.RouteTableId)
		agentElem, _ := utils.CreateElement(elem, *elem.RouteTableId, routeTableInternalID, aws_shared_model.AwsTypeRouteTable, agent.StatusNoStatus, "", crawlTime)
		returnedElems = append(returnedElems, agentElem)

		rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *elem.VpcId), routeTableInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			returnedElems = append(returnedElems, rel)
		}

		rel, errRel = utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *elem.OwnerId), routeTableInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			returnedElems = append(returnedElems, rel)
		}
	}

	return returnedElems, nil
}

func describeAllDHCPOptions(session *session.Session, owner []*string, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	var returnedElems []*agent.Element

	svc := ec2.New(session)
	input := &ec2.DescribeDhcpOptionsInput{
		// DhcpOptionsIds: dhcpOptionIds,
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("owner-id"),
				Values: owner,
			},
		},
	}

	result, err := svc.DescribeDhcpOptions(input)
	if err != nil {
		return nil, err
	}

	for _, elem := range result.DhcpOptions {
		dhcpOptionInternalID := cloudutils.CreateAWSInternalID(scopeID, *elem.DhcpOptionsId)
		agentElem, _ := utils.CreateElement(elem, *elem.DhcpOptionsId, dhcpOptionInternalID, aws_shared_model.AwsTypeDHCPOptions, agent.StatusNoStatus, "", crawlTime)

		returnedElems = append(returnedElems, agentElem)
	}

	return returnedElems, nil
}

func describeAllSubnets(session *session.Session, owner []*string, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	var returnedElems []*agent.Element

	svc := ec2.New(session)
	input := &ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("owner-id"),
				Values: owner,
			},
		},
	}

	result, err := svc.DescribeSubnets(input)
	if err != nil {
		return nil, err
	}

	for _, elem := range result.Subnets {
		subnetInternalID := cloudutils.CreateAWSInternalID(scopeID, *elem.SubnetId)
		agentElem, _ := utils.CreateElement(elem, *elem.SubnetId, subnetInternalID, aws_shared_model.AwsTypeSubnet, agent.StatusNoStatus, "", crawlTime)
		returnedElems = append(returnedElems, agentElem)

		rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *elem.VpcId), subnetInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			returnedElems = append(returnedElems, rel)
		}

		rel, errRel = utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *elem.OwnerId), subnetInternalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			returnedElems = append(returnedElems, rel)
		}
	}

	return returnedElems, nil
}

func describeNATGateways(session *session.Session, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	var returnedElems []*agent.Element

	svc := ec2.New(session)
	input := &ec2.DescribeNatGatewaysInput{}

	result, err := svc.DescribeNatGateways(input)
	if err != nil {
		return nil, err
	}

	for _, elem := range result.NatGateways {
		internalID := cloudutils.CreateAWSInternalID(scopeID, *elem.NatGatewayId)
		agentElem, _ := utils.CreateElement(elem, *elem.NatGatewayId, internalID, aws_shared_model.AwsTypeNatGw, agent.StatusNoStatus, "", crawlTime)
		returnedElems = append(returnedElems, agentElem)

		rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *elem.VpcId), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			returnedElems = append(returnedElems, rel)
		}

		rel, errRel = utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *elem.SubnetId), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			returnedElems = append(returnedElems, rel)
		}
	}

	return returnedElems, nil
}

func describeNetworkACLs(session *session.Session, owner []*string, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	var returnedElems []*agent.Element

	svc := ec2.New(session)
	input := &ec2.DescribeNetworkAclsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("owner-id"),
				Values: owner,
			},
		},
	}

	result, err := svc.DescribeNetworkAcls(input)
	if err != nil {
		return nil, err
	}

	for _, elem := range result.NetworkAcls {
		internalID := cloudutils.CreateAWSInternalID(scopeID, *elem.NetworkAclId)
		agentElem, _ := utils.CreateElement(elem, *elem.NetworkAclId, internalID, aws_shared_model.AwsTypeNetworkACL, agent.StatusNoStatus, "", crawlTime)
		returnedElems = append(returnedElems, agentElem)

		rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *elem.VpcId), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			returnedElems = append(returnedElems, rel)
		}
	}

	return returnedElems, nil
}

func describeAllAvailabilityZones(session *session.Session, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	var returnedElems []*agent.Element

	svc := ec2.New(session)
	input := &ec2.DescribeAvailabilityZonesInput{}

	result, err := svc.DescribeAvailabilityZones(input)
	if err != nil {
		return nil, err
	}

	for _, elem := range result.AvailabilityZones {
		internalID := cloudutils.CreateAWSInternalID(scopeID, *elem.ZoneId)
		agentElem, _ := utils.CreateElement(elem, *elem.ZoneName, internalID, aws_shared_model.AwsTypeAvailabilityZone, agent.StatusNoStatus, "", crawlTime)
		returnedElems = append(returnedElems, agentElem)

		rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *elem.RegionName), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			returnedElems = append(returnedElems, rel)
		}
	}

	return returnedElems, nil
}

func describeAllAMIs(session *session.Session, owner []*string, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	var returnedElems []*agent.Element

	svc := ec2.New(session)
	input := &ec2.DescribeImagesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("owner-id"),
				Values: owner,
			},
		},
	}

	result, err := svc.DescribeImages(input)
	if err != nil {
		return nil, err
	}

	for _, elem := range result.Images {
		imageInternalID := cloudutils.CreateAWSInternalID(scopeID, *elem.ImageId)
		agentElem, _ := utils.CreateElement(elem, *elem.Name, imageInternalID, aws_shared_model.AwsTypeAMI, agent.StatusNoStatus, "", crawlTime)
		returnedElems = append(returnedElems, agentElem)
	}

	return returnedElems, nil
}

func describeAllInstances(session *session.Session, owner []*string, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	var returnedElems []*agent.Element

	svc := ec2.New(session)
	input := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("owner-id"),
				Values: owner,
			},
		},
	}

	result, err := svc.DescribeInstances(input)
	if err != nil {
		return nil, err
	}

	for _, reservation := range result.Reservations {
		for _, elem := range reservation.Instances {
			if elem.VpcId == nil || *elem.VpcId == "" {
				continue
			}

			ec2Status := agent.StatusRed
			if *elem.State.Code == 16 {
				ec2Status = agent.StatusGreen
			}

			internalID := cloudutils.CreateAWSInternalID(scopeID, *elem.InstanceId)
			agentElem, _ := utils.CreateElement(elem, *elem.InstanceId, internalID, aws_shared_model.AwsTypeInstance, ec2Status, "", crawlTime)
			returnedElems = append(returnedElems, agentElem)

			rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *elem.VpcId), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				returnedElems = append(returnedElems, rel)
			}

			rel, errRel = utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *elem.SubnetId), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				returnedElems = append(returnedElems, rel)
			}

			rel, errRel = utils.CreateRelationship(internalID, cloudutils.CreateAWSInternalID(scopeID, *elem.ImageId), agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				returnedElems = append(returnedElems, rel)
			}

			for _, secGroup := range elem.SecurityGroups {
				rel, errRel := utils.CreateRelationship(internalID, cloudutils.CreateAWSInternalID(scopeID, *secGroup.GroupId), agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					returnedElems = append(returnedElems, rel)
				}
			}
		}
	}

	return returnedElems, nil
}

func describeAllSecurityGroups(session *session.Session, owner []*string, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	var returnedElems []*agent.Element

	svc := ec2.New(session)
	input := &ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("owner-id"),
				Values: owner,
			},
		},
	}

	result, err := svc.DescribeSecurityGroups(input)
	if err != nil {
		return nil, err
	}

	for _, elem := range result.SecurityGroups {
		internalID := cloudutils.CreateAWSInternalID(scopeID, *elem.GroupId)
		agentElem, _ := utils.CreateElement(elem, cloudutils.CreateAWSInternalID(scopeID, *elem.GroupName), internalID, aws_shared_model.AwsTypeSecGroup, agent.StatusNoStatus, "", crawlTime)
		returnedElems = append(returnedElems, agentElem)

		rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *elem.VpcId), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			returnedElems = append(returnedElems, rel)
		}
	}

	return returnedElems, nil
}

func describeAllVolumes(session *session.Session, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	var returnedElems []*agent.Element

	svc := ec2.New(session)
	input := &ec2.DescribeVolumesInput{}

	result, err := svc.DescribeVolumes(input)
	if err != nil {
		return nil, err
	}

	for _, elem := range result.Volumes {
		internalID := cloudutils.CreateAWSInternalID(scopeID, *elem.VolumeId)
		agentElem, _ := utils.CreateElement(elem, *elem.VolumeId, internalID, aws_shared_model.AwsTypeVolume, agent.StatusNoStatus, "", crawlTime)
		returnedElems = append(returnedElems, agentElem)

		for _, volumeAttachment := range elem.Attachments {
			rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *volumeAttachment.InstanceId), cloudutils.CreateAWSInternalID(scopeID, *volumeAttachment.VolumeId), agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				returnedElems = append(returnedElems, rel)
			}
		}
	}

	return returnedElems, nil
}

func describeAllLoadBalancers(session *session.Session, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	var returnedElems []*agent.Element

	svc := elbv2.New(session)
	input := &elbv2.DescribeLoadBalancersInput{}

	result, err := svc.DescribeLoadBalancers(input)
	if err != nil {
		return nil, err
	}

	for _, elem := range result.LoadBalancers {
		var lbType string

		if *elem.Type == elbv2.LoadBalancerTypeEnumApplication {
			lbType = aws_shared_model.AwsTypeApplicationLoadBalancer
		} else if *elem.Type == elbv2.LoadBalancerTypeEnumNetwork {
			lbType = aws_shared_model.AwsTypeNetworkLoadBalancer
		} else if *elem.Type == elbv2.LoadBalancerTypeEnumGateway {
			lbType = aws_shared_model.AwsTypeGatewayLoadBalancer
		}

		internalID := cloudutils.CreateAWSInternalID(scopeID, *elem.LoadBalancerArn)
		agentElem, _ := utils.CreateElement(elem, *elem.LoadBalancerName, internalID, lbType, agent.StatusNoStatus, "", crawlTime)

		relVpc, errRelVpc := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *elem.VpcId), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRelVpc == nil {
			returnedElems = append(returnedElems, relVpc)
		}

		for _, secGroupID := range elem.SecurityGroups {
			rel, errRel := utils.CreateRelationship(internalID, cloudutils.CreateAWSInternalID(scopeID, *secGroupID), agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				returnedElems = append(returnedElems, rel)
			}
		}

		for _, availabilityZone := range elem.AvailabilityZones {
			rel, errRel := utils.CreateRelationship(internalID, cloudutils.CreateAWSInternalID(scopeID, *availabilityZone.SubnetId), agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				returnedElems = append(returnedElems, rel)
			}
		}

		rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *elem.VpcId), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			returnedElems = append(returnedElems, rel)
		}

		returnedElems = append(returnedElems, agentElem)

		input := &elbv2.DescribeTargetGroupsInput{
			LoadBalancerArn: elem.LoadBalancerArn,
		}
		result, err := svc.DescribeTargetGroups(input)
		if err != nil {
			continue
		}

		for _, elbTargetGroup := range result.TargetGroups {
			input := &elbv2.DescribeTargetHealthInput{
				TargetGroupArn: elbTargetGroup.TargetGroupArn,
			}
			result, err := svc.DescribeTargetHealth(input)
			if err != nil {
				continue
			}

			if *elbTargetGroup.TargetType != elbv2.TargetTypeEnumInstance {
				continue
			}

			for _, targetHealthDescription := range result.TargetHealthDescriptions {
				loadBalancerTargetRelation := agent.RelationshipElement{
					SourceID:         internalID,
					DestinationID:    cloudutils.CreateAWSInternalID(scopeID, *targetHealthDescription.Target.Id),
					RelationshipType: aws_shared_model.AwsRelationshipTypeLoadBalancerV2Targets,
					RelationType:     agent.ParentChildTypeRelation,
				}

				dummyID := fmt.Sprintf("%s-%s", loadBalancerTargetRelation.SourceID, loadBalancerTargetRelation.DestinationID)

				agentElem, _ := utils.CreateElement(loadBalancerTargetRelation, dummyID, dummyID, aws_shared_model.AwsTypeLoadBalancerTargetsSkipinsert, agent.StatusNoStatus, "", crawlTime)

				// add ID-> loadbalancerarn and NAME->TargetGroupArn
				returnedElems = append(returnedElems, agentElem)
			}
		}
	}

	// describe classic LB
	svcElb := elb.New(session)
	inputElb := &elb.DescribeLoadBalancersInput{}

	resultElb, err := svcElb.DescribeLoadBalancers(inputElb)
	if err != nil {
		return nil, err
	}

	for _, elem := range resultElb.LoadBalancerDescriptions {
		internalID := cloudutils.CreateAWSInternalID(scopeID, *elem.DNSName)
		agentElem, _ := utils.CreateElement(elem, *elem.LoadBalancerName, internalID, aws_shared_model.AwsTypeClassicalLB, agent.StatusNoStatus, "", crawlTime)
		returnedElems = append(returnedElems, agentElem)

		rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *elem.VPCId), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			returnedElems = append(returnedElems, rel)
		}

		for _, instance := range elem.Instances {
			rel, errRel := utils.CreateRelationship(internalID, cloudutils.CreateAWSInternalID(scopeID, *instance.InstanceId), agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				returnedElems = append(returnedElems, rel)
			}
		}

		for _, secGroupID := range elem.SecurityGroups {
			rel, errRel := utils.CreateRelationship(internalID, cloudutils.CreateAWSInternalID(scopeID, *secGroupID), agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				returnedElems = append(returnedElems, rel)
			}
		}

		for _, subnet := range elem.Subnets {
			rel, errRel := utils.CreateRelationship(*elem.DNSName, cloudutils.CreateAWSInternalID(scopeID, *subnet), agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				returnedElems = append(returnedElems, rel)
			}
		}
	}

	return returnedElems, nil
}

func getAllS3Buckets(session *session.Session, owner []*string, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	var returnedElems []*agent.Element
	svc := s3.New(session)

	result, err := svc.ListBuckets(&s3.ListBucketsInput{})
	if err != nil {
		return nil, err
	}

	result.Owner.SetID(*owner[0])
	result.Owner.SetDisplayName(*owner[0])

	for _, elem := range result.Buckets {
		bucketList := &s3.ListBucketsOutput{
			Buckets: []*s3.Bucket{elem},
			Owner:   result.Owner,
		}
		internalID := cloudutils.CreateAWSInternalID(scopeID, *elem.Name)
		agentElem, _ := utils.CreateElement(bucketList, *elem.Name, internalID, aws_shared_model.AwsTypeS3Bucket, agent.StatusNoStatus, "", crawlTime)

		// add relationship between the owner and the bucket
		rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *owner[0]), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			returnedElems = append(returnedElems, rel)
		}

		returnedElems = append(returnedElems, agentElem)
	}

	return returnedElems, nil
}

func getAllLambdaFunctions(session *session.Session, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	var returnedElems []*agent.Element
	svc := lambda.New(session)

	result, err := svc.ListFunctions(nil)
	if err != nil {
		return returnedElems, err
	}

	for _, lambdaFun := range result.Functions {
		lambdaFunState := agent.StatusNoStatus

		if lambdaFun.State != nil {
			if *lambdaFun.State == lambda.StateActive {
				lambdaFunState = agent.StatusGreen
			} else if *lambdaFun.State == lambda.StateFailed || *lambdaFun.State == lambda.StateInactive {
				lambdaFunState = agent.StatusRed
			} else if *lambdaFun.State == lambda.StatePending {
				lambdaFunState = agent.StatusOrange
			}
		}

		internalID := cloudutils.CreateAWSInternalID(scopeID, *lambdaFun.FunctionArn)
		agentElem, _ := utils.CreateElement(lambdaFun, *lambdaFun.FunctionName, internalID, aws_shared_model.AwsTypeLambda, lambdaFunState, "", crawlTime)
		returnedElems = append(returnedElems, agentElem)

		if lambdaFun.VpcConfig == nil || *lambdaFun.VpcConfig.VpcId == "" {
			continue
		}

		rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *lambdaFun.VpcConfig.VpcId), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			returnedElems = append(returnedElems, rel)
		}

		for _, lambdaSubnetID := range lambdaFun.VpcConfig.SubnetIds {
			rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *lambdaSubnetID), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				returnedElems = append(returnedElems, rel)
			}
		}

		for _, lambdaSecGroupID := range lambdaFun.VpcConfig.SecurityGroupIds {
			rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *lambdaSecGroupID), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				returnedElems = append(returnedElems, rel)
			}
		}
	}

	return returnedElems, nil
}

func getAllRDSInstances(session *session.Session, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	var returnedElems []*agent.Element
	svc := rds.New(session)

	result, err := svc.DescribeDBInstances(nil)
	if err != nil {
		return returnedElems, err
	}

	for _, dbInstance := range result.DBInstances {
		greenStates := []string{"Available", "Configuring-enhanced-monitoring", "Configuring-iam-database-auth", "Resetting-master-credentials", "Renaming"}
		redStates := []string{"Restore-error", "Storage-full", "Failed", "Deleting", "Stopped", "Stopping"}
		orangeStates := []string{}
		dbStatus := agent.StatusNoStatus

		if slices.Contains(greenStates, *dbInstance.DBInstanceStatus) {
			dbStatus = agent.StatusGreen
		} else if slices.Contains(redStates, *dbInstance.DBInstanceStatus) {
			dbStatus = agent.StatusRed
		} else if slices.Contains(orangeStates, *dbInstance.DBInstanceStatus) {
			dbStatus = agent.StatusOrange
		}

		internalID := cloudutils.CreateAWSInternalID(scopeID, *dbInstance.Endpoint.Address)
		agentElem, _ := utils.CreateElement(dbInstance, internalID, internalID, aws_shared_model.AwsTypeRDS, dbStatus, "", crawlTime)
		returnedElems = append(returnedElems, agentElem)

		rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *dbInstance.AvailabilityZone), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			returnedElems = append(returnedElems, rel)
		}

		for _, dbSecGroup := range dbInstance.DBSecurityGroups {
			// FIXME: this does not work. We need to get the secGroupID
			rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *dbSecGroup.DBSecurityGroupName), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				returnedElems = append(returnedElems, rel)
			}
		}

		for _, dbSecGroup := range dbInstance.VpcSecurityGroups {
			rel, errRel := utils.CreateRelationship(internalID, cloudutils.CreateAWSInternalID(scopeID, *dbSecGroup.VpcSecurityGroupId), agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				returnedElems = append(returnedElems, rel)
			}
		}

		rel, errRel = utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *dbInstance.DBSubnetGroup.VpcId), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			returnedElems = append(returnedElems, rel)
		}

		for _, subnet := range dbInstance.DBSubnetGroup.Subnets {
			rel, errRel = utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *subnet.SubnetIdentifier), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				returnedElems = append(returnedElems, rel)
			}

			// FIXME: this will not work as we need the AZ ID
			rel, errRel = utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *subnet.SubnetAvailabilityZone.Name), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				returnedElems = append(returnedElems, rel)
			}
		}

	}

	return returnedElems, nil
}

func getAllAutoscalingGroups(session *session.Session, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	returnedElems := []*agent.Element{}

	autoScalingSvc := autoscaling.New(session)
	inputDescribeAutoscalingGroup := &autoscaling.DescribeAutoScalingGroupsInput{}

	describeAutoScalingGroupsResult, errDescribeAutoscalingGroups := autoScalingSvc.DescribeAutoScalingGroups(inputDescribeAutoscalingGroup)
	if errDescribeAutoscalingGroups != nil {
		return returnedElems, errDescribeAutoscalingGroups
	}

	for _, autoScalingGroup := range describeAutoScalingGroupsResult.AutoScalingGroups {
		internalID := cloudutils.CreateAWSInternalID(scopeID, *autoScalingGroup.AutoScalingGroupARN)
		elem, errElem := utils.CreateElement(autoScalingGroup, *autoScalingGroup.AutoScalingGroupName, internalID, aws_shared_model.AwsTypeAutoscalingGroup, agent.StatusNoStatus, "", crawlTime)
		if errElem != nil {
			continue
		}

		returnedElems = append(returnedElems, elem)

		for _, subnetID := range strings.Split(*autoScalingGroup.VPCZoneIdentifier, ",") {
			if strings.HasPrefix(*autoScalingGroup.VPCZoneIdentifier, "subnet") {
				svc := ec2.New(session)
				input := &ec2.DescribeSubnetsInput{
					Filters: []*ec2.Filter{
						{
							Name: aws.String("subnet-id"),
							Values: []*string{
								aws.String(subnetID),
							},
						},
					},
				}

				describedSubnets, errDescribeSubnet := svc.DescribeSubnets(input)
				if errDescribeSubnet == nil {
					for _, subnet := range describedSubnets.Subnets {
						rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *subnet.VpcId), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
						if errRel == nil {
							returnedElems = append(returnedElems, rel)
						}
					}
				}

				rel, errRel := utils.CreateRelationship(internalID, cloudutils.CreateAWSInternalID(scopeID, subnetID), agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					returnedElems = append(returnedElems, rel)
				}
			}
		}

		for _, instance := range autoScalingGroup.Instances {
			rel, errRel := utils.CreateRelationship(internalID, cloudutils.CreateAWSInternalID(scopeID, *instance.InstanceId), agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				returnedElems = append(returnedElems, rel)
			}
		}

		// create link with the load balancer
		elbV2Svc := elbv2.New(session)
		input := &elbv2.DescribeLoadBalancersInput{
			Names: autoScalingGroup.LoadBalancerNames,
		}

		resultElbV2, err := elbV2Svc.DescribeLoadBalancers(input)
		if err != nil {
			return nil, err
		}

		for _, elem := range resultElbV2.LoadBalancers {
			asgInstanceRelationship, errAsgInstanceRelationship := utils.CreateRelationship(
				internalID,
				cloudutils.CreateAWSInternalID(scopeID, *elem.LoadBalancerArn),
				aws_shared_model.AwsRelationshipSkipinsert,
				agent.ParentChildTypeRelation,
				crawlTime,
			)
			if errAsgInstanceRelationship != nil {
				continue
			}
			returnedElems = append(returnedElems, asgInstanceRelationship)
		}

		svcElb := elb.New(session)
		inputElb := &elb.DescribeLoadBalancersInput{}

		resultElb, err := svcElb.DescribeLoadBalancers(inputElb)
		if err != nil {
			return nil, err
		}

		for _, elem := range resultElb.LoadBalancerDescriptions {
			asgInstanceRelationship, errAsgInstanceRelationship := utils.CreateRelationship(
				internalID,
				cloudutils.CreateAWSInternalID(scopeID, *elem.DNSName),
				aws_shared_model.AwsRelationshipSkipinsert,
				agent.ParentChildTypeRelation,
				crawlTime,
			)
			if errAsgInstanceRelationship != nil {
				continue
			}
			returnedElems = append(returnedElems, asgInstanceRelationship)
		}

		// Add the relationship between the autoscaling group and the instance id
		for _, instance := range autoScalingGroup.Instances {
			asgInstanceRelationship, errAsgInstanceRelationship := utils.CreateRelationship(
				internalID,
				cloudutils.CreateAWSInternalID(scopeID, *instance.InstanceId),
				aws_shared_model.AwsTypeAutoscalingGroup,
				agent.ParentChildTypeRelation,
				crawlTime,
			)
			if errAsgInstanceRelationship != nil {
				continue
			}
			returnedElems = append(returnedElems, asgInstanceRelationship)
		}
	}

	return returnedElems, nil
}

func getAllEKSClusters(session *session.Session, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	var returnedElems []*agent.Element
	svc := eks.New(session)
	input := &eks.ListClustersInput{}

	result, err := svc.ListClusters(input)
	if err != nil {
		return returnedElems, err
	}

	for _, eksClusterName := range result.Clusters {
		input := &eks.DescribeClusterInput{
			Name: aws.String(*eksClusterName),
		}

		result, errDescribeCluster := svc.DescribeCluster(input)
		if errDescribeCluster != nil {
			return returnedElems, errDescribeCluster
		}

		internalID := cloudutils.CreateAWSInternalID(scopeID, *result.Cluster.Arn)
		agentElem, _ := utils.CreateElement(result.Cluster, *result.Cluster.Name, internalID, aws_shared_model.AwsTypeEKS, agent.StatusNoStatus, "", crawlTime)
		returnedElems = append(returnedElems, agentElem)

		rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *result.Cluster.ResourcesVpcConfig.VpcId), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			returnedElems = append(returnedElems, rel)
		}

		for _, subnetID := range result.Cluster.ResourcesVpcConfig.SubnetIds {
			rel, errRel := utils.CreateRelationship(internalID, cloudutils.CreateAWSInternalID(scopeID, *subnetID), agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
			if errRel == nil {
				returnedElems = append(returnedElems, rel)
			}
		}

		// list nodegroups of the cluster
		listNodeGroupInput := eks.ListNodegroupsInput{
			ClusterName: eksClusterName,
		}

		clusterNodeGroups, errClusterNodeGroups := svc.ListNodegroups(&listNodeGroupInput)
		if errClusterNodeGroups != nil {
			continue
		}

		for _, clusterNodeGroup := range clusterNodeGroups.Nodegroups {
			// get the nodegroup
			clusterNodeGroupInput := &eks.DescribeNodegroupInput{
				NodegroupName: clusterNodeGroup,
			}

			clusterNodeGroupInputResult, errClusterNodeGroupInput := svc.DescribeNodegroup(clusterNodeGroupInput)
			if errClusterNodeGroupInput != nil {
				continue
			}

			clusterNodeGroupInternalID := cloudutils.CreateAWSInternalID(scopeID, *clusterNodeGroupInput.NodegroupName)
			clusterNodeGroupElem, errClusterNodeGroupElem := utils.CreateAWSElement(
				clusterNodeGroupInputResult,
				clusterNodeGroupInternalID,
				clusterNodeGroupInternalID,
				aws_shared_model.AwsTypeEKSNodeGroup,
				agent.StatusNoStatus,
				"",
				crawlTime,
			)
			if errClusterNodeGroupElem == nil {
				returnedElems = append(returnedElems, clusterNodeGroupElem)
			}

			rel, errRel := utils.CreateRelationship(
				internalID,
				clusterNodeGroupInternalID,
				agent.RelationshipType,
				agent.ParentChildTypeRelation,
				crawlTime,
			)
			if errRel == nil {
				returnedElems = append(returnedElems, rel)
			}

			// get the autoscalinggroups of the nodegroup
			autoScalingSvc := autoscaling.New(session)
			autoScalingGroupNames := []*string{}
			for _, autoscalingGroup := range clusterNodeGroupInputResult.Nodegroup.Resources.AutoScalingGroups {
				autoScalingGroupNames = append(autoScalingGroupNames, autoscalingGroup.Name)
			}

			inputDescribeAutoscalingGroup := &autoscaling.DescribeAutoScalingGroupsInput{
				AutoScalingGroupNames: autoScalingGroupNames,
			}

			describeAutoScalingGroupsResult, errDescribeAutoscalingGroups := autoScalingSvc.DescribeAutoScalingGroups(inputDescribeAutoscalingGroup)
			if errDescribeAutoscalingGroups != nil {
				continue
			}
			for _, autoScalingGroup := range describeAutoScalingGroupsResult.AutoScalingGroups {
				relationshipAutoscalingNodegroupGroupElem, errRelationshipAutoscalingNodegroupNodeGroupElem := utils.CreateRelationship(
					internalID,
					cloudutils.CreateAWSInternalID(scopeID, *autoScalingGroup.AutoScalingGroupARN),
					aws_shared_model.AwsRelationshipSkipinsert,
					agent.ParentChildTypeRelation,
					crawlTime)
				if errRelationshipAutoscalingNodegroupNodeGroupElem != nil {
					continue
				}
				returnedElems = append(returnedElems, relationshipAutoscalingNodegroupGroupElem)
			}
		}

		// get all instances that have a tag alpha.eksctl.io/cluster-name = <EKS cluster name>
		ec2TagFilters := map[string]string{
			"alpha.eksctl.io/cluster-name": *eksClusterName,
		}

		eksInstances, errEksInstances := getFilteredEC2(session, ec2TagFilters)
		if errEksInstances != nil {
			continue
		}

		for _, eksInstance := range eksInstances {
			eksClusterInstanceRelationship, errEksClusterInstanceRelationship := utils.CreateRelationship(
				internalID,
				cloudutils.CreateAWSInternalID(scopeID, *eksInstance.InstanceId),
				aws_shared_model.AwsRelationshipSkipinsert,
				agent.ParentChildTypeRelation,
				crawlTime,
			)
			if errEksClusterInstanceRelationship != nil {
				continue
			}

			returnedElems = append(returnedElems, eksClusterInstanceRelationship)
		}
	}

	return returnedElems, nil
}

func getAllECRReposAndImages(session *session.Session, scopeID string, crawlTime time.Time) ([]*agent.Element, error) {
	var returnedElems []*agent.Element

	svc := ecr.New(session)
	input := &ecr.DescribeRepositoriesInput{}

	ecrRepos, errDescribeRepositories := svc.DescribeRepositories(input)
	if errDescribeRepositories != nil {
		return returnedElems, errDescribeRepositories
	}

	for _, ecrRepo := range ecrRepos.Repositories {

		internalID := cloudutils.CreateAWSInternalID(scopeID, *ecrRepo.RepositoryUri)
		agentElem, _ := utils.CreateElement(ecrRepo, *ecrRepo.RepositoryName, internalID, aws_shared_model.AwsTypeECRRepository, agent.StatusNoStatus, "", crawlTime)

		returnedElems = append(returnedElems, agentElem)

		rel, errRel := utils.CreateRelationship(cloudutils.CreateAWSInternalID(scopeID, *ecrRepo.RegistryId), internalID, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
		if errRel == nil {
			returnedElems = append(returnedElems, rel)
		}

		svc := ecr.New(session)
		input := &ecr.ListImagesInput{
			RepositoryName: aws.String(*ecrRepo.RepositoryName),
		}

		repoImages, errListImages := svc.ListImages(input)
		if errListImages != nil {
			continue
		}

		describeImagesInput := &ecr.DescribeImagesInput{
			ImageIds:       repoImages.ImageIds,
			RepositoryName: ecrRepo.RepositoryName,
			RegistryId:     ecrRepo.RegistryId,
		}

		describedRepoImages, errDescribedRepoImages := svc.DescribeImages(describeImagesInput)
		if errDescribedRepoImages != nil {
			continue
		}

		for _, repoImage := range describedRepoImages.ImageDetails {
			if len(repoImage.ImageTags) == 0 {
				continue
			}

			for _, imageTag := range repoImage.ImageTags {
				imageInternalID := cloudutils.CreateAWSInternalID(scopeID, fmt.Sprintf("%s:%s", *ecrRepo.RepositoryUri, *imageTag))

				agentElem, _ := utils.CreateElement(repoImage, imageInternalID, imageInternalID, aws_shared_model.AwsTypeECRRepositoryImage, agent.StatusNoStatus, "", crawlTime)

				returnedElems = append(returnedElems, agentElem)

				relationshipECRRepoImageElem, errRelationshipECRRepoImageElem := utils.CreateRelationship(
					internalID,
					imageInternalID,
					aws_shared_model.AwsRelationshipSkipinsert,
					agent.ParentChildTypeRelation,
					crawlTime)
				if errRelationshipECRRepoImageElem == nil {
					returnedElems = append(returnedElems, relationshipECRRepoImageElem)
				}

				// TODO: this probably does not result in any relationship. Check again.
				rel, errRel := utils.CreateRelationship(imageInternalID, *imageTag, agent.RelationshipType, agent.ParentChildTypeRelation, crawlTime)
				if errRel == nil {
					returnedElems = append(returnedElems, rel)
				}
			}

		}

		for _, repoImage := range repoImages.ImageIds {
			if repoImage.ImageTag == nil {
				continue
			}
		}

	}

	return returnedElems, nil
}
