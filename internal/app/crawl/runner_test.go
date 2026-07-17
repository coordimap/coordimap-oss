package crawl

import (
	"testing"

	"github.com/coordimap/agent/pkg/domain/agent"
)

func TestRunnerRunDoesNotStartDuplicateCrawlers(t *testing.T) {
	runner := NewRunner(map[string][]*agent.DataSource{}, make(chan *agent.CloudCrawlData, 1))
	if err := runner.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	firstID, alreadyRunning, err := runner.Run("")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !alreadyRunning || firstID == "" {
		t.Errorf("Run() = (%q, %t), want stable already-running ID", firstID, alreadyRunning)
	}
	secondID, alreadyRunning, err := runner.Run("")
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if !alreadyRunning || secondID != firstID {
		t.Errorf("second Run() = (%q, %t), want (%q, true)", secondID, alreadyRunning, firstID)
	}
}

func TestRunnerRunRejectsUnknownDataSource(t *testing.T) {
	runner := NewRunner(map[string][]*agent.DataSource{}, make(chan *agent.CloudCrawlData, 1))
	if _, _, err := runner.Run("missing"); err == nil {
		t.Fatal("Run(missing) error = nil, want unknown data source error")
	}
}
