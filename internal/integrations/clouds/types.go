package clouds

import (
	"encoding/json"
	"time"
)

//Element represents a single retrieved AWS element stored as JSON together with it's corresponding HASH and timestamp of retrieval
type Element struct {
	RetrievedAt time.Time       `json:"retrieved_at"`
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	ID          string          `json:"id"`
	Hash        string          `json:"hash"`
	Data        json.RawMessage `json:"data"`
}

//CrawledData all the crawled elements of the specific cloud
type CrawledData struct {
	Data []*Element `json:"data"`
}

//CloudInformation Structure that holds information about the cloud account
type CloudInformation struct {
	AccountID string `json:"account_id"`
	Version   string `json:"version"`
	Type      string `json:"type"`
	Name      string `json:"name"`
}

//CloudData contains all the crawled resources of the cloud Type
type CloudData struct {
	Data      json.RawMessage `json:"crawled_data"`
	Hash      string          `json:"hash"`
	Timestamp time.Time       `josn:"timestamp"`
}

//CloudCrawlData the data structure that holds all the crawled information about the cloud
type CloudCrawlData struct {
	CloudInfo CloudInformation `json:"cloud_info"`
	Data      CloudData        `json:"data"`
	Timestamp time.Time        `json:"timestamp"`
}
