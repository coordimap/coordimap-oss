package awsflowlogs

const (
	AWS_FLOW_LOGS_TYPE_EC2_SKIPINSERT = "awsflowlogs.ec2_skipinsert"
	AWS_FLOW_LOGS_TYPE_VPC_SKIPINSERT = "awsflowlogs.vpc_skipinsert"
)

type AWSFlowLog struct {
	Version          string `json:"version"`
	AccountID        string `json:"account_id"`
	InterfaceID      string `json:"interface_id"`
	SrcAddr          string `json:"src_addr"`
	SrcPort          string `json:"src_port"`
	DstAddr          string `json:"dst_addr"`
	DstPort          string `json:"dst_port"`
	Protocol         string `json:"protocol"`
	Packets          string `json:"packets"`
	Bytes            string `json:"bytes"`
	Start            string `json:"start"`
	End              string `json:"end"`
	Action           string `json:"action"`
	LogStatus        string `json:"log_status"`
	VpcID            string `json:"vpc_id"`
	SubnetID         string `json:"subnet_id"`
	InstanceID       string `json:"instance_id"`
	TCPFlags         string `json:"tcp_flags"`
	Type             string `json:"type"`
	PktSrcAddr       string `json:"pkt_src_addr"`
	PktDstAddr       string `json:"pkt_dst_addr"`
	Region           string `json:"region"`
	AZID             string `json:"az_id"`
	SublocationType  string `json:"sublocation_type"`
	SublocationID    string `json:"sublocation_id"`
	PktSrcAWSService string `json:"pkt_src_aws_service"`
	PktDstAWSService string `json:"pkt_dst_aws_service"`
	FlowDirection    string `json:"flow_direction"`
	TrafficPath      string `json:"traffic_path"`
}

type FlowRelation struct {
	Src *AWSFlowLog `json:"source"`
	Dst *AWSFlowLog `json:"destination"`
}
