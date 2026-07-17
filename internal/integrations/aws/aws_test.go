package aws

import (
	"testing"
	"time"

	"github.com/coordimap/agent/pkg/domain/agent"
)

func TestAwsCrawl_GetCrawlInterval(t *testing.T) {
	type fields struct {
		ds *agent.DataSource
	}
	tests := []struct {
		fields  fields
		name    string
		want    time.Duration
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aws := &AwsCrawl{
				ds: tt.fields.ds,
			}
			got, err := aws.GetCrawlInterval()
			if (err != nil) != tt.wantErr {
				t.Errorf("AwsCrawl.GetCrawlInterval() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("AwsCrawl.GetCrawlInterval() = %v, want %v", got, tt.want)
			}
		})
	}
}
