package aws

import (
	"encoding/json"
	"time"
)

// Element represents a single retrieved AWS element stored as JSON together with it's corresponding HASH and timestamp of retrieval
type Element struct {
	RetrievedAt time.Time       `json:"retrieved_at"`
	Name        string          `json:"name"`
	ID          string          `json:"id"`
	Hash        string          `json:"hash"`
	Data        json.RawMessage `json:"data"`
}

// CrawledDataAWS Structure that holds all the information that was able to be retrieved from the cloud
type CrawledDataAWS struct {
	Vpcs              []*Element `json:"vpcs"`
	DhcpOptions       []*Element `json:"dhcp_options"`
	Subnets           []*Element `json:"subnets"`
	AvailabilityZones []*Element `json:"availability_zones"`
	NatGateways       []*Element `json:"nat_gateways"`
	NetworkAcls       []*Element `json:"network_acls"`
	Regions           []*Element `json:"regions"`
	RouteTables       []*Element `json:"route_tables"`
	SecurityGroups    []*Element `json:"security_groups"`
	Instances         []*Element `json:"instances"`
	Volumes           []*Element `json:"volumes"`
	AMIs              []*Element `json:"amis"`
	LoadBalancers     []*Element `json:"load_balancers"`
}
