package dedup

import (
	"testing"
	"time"

	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/coordimap/agent/pkg/domain/metrictrigger"
	"github.com/coordimap/agent/pkg/utils"
)

func TestCloudCrawlDataDeduplicatesAssetsByTypeAndID(t *testing.T) {
	now := time.Now().UTC()
	first, err := utils.CreateElement(map[string]string{"name": "a"}, "asset", "id-1", "test.asset", agent.StatusNoStatus, "", now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := utils.CreateElement(map[string]string{"name": "a"}, "asset", "id-1", "test.asset", agent.StatusNoStatus, "", now)
	if err != nil {
		t.Fatal(err)
	}

	result := CloudCrawlData(&agent.CloudCrawlData{CrawledData: agent.CrawledData{Data: []*agent.Element{first, second}}})

	if result.OutputCount != 1 {
		t.Fatalf("expected 1 output element, got %d", result.OutputCount)
	}
	if result.DroppedAssetDuplicates != 1 {
		t.Fatalf("expected 1 dropped asset duplicate, got %d", result.DroppedAssetDuplicates)
	}
	if result.CloudCrawlData.CrawledData.Data[0] != first {
		t.Fatal("expected first asset to win")
	}
}

func TestCloudCrawlDataCountsAssetConflicts(t *testing.T) {
	now := time.Now().UTC()
	first, err := utils.CreateElement(map[string]string{"name": "a"}, "asset", "id-1", "test.asset", agent.StatusNoStatus, "", now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := utils.CreateElement(map[string]string{"name": "b"}, "asset", "id-1", "test.asset", agent.StatusNoStatus, "", now)
	if err != nil {
		t.Fatal(err)
	}

	result := CloudCrawlData(&agent.CloudCrawlData{CrawledData: agent.CrawledData{Data: []*agent.Element{first, second}}})

	if result.ConflictCount != 1 {
		t.Fatalf("expected 1 conflict, got %d", result.ConflictCount)
	}
}

func TestCloudCrawlDataKeepsDifferentTypesWithSameID(t *testing.T) {
	now := time.Now().UTC()
	first, err := utils.CreateElement(map[string]string{"name": "a"}, "asset", "id-1", "type.a", agent.StatusNoStatus, "", now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := utils.CreateElement(map[string]string{"name": "b"}, "asset", "id-1", "type.b", agent.StatusNoStatus, "", now)
	if err != nil {
		t.Fatal(err)
	}

	result := CloudCrawlData(&agent.CloudCrawlData{CrawledData: agent.CrawledData{Data: []*agent.Element{first, second}}})

	if result.OutputCount != 2 {
		t.Fatalf("expected 2 output elements, got %d", result.OutputCount)
	}
}

func TestCloudCrawlDataDeduplicatesRelationships(t *testing.T) {
	now := time.Now().UTC()
	first, err := utils.CreateRelationship("src", "dst", agent.RelationshipType, agent.ParentChildTypeRelation, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := utils.CreateRelationship("src", "dst", agent.RelationshipType, agent.ParentChildTypeRelation, now)
	if err != nil {
		t.Fatal(err)
	}

	result := CloudCrawlData(&agent.CloudCrawlData{CrawledData: agent.CrawledData{Data: []*agent.Element{first, second}}})

	if result.OutputCount != 1 {
		t.Fatalf("expected 1 output element, got %d", result.OutputCount)
	}
	if result.DroppedRelDuplicates != 1 {
		t.Fatalf("expected 1 dropped relationship duplicate, got %d", result.DroppedRelDuplicates)
	}
}

func TestCloudCrawlDataKeepsRelationshipsWithDifferentRelationTypes(t *testing.T) {
	now := time.Now().UTC()
	first, err := utils.CreateRelationship("src", "dst", agent.RelationshipType, agent.ParentChildTypeRelation, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := utils.CreateRelationship("src", "dst", agent.RelationshipType, agent.ErTypeRelation, now)
	if err != nil {
		t.Fatal(err)
	}

	result := CloudCrawlData(&agent.CloudCrawlData{CrawledData: agent.CrawledData{Data: []*agent.Element{first, second}}})

	if result.OutputCount != 2 {
		t.Fatalf("expected 2 output elements, got %d", result.OutputCount)
	}
}

func TestCloudCrawlDataFallsBackForMalformedRelationshipPayload(t *testing.T) {
	now := time.Now().UTC()
	first := &agent.Element{Type: agent.RelationshipType, ID: "bad-1", Data: []byte("not-json"), RetrievedAt: now}
	second := &agent.Element{Type: agent.RelationshipType, ID: "bad-1", Data: []byte("also-not-json"), RetrievedAt: now}

	result := CloudCrawlData(&agent.CloudCrawlData{CrawledData: agent.CrawledData{Data: []*agent.Element{first, second}}})

	if result.OutputCount != 1 {
		t.Fatalf("expected 1 output element, got %d", result.OutputCount)
	}
}

func TestCloudCrawlDataPreservesDifferentMetricTriggers(t *testing.T) {
	now := time.Now().UTC()
	first, err := utils.CreateMetricTriggerElement(metrictrigger.Trigger{Targets: []metrictrigger.Target{{InternalID: "a"}}}, "trigger", "t-1", now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := utils.CreateMetricTriggerElement(metrictrigger.Trigger{Targets: []metrictrigger.Target{{InternalID: "b"}}}, "trigger", "t-2", now)
	if err != nil {
		t.Fatal(err)
	}

	result := CloudCrawlData(&agent.CloudCrawlData{CrawledData: agent.CrawledData{Data: []*agent.Element{first, second}}})

	if result.OutputCount != 2 {
		t.Fatalf("expected 2 output elements, got %d", result.OutputCount)
	}
}
