package dedup

import (
	"encoding/json"
	"fmt"

	"github.com/coordimap/agent/pkg/domain/agent"
)

type Result struct {
	CloudCrawlData         *agent.CloudCrawlData
	InputCount             int
	OutputCount            int
	DroppedAssetDuplicates int
	DroppedRelDuplicates   int
	ConflictCount          int
}

func CloudCrawlData(input *agent.CloudCrawlData) Result {
	if input == nil {
		return Result{}
	}

	output := &agent.CloudCrawlData{
		Timestamp:       input.Timestamp,
		DataSource:      input.DataSource,
		CrawlInternalID: input.CrawlInternalID,
		CrawledData: agent.CrawledData{
			Data: make([]*agent.Element, 0, len(input.CrawledData.Data)),
		},
	}

	assetKeys := make(map[string]string, len(input.CrawledData.Data))
	relKeys := make(map[string]struct{}, len(input.CrawledData.Data))

	result := Result{
		CloudCrawlData: output,
		InputCount:     len(input.CrawledData.Data),
	}

	for _, elem := range input.CrawledData.Data {
		if elem == nil {
			continue
		}

		if elem.Type == agent.RelationshipType {
			relKey, ok := relationshipKey(elem)
			if ok {
				if _, exists := relKeys[relKey]; exists {
					result.DroppedRelDuplicates++
					continue
				}

				relKeys[relKey] = struct{}{}
				output.CrawledData.Data = append(output.CrawledData.Data, elem)
				continue
			}
		}

		assetKey := fmt.Sprintf("%s\x00%s", elem.Type, elem.ID)
		if existingHash, exists := assetKeys[assetKey]; exists {
			result.DroppedAssetDuplicates++
			if existingHash != elem.Hash {
				result.ConflictCount++
			}
			continue
		}

		assetKeys[assetKey] = elem.Hash
		output.CrawledData.Data = append(output.CrawledData.Data, elem)
	}

	result.OutputCount = len(output.CrawledData.Data)

	return result
}

func relationshipKey(elem *agent.Element) (string, bool) {
	var rel agent.RelationshipElement
	if err := json.Unmarshal(elem.Data, &rel); err != nil {
		return "", false
	}

	return fmt.Sprintf("%s\x00%s\x00%s\x00%d", rel.SourceID, rel.DestinationID, rel.RelationshipType, rel.RelationType), true
}
