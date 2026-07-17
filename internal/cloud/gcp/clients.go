package gcp

import (
	"context"

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/container/v1"
	"google.golang.org/api/option"
)

func createComputeClient(opts []option.ClientOption) (*compute.Service, error) {
	client, errClient := compute.NewService(context.Background(), opts...)
	if errClient != nil {
		return nil, errClient
	}
	return client, nil
}

func createContainerClient(opts []option.ClientOption) (*container.Service, error) {
	client, errClient := container.NewService(context.Background(), opts...)
	if errClient != nil {
		return nil, errClient
	}

	return client, nil
}
