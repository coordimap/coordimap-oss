// Package mcp exposes locally persisted crawl inventory through MCP stdio.
package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/coordimap/agent/internal/app/ports"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// CrawlRunner starts configured crawlers without exposing integration details.
type CrawlRunner interface {
	Run(dataSourceID string) (runID string, running bool, err error)
}

// NewServer creates the read-only local inventory MCP server.
func NewServer(store ports.Store, runner CrawlRunner) *server.MCPServer {
	srv := server.NewMCPServer("coordimap-local", "0.1.0")

	srv.AddTool(mcpgo.NewTool("coordimap_list_data_sources", mcpgo.WithDescription("List stored data sources and their latest completed crawl.")), func(ctx context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		var sources []ports.DataSourceSummary
		if err := read(ctx, store, func(query ports.QueryRepository) error {
			var err error
			sources, err = query.ListDataSources(ctx)
			return err
		}); err != nil {
			return toolError(err), nil
		}
		return jsonResult(sources), nil
	})

	srv.AddTool(mcpgo.NewTool("coordimap_search_assets",
		mcpgo.WithDescription("Search stored assets by name."),
		mcpgo.WithString("query", mcpgo.Required()),
		mcpgo.WithString("type"),
		mcpgo.WithString("data_source_id"),
		mcpgo.WithString("status"),
		mcpgo.WithNumber("limit"),
	), func(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		var args struct {
			Query        string `json:"query"`
			Type         string `json:"type"`
			DataSourceID string `json:"data_source_id"`
			Status       string `json:"status"`
			Limit        *int   `json:"limit"`
		}
		if err := request.BindArguments(&args); err != nil {
			return toolError(err), nil
		}
		limit, err := normalizedLimit(args.Limit, 25, 100)
		if err != nil {
			return toolError(err), nil
		}
		var assets []ports.AssetSummary
		if err := read(ctx, store, func(query ports.QueryRepository) error {
			var err error
			assets, err = query.SearchAssets(ctx, ports.AssetSearch{Query: args.Query, Type: args.Type, DataSourceID: args.DataSourceID, Status: args.Status, Limit: limit})
			return err
		}); err != nil {
			return toolError(err), nil
		}
		return jsonResult(assets), nil
	})

	srv.AddTool(mcpgo.NewTool("coordimap_get_asset",
		mcpgo.WithDescription("Get one stored asset and its payload."),
		mcpgo.WithString("internal_id", mcpgo.Required()),
		mcpgo.WithString("type"),
		mcpgo.WithString("data_source_id"),
	), func(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		var args struct {
			InternalID   string `json:"internal_id"`
			Type         string `json:"type"`
			DataSourceID string `json:"data_source_id"`
		}
		if err := request.BindArguments(&args); err != nil {
			return toolError(err), nil
		}
		return getAsset(ctx, store, args.InternalID, args.Type, args.DataSourceID)
	})

	srv.AddTool(mcpgo.NewTool("coordimap_get_relationships",
		mcpgo.WithDescription("Get incoming and outgoing stored relationships for an asset."),
		mcpgo.WithString("internal_id", mcpgo.Required()),
		mcpgo.WithString("direction", mcpgo.Enum("incoming", "outgoing", "both")),
		mcpgo.WithNumber("relation_type"),
		mcpgo.WithNumber("limit"),
	), func(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		var args struct {
			InternalID   string `json:"internal_id"`
			Direction    string `json:"direction"`
			RelationType *int   `json:"relation_type"`
			Limit        *int   `json:"limit"`
		}
		if err := request.BindArguments(&args); err != nil {
			return toolError(err), nil
		}
		if args.Direction == "" {
			args.Direction = "both"
		}
		if args.Direction != "incoming" && args.Direction != "outgoing" && args.Direction != "both" {
			return toolError(fmt.Errorf("direction must be incoming, outgoing, or both")), nil
		}
		limit, err := normalizedLimit(args.Limit, 50, 200)
		if err != nil {
			return toolError(err), nil
		}
		return getRelationships(ctx, store, args.InternalID, args.Direction, args.RelationType, limit)
	})

	srv.AddTool(mcpgo.NewTool("coordimap_run_crawl",
		mcpgo.WithDescription("Start configured crawlers, or report that they are already running."),
		mcpgo.WithString("data_source_id"),
	), func(_ context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		if runner == nil {
			return toolError(fmt.Errorf("crawl runner is unavailable")), nil
		}
		var args struct {
			DataSourceID string `json:"data_source_id"`
		}
		if err := request.BindArguments(&args); err != nil {
			return toolError(err), nil
		}
		runID, running, err := runner.Run(args.DataSourceID)
		if err != nil {
			return toolError(err), nil
		}
		status := "started"
		if running {
			status = "already_running"
		}
		return jsonResult(map[string]string{"crawl_run_id": runID, "status": status}), nil
	})

	srv.AddResource(mcpgo.NewResource("coordimap://data-sources", "Coordimap data sources", mcpgo.WithMIMEType("application/json")), func(ctx context.Context, _ mcpgo.ReadResourceRequest) ([]mcpgo.ResourceContents, error) {
		var sources []ports.DataSourceSummary
		if err := read(ctx, store, func(query ports.QueryRepository) error {
			var err error
			sources, err = query.ListDataSources(ctx)
			return err
		}); err != nil {
			return nil, err
		}
		return resourceJSON("coordimap://data-sources", sources)
	})
	srv.AddResourceTemplate(mcpgo.NewResourceTemplate("coordimap://assets/{internal_id}", "Coordimap asset", mcpgo.WithTemplateMIMEType("application/json")), func(ctx context.Context, request mcpgo.ReadResourceRequest) ([]mcpgo.ResourceContents, error) {
		internalID, err := resourceID(request.Params.URI, "assets")
		if err != nil {
			return nil, err
		}
		result, err := getAssetValue(ctx, store, internalID, "", "")
		if err != nil {
			return nil, err
		}
		return resourceJSON(request.Params.URI, result)
	})
	srv.AddResourceTemplate(mcpgo.NewResourceTemplate("coordimap://relationships/{internal_id}", "Coordimap relationships", mcpgo.WithTemplateMIMEType("application/json")), func(ctx context.Context, request mcpgo.ReadResourceRequest) ([]mcpgo.ResourceContents, error) {
		internalID, err := resourceID(request.Params.URI, "relationships")
		if err != nil {
			return nil, err
		}
		var relationships []ports.Relationship
		if err := read(ctx, store, func(query ports.QueryRepository) error {
			var err error
			relationships, err = query.GetRelationships(ctx, ports.RelationshipSearch{InternalID: internalID, Direction: "both", Limit: 50})
			return err
		}); err != nil {
			return nil, err
		}
		return resourceJSON(request.Params.URI, relationships)
	})

	return srv
}

// ServeStdio starts the MCP server over stdin and stdout.
func ServeStdio(store ports.Store, runner CrawlRunner) error {
	return server.ServeStdio(NewServer(store, runner))
}

func getAsset(ctx context.Context, store ports.Store, internalID, elementType, dataSourceID string) (*mcpgo.CallToolResult, error) {
	asset, err := getAssetValue(ctx, store, internalID, elementType, dataSourceID)
	if err != nil {
		return toolError(err), nil
	}
	return jsonResult(asset), nil
}

func getAssetValue(ctx context.Context, store ports.Store, internalID, elementType, dataSourceID string) (map[string]any, error) {
	var assets []ports.Asset
	if err := read(ctx, store, func(query ports.QueryRepository) error {
		var err error
		assets, err = query.GetAssets(ctx, internalID, elementType, dataSourceID)
		return err
	}); err != nil {
		return nil, err
	}
	if len(assets) == 0 {
		return nil, fmt.Errorf("asset not found")
	}
	if len(assets) > 1 {
		return nil, fmt.Errorf("asset is ambiguous; supply type and/or data_source_id")
	}
	asset := assets[0]
	result := map[string]any{
		"internal_id":    asset.InternalID,
		"type":           asset.Type,
		"data_source_id": asset.DataSourceID,
		"name":           asset.Name,
		"status":         asset.Status,
		"last_seen":      asset.LastSeen,
		"hash":           asset.Hash,
		"is_json_data":   asset.IsJSONData,
		"version":        asset.Version,
		"retrieved_at":   asset.RetrievedAt,
		"first_seen":     asset.FirstSeen,
	}
	if asset.RawJSON != nil {
		var payload any
		if err := json.Unmarshal([]byte(*asset.RawJSON), &payload); err != nil {
			return nil, fmt.Errorf("decode stored JSON: %w", err)
		}
		result["raw_json"] = payload
	} else {
		result["raw_data_base64"] = base64.StdEncoding.EncodeToString(asset.RawData)
	}
	return result, nil
}

func getRelationships(ctx context.Context, store ports.Store, internalID, direction string, relationType *int, limit int) (*mcpgo.CallToolResult, error) {
	var relationships []ports.Relationship
	if err := read(ctx, store, func(query ports.QueryRepository) error {
		var err error
		relationships, err = query.GetRelationships(ctx, ports.RelationshipSearch{InternalID: internalID, Direction: direction, RelationType: relationType, Limit: limit})
		return err
	}); err != nil {
		return toolError(err), nil
	}
	return jsonResult(relationships), nil
}

func read(ctx context.Context, store ports.Store, fn func(ports.QueryRepository) error) error {
	return store.WithTx(ctx, func(ctx context.Context, repos ports.Repositories) error {
		return fn(repos.Query())
	})
}

func normalizedLimit(limit *int, defaultLimit, maxLimit int) (int, error) {
	if limit == nil {
		return defaultLimit, nil
	}
	if *limit < 1 {
		return 0, fmt.Errorf("limit must be at least 1")
	}
	if *limit > maxLimit {
		return maxLimit, nil
	}
	return *limit, nil
}

func jsonResult(value any) *mcpgo.CallToolResult {
	data, err := json.Marshal(value)
	if err != nil {
		return toolError(fmt.Errorf("encode response: %w", err))
	}
	return mcpgo.NewToolResultText(string(data))
}

func toolError(err error) *mcpgo.CallToolResult {
	return mcpgo.NewToolResultError(err.Error())
}

func resourceJSON(uri string, value any) ([]mcpgo.ResourceContents, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode resource: %w", err)
	}
	return []mcpgo.ResourceContents{mcpgo.TextResourceContents{URI: uri, MIMEType: "application/json", Text: string(data)}}, nil
}

func resourceID(rawURI, expectedHost string) (string, error) {
	parsed, err := url.Parse(rawURI)
	if err != nil || parsed.Scheme != "coordimap" || parsed.Host != expectedHost {
		return "", fmt.Errorf("invalid resource URI %q", rawURI)
	}
	id := strings.TrimPrefix(parsed.EscapedPath(), "/")
	if id == "" || strings.Contains(id, "/") {
		return "", fmt.Errorf("invalid resource URI %q", rawURI)
	}
	return url.PathUnescape(id)
}
