package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/coordimap/agent/internal/app/ingest"
	"github.com/coordimap/agent/internal/storage/sqlite"
	"github.com/coordimap/agent/pkg/domain/agent"
	"github.com/coordimap/agent/pkg/utils"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

type testRunner struct {
	runID   string
	running bool
	err     error
}

func (r testRunner) Run(string) (string, bool, error) { return r.runID, r.running, r.err }

func TestServerToolsAndResources(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open("file:coordimap_mcp_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("store.Migrate() error = %v", err)
	}

	now := time.Now().UTC()
	jsonAsset, err := utils.CreateElement(map[string]string{"kind": "json"}, "Seed Asset", "asset-1", "test.asset", agent.StatusGreen, "v1", now)
	if err != nil {
		t.Fatal(err)
	}
	relationship, err := utils.CreateRelationship("asset-1", "binary-1", "contains", agent.ParentChildTypeRelation, now)
	if err != nil {
		t.Fatal(err)
	}
	binaryAsset := &agent.Element{ID: "binary-1", Name: "Binary Asset", Type: "test.binary", Hash: "binary-hash", Data: []byte{0, 1, 2}, RetrievedAt: now, Version: "v1", Status: agent.StatusNoStatus}
	ambiguousOne := &agent.Element{ID: "ambiguous", Name: "Ambiguous", Type: "test.one", Hash: "one", Data: []byte(`{"one":true}`), IsJSONData: true, RetrievedAt: now, Version: "v1", Status: agent.StatusNoStatus}
	ambiguousTwo := &agent.Element{ID: "ambiguous", Name: "Ambiguous", Type: "test.two", Hash: "two", Data: []byte(`{"two":true}`), IsJSONData: true, RetrievedAt: now, Version: "v1", Status: agent.StatusNoStatus}
	if err := ingest.NewService(store).StoreCrawl(ctx, agent.CloudCrawlData{
		DataSource:  agent.DataSource{DataSourceID: "test-ds", Info: agent.DataSourceInfo{Name: "Test", Desc: "Test", Type: "test"}},
		CrawledData: agent.CrawledData{Data: []*agent.Element{jsonAsset, binaryAsset, relationship, ambiguousOne, ambiguousTwo}},
		Timestamp:   now,
	}); err != nil {
		t.Fatalf("StoreCrawl() error = %v", err)
	}

	srv := NewServer(store, testRunner{runID: "run-1"})
	assertResponseContains(t, srv, "tools/list", nil, `coordimap_search_assets`)
	assertResponseContains(t, srv, "resources/templates/list", nil, `coordimap://assets/{internal_id}`)
	assertResponseContains(t, srv, "tools/call", map[string]any{"name": "coordimap_list_data_sources", "arguments": map[string]any{}}, `test-ds`)
	assertResponseContains(t, srv, "tools/call", map[string]any{"name": "coordimap_search_assets", "arguments": map[string]any{"query": "seed"}}, `asset-1`)
	assertResponseContains(t, srv, "tools/call", map[string]any{"name": "coordimap_get_asset", "arguments": map[string]any{"internal_id": "asset-1"}}, `raw_json`)
	assertResponseContains(t, srv, "tools/call", map[string]any{"name": "coordimap_get_asset", "arguments": map[string]any{"internal_id": "binary-1"}}, `AAEC`)
	assertResponseContains(t, srv, "tools/call", map[string]any{"name": "coordimap_get_relationships", "arguments": map[string]any{"internal_id": "asset-1", "direction": "outgoing"}}, `binary-1`)
	assertResponseContains(t, srv, "tools/call", map[string]any{"name": "coordimap_run_crawl", "arguments": map[string]any{}}, `started`)
	assertResponseContains(t, srv, "resources/read", map[string]any{"uri": "coordimap://relationships/asset-1"}, `binary-1`)

	assertToolError(t, srv, map[string]any{"name": "coordimap_get_asset", "arguments": map[string]any{"internal_id": "ambiguous"}}, "asset is ambiguous; supply type and/or data_source_id")
	assertToolError(t, srv, map[string]any{"name": "coordimap_get_relationships", "arguments": map[string]any{"internal_id": "asset-1", "direction": "sideways"}}, "isError")
	assertToolError(t, srv, map[string]any{"name": "coordimap_search_assets", "arguments": map[string]any{"query": "seed", "limit": 0}}, "limit must be at least 1")
}

func assertResponseContains(t *testing.T, srv interface {
	HandleMessage(context.Context, json.RawMessage) mcpgo.JSONRPCMessage
}, method string, params any, want string) {
	t.Helper()
	request := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		request["params"] = params
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response, err := json.Marshal(srv.HandleMessage(context.Background(), json.RawMessage(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(response), want) {
		t.Errorf("%s response = %s, want %q", method, response, want)
	}
}

func assertToolError(t *testing.T, srv interface {
	HandleMessage(context.Context, json.RawMessage) mcpgo.JSONRPCMessage
}, params map[string]any, want string) {
	t.Helper()
	request := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": params}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response, err := json.Marshal(srv.HandleMessage(context.Background(), json.RawMessage(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(response), `"isError":true`) || !strings.Contains(string(response), want) {
		t.Errorf("tool error response = %s, want tool error containing %q", response, want)
	}
}
