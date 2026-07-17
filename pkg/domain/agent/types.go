package agent

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	RelationshipType                        = "coordimap.relationship_skipinsert"
	MetricTriggerElementType                = "coordimap.metric_trigger"
	RelationshipExternalBothSidesType       = "coordimap.relationship.external.both_sides"
	RelationshipExternalSourceSideType      = "coordimap.relationship.external.source_side"
	RelationshipExternalDestinationSideType = "coordimap.relationship.external.destination_side"
)

const (
	ParentChildTypeRelation          = 3
	ErTypeRelation                   = 4
	GenericFlowTypeRelation          = 100
	GCPNetworkFlowTypeRelation       = 101
	KubernetesRetinaFlowTypeRelation = 102
	KubernetesIstioFlowTypeRelation  = 103
	EBPFFlowTypeRelation             = 104
	AWSNetworkFlowTypeRelation       = 105
)

const (
	StatusNoStatus = "NoStatus"
	StatusRed      = "Red"
	StatusGreen    = "Green"
	StatusOrange   = "Orange"
)

// Element represents a single retrieved AWS element stored as JSON together with it's corresponding HASH and timestamp of retrieval
type Element struct {
	RetrievedAt time.Time `json:"retrieved_at"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	ID          string    `json:"id"`
	Hash        string    `json:"hash"`
	Data        []byte    `json:"data"`
	IsJSONData  bool      `json:"is_json_data"`
	Version     string    `json:"version"`
	Status      string    `json:"status"`
}

func (element *Element) String() string {
	return fmt.Sprintf("[Element] ID: %s -- Name: %s -- Type: %s -- Hash: %s", element.ID, element.Name, element.Type, element.Hash)
}

type coordimapElements []*Element

func (elements coordimapElements) String() {
	for _, element := range elements {
		fmt.Printf("[Element] ID: %s -- Name: %s -- Version: %s -- Type: %s -- Hash: %s", element.ID, element.Name, element.Version, element.Type, element.Hash)
	}
}

// CrawledData all the crawled elements of the specific cloud
type CrawledData struct {
	Data []*Element `json:"data"`
}

type DataSource struct {
	Info         DataSourceInfo   `json:"data_source_info"`
	DataSourceID string           `json:"data_source_id"`
	Config       DataSourceConfig `json:"data_source_config"`
}

// DataSourceInfo Structure that holds generic information regarding the data source that is being crawled.
type DataSourceInfo struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
	Type string `json:"type"`
}

// DataSourceConfig Structure that holds information about the configuration of data source that to be crawled.
// Any keys that contain "password" or "secret" will not be transferred.
type DataSourceConfig struct {
	ValuePairs  []KeyValue   `json:"value_pairs"`
	MetricRules []MetricRule `json:"metric_rules,omitempty"`
}

// MetricRule is the datasource-level metric rule definition sent to the backend.
type MetricRule struct {
	ID                 string              `json:"id" yaml:"id"`
	Name               string              `json:"name" yaml:"name"`
	Provider           string              `json:"provider" yaml:"provider"`
	Query              string              `json:"query,omitempty" yaml:"query,omitempty"`
	Filter             string              `json:"filter,omitempty" yaml:"filter,omitempty"`
	MetricType         string              `json:"metric_type,omitempty" yaml:"metric_type,omitempty"`
	Lookback           string              `json:"lookback,omitempty" yaml:"lookback,omitempty"`
	AlignmentPeriod    string              `json:"alignment_period,omitempty" yaml:"alignment_period,omitempty"`
	PerSeriesAligner   string              `json:"per_series_aligner,omitempty" yaml:"per_series_aligner,omitempty"`
	CrossSeriesReducer string              `json:"cross_series_reducer,omitempty" yaml:"cross_series_reducer,omitempty"`
	GroupByFields      []string            `json:"group_by_fields,omitempty" yaml:"group_by_fields,omitempty"`
	Threshold          MetricRuleThreshold `json:"threshold" yaml:"threshold"`
	Target             MetricRuleTarget    `json:"target" yaml:"target"`
	Enabled            *bool               `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}

type MetricRuleThreshold struct {
	Operator string  `json:"operator" yaml:"operator"`
	Value    float64 `json:"value" yaml:"value"`
}

type MetricRuleTarget struct {
	Resolver           string `json:"resolver" yaml:"resolver"`
	NamespaceLabel     string `json:"namespace_label,omitempty" yaml:"namespace_label,omitempty"`
	NameLabel          string `json:"name_label,omitempty" yaml:"name_label,omitempty"`
	ZoneLabel          string `json:"zone_label,omitempty" yaml:"zone_label,omitempty"`
	RegionLabel        string `json:"region_label,omitempty" yaml:"region_label,omitempty"`
	MappingKeyTemplate string `json:"mapping_key_template,omitempty" yaml:"mapping_key_template,omitempty"`
	MappingValueType   string `json:"mapping_value_type,omitempty" yaml:"mapping_value_type,omitempty"`
}

// CloudData contains all the crawled resources of the cloud Type
type CloudData struct {
	Data            json.RawMessage `json:"crawled_data"`
	Hash            string          `json:"hash"`
	Timestamp       time.Time       `json:"timestamp"`
	CrawlInternalID string          `json:"crawl_internal_id"`
}

// CloudCrawlData the data structure that holds all the crawled information about the cloud
type CloudCrawlData struct {
	DataSource      DataSource  `json:"data_source"`
	CrawledData     CrawledData `json:"crawled_data"`
	Timestamp       time.Time   `json:"timestamp"`
	CrawlInternalID string      `json:"crawl_internal_id"`
}

type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type RelationshipElement struct {
	SourceID         string `json:"source_id"`
	DestinationID    string `json:"destination_id"`
	RelationshipType string `json:"relationship_type"`
	RelationType     int    `json:"relation_type"`
}
