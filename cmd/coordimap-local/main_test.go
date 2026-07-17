package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coordimap/agent/internal/app/ingest"
	"github.com/coordimap/agent/internal/storage/sqlite"
	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/coordimap/agent/pkg/utils"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

func TestLocalBinaryServesPersistedInventoryOverStdio(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tempDir := t.TempDir()
	databasePath := filepath.Join(tempDir, "coordimap.db")
	store, err := sqlite.Open("file:" + databasePath)
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		_ = store.Close()
		t.Fatalf("store.Migrate() error = %v", err)
	}
	asset, err := utils.CreateElement(map[string]string{"kind": "seed"}, "Seed asset", "seed-asset", "test.asset", agent.StatusNoStatus, "v1", time.Now().UTC())
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	relationship, err := utils.CreateRelationship("seed-asset", "remote-asset", "flows_to", agent.GenericFlowTypeRelation, time.Now().UTC())
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := ingest.NewService(store).StoreCrawl(ctx, agent.CloudCrawlData{
		DataSource:  agent.DataSource{DataSourceID: "seed-source", Info: agent.DataSourceInfo{Name: "Seed source", Desc: "Seed source", Type: "test"}},
		CrawledData: agent.CrawledData{Data: []*agent.Element{asset, relationship}},
		Timestamp:   time.Now().UTC(),
	}); err != nil {
		_ = store.Close()
		t.Fatalf("StoreCrawl() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("coordimap:\n  database:\n    driver: sqlite\n    connection_string: file:"+databasePath+"\n  data_sources: []\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	binaryPath := filepath.Join(tempDir, "coordimap-local")
	build := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build error = %v\n%s", err, output)
	}

	mcpClient := client.NewClient(transport.NewStdio(binaryPath, nil, "--config", configPath))
	if err := mcpClient.Start(ctx); err != nil {
		t.Fatalf("client.Start() error = %v", err)
	}
	defer mcpClient.Close()
	initialize := mcpgo.InitializeRequest{}
	initialize.Params.ProtocolVersion = mcpgo.LATEST_PROTOCOL_VERSION
	initialize.Params.ClientInfo = mcpgo.Implementation{Name: "coordimap-local-test", Version: "1.0"}
	initialize.Params.Capabilities = mcpgo.ClientCapabilities{}
	if _, err := mcpClient.Initialize(ctx, initialize); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	searchRequest := mcpgo.CallToolRequest{Params: mcpgo.CallToolParams{Name: "coordimap_search_assets", Arguments: map[string]any{"query": "seed"}}}
	searchResult, err := mcpClient.CallTool(ctx, searchRequest)
	if err != nil {
		t.Fatalf("CallTool(search) error = %v", err)
	}
	assertMCPResultContains(t, searchResult, "seed-asset")

	assetRequest := mcpgo.CallToolRequest{Params: mcpgo.CallToolParams{Name: "coordimap_get_asset", Arguments: map[string]any{"internal_id": "seed-asset"}}}
	assetResult, err := mcpClient.CallTool(ctx, assetRequest)
	if err != nil {
		t.Fatalf("CallTool(get asset) error = %v", err)
	}
	assertMCPResultContains(t, assetResult, "raw_json")

	resource, err := mcpClient.ReadResource(ctx, mcpgo.ReadResourceRequest{Params: mcpgo.ReadResourceParams{URI: "coordimap://relationships/seed-asset"}})
	if err != nil {
		t.Fatalf("ReadResource() error = %v", err)
	}
	encoded, err := json.Marshal(resource)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), "remote-asset") {
		t.Errorf("ReadResource() = %s, want relationship endpoint", encoded)
	}
}

func assertMCPResultContains(t *testing.T, result *mcpgo.CallToolResult, want string) {
	t.Helper()
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(string(encoded), want) {
		t.Errorf("tool result = %s, want non-error result containing %q", encoded, want)
	}
}
