package collector

import (
	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/coordimap/agent/pkg/domain/generic"
)

type TenantInfo struct {
	TenantID     string `json:"tenant_id"`
	DataSourceID string `json:"data_source_id"`
}

type CloudAsset struct {
	TenantInfo     TenantInfo           `json:"tenant_info"`
	DataSourceInfo agent.DataSourceInfo `json:"data_source_info"`
	Element        agent.Element        `json:"element"`
}

type TenantDataSource struct {
	TenantID     string `json:"tenant_id"`
	DataSourceID string `json:"data_source_id"`
}

type AddCrawledInfraFromAgentRequest struct {
	CloudCrawlData agent.CloudCrawlData `json:"cloud_crawl_data"`
}

type AddCrawledInfraFromAgentResponse struct {
	Status generic.ResponseStatus `json:"status"`
}
