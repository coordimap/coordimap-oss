package aws

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func getFilteredEC2(session *session.Session, tags map[string]string) ([]*ec2.Instance, error) {
	foundInstances := []*ec2.Instance{}
	svc := ec2.New(session)

	filters := []*ec2.Filter{}

	for key, value := range tags {
		filters = append(filters, &ec2.Filter{
			Name: aws.String(fmt.Sprintf("tag:%s", key)),
			Values: []*string{
				aws.String(value),
			},
		})
	}

	params := &ec2.DescribeInstancesInput{
		Filters: filters,
	}

	res, errDescribe := svc.DescribeInstances(params)
	if errDescribe != nil {
		return foundInstances, errDescribe
	}

	for resIdx := range res.Reservations {
		for _, i := range res.Reservations[resIdx].Instances {
			foundInstances = append(foundInstances, i)
		}
	}

	return foundInstances, nil
}
