package aws

import (
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/coordimap/agent/pkg/domain/agent"
	aws_shared_model "github.com/coordimap/agent/pkg/domain/aws"
)

func worker(whatToCrawl string, owner []*string, regionSession *session.Session, results chan<- []*agent.Element, wg *sync.WaitGroup, dataSourceID string, crawlTime time.Time) {
	defer wg.Done()

	var res []*agent.Element
	var err error

	switch whatToCrawl {
	case "vpcs":
		res, _ = describeAllVPCs(regionSession, owner, dataSourceID, crawlTime)

	case "route_tables":
		res, _ = describeAllRouteTables(regionSession, owner, dataSourceID, crawlTime)

	case "dhcp_options":
		res, _ = describeAllDHCPOptions(regionSession, owner, dataSourceID, crawlTime)

	case "subnets":
		res, _ = describeAllSubnets(regionSession, owner, dataSourceID, crawlTime)

	case "natgws":
		res, _ = describeNATGateways(regionSession, dataSourceID, crawlTime)

	case "net_acls":
		res, _ = describeNetworkACLs(regionSession, owner, dataSourceID, crawlTime)

	case "azs":
		res, _ = describeAllAvailabilityZones(regionSession, dataSourceID, crawlTime)

	case "amis":
		res, _ = describeAllAMIs(regionSession, owner, dataSourceID, crawlTime)

	case "ec2":
		res, _ = describeAllInstances(regionSession, owner, dataSourceID, crawlTime)

	case "sec_groups":
		res, _ = describeAllSecurityGroups(regionSession, owner, dataSourceID, crawlTime)

	case "vols":
		res, _ = describeAllVolumes(regionSession, dataSourceID, crawlTime)

	case "lbs":
		res, _ = describeAllLoadBalancers(regionSession, dataSourceID, crawlTime)

	case "s3-buckets":
		res, _ = getAllS3Buckets(regionSession, owner, dataSourceID, crawlTime)

	case "lambdas":
		res, _ = getAllLambdaFunctions(regionSession, dataSourceID, crawlTime)

	case "rds":
		res, _ = getAllRDSInstances(regionSession, dataSourceID, crawlTime)

	case aws_shared_model.AwsTypeEKS:
		res, _ = getAllEKSClusters(regionSession, dataSourceID, crawlTime)

	case aws_shared_model.AwsTypeECRRepository:
		res, _ = getAllECRReposAndImages(regionSession, dataSourceID, crawlTime)

	case aws_shared_model.AwsTypeAutoscalingGroup:
		res, _ = getAllAutoscalingGroups(regionSession, dataSourceID, crawlTime)

	default:
		fmt.Println("notnig")

	}

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
	}

	results <- res
}
