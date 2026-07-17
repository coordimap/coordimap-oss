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

	srv.AddTool(mcpgo.NewTool("coordimap_get_infrastructure_summary",
		mcpgo.WithDescription("Summarize stored crawl observations; results are not live provider state."),
		mcpgo.WithString("data_source_id"),
	), func(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		var args struct {
			DataSourceID string `json:"data_source_id"`
		}
		if err := request.BindArguments(&args); err != nil {
			return toolError(err), nil
		}
		var summary ports.InfrastructureSummary
		if err := read(ctx, store, func(query ports.QueryRepository) error {
			var err error
			summary, err = query.GetInfrastructureSummary(ctx, args.DataSourceID)
			return err
		}); err != nil {
			return toolError(err), nil
		}
		return jsonResult(summary), nil
	})

	srv.AddTool(mcpgo.NewTool("coordimap_list_relationship_types",
		mcpgo.WithDescription("List stored crawl-observation relationship kinds; results are not live provider state."),
		mcpgo.WithString("data_source_id"),
	), func(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		var args struct {
			DataSourceID string `json:"data_source_id"`
		}
		if err := request.BindArguments(&args); err != nil {
			return toolError(err), nil
		}
		var relationshipTypes []ports.RelationshipTypeSummary
		if err := read(ctx, store, func(query ports.QueryRepository) error {
			var err error
			relationshipTypes, err = query.ListRelationshipTypes(ctx, args.DataSourceID)
			return err
		}); err != nil {
			return toolError(err), nil
		}
		return jsonResult(relationshipTypes), nil
	})

	srv.AddTool(mcpgo.NewTool("coordimap_list_crawl_runs",
		mcpgo.WithDescription("List stored crawl-run observations; results are not live provider state."),
		mcpgo.WithString("data_source_id"),
		mcpgo.WithNumber("limit"),
	), func(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		var args struct {
			DataSourceID string `json:"data_source_id"`
			Limit        *int   `json:"limit"`
		}
		if err := request.BindArguments(&args); err != nil {
			return toolError(err), nil
		}
		limit, err := normalizedLimit(args.Limit, 25, 100)
		if err != nil {
			return toolError(err), nil
		}
		var runs []ports.CrawlRunSummary
		if err := read(ctx, store, func(query ports.QueryRepository) error {
			var err error
			runs, err = query.ListCrawlRuns(ctx, ports.CrawlRunSearch{DataSourceID: args.DataSourceID, Limit: limit})
			return err
		}); err != nil {
			return toolError(err), nil
		}
		return jsonResult(runs), nil
	})

	srv.AddTool(mcpgo.NewTool("coordimap_get_asset_versions",
		mcpgo.WithDescription("Get stored crawl-observation payload versions for one asset; results are not live provider state."),
		mcpgo.WithString("internal_id", mcpgo.Required()),
		mcpgo.WithString("type"),
		mcpgo.WithString("data_source_id"),
		mcpgo.WithBoolean("include_payload"),
		mcpgo.WithNumber("limit"),
	), func(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		var args struct {
			InternalID     string `json:"internal_id"`
			Type           string `json:"type"`
			DataSourceID   string `json:"data_source_id"`
			IncludePayload bool   `json:"include_payload"`
			Limit          *int   `json:"limit"`
		}
		if err := request.BindArguments(&args); err != nil {
			return toolError(err), nil
		}
		asset, err := resolveAsset(ctx, store, args.InternalID, args.Type, args.DataSourceID)
		if err != nil {
			return toolError(err), nil
		}
		limit, err := normalizedLimit(args.Limit, 10, 25)
		if err != nil {
			return toolError(err), nil
		}
		var versions []ports.AssetVersion
		if err := read(ctx, store, func(query ports.QueryRepository) error {
			var err error
			versions, err = query.GetAssetVersions(ctx, ports.AssetVersionSearch{InternalID: asset.InternalID, Type: asset.Type, DataSourceID: asset.DataSourceID, Limit: limit})
			return err
		}); err != nil {
			return toolError(err), nil
		}
		values := make([]map[string]any, 0, len(versions))
		for _, version := range versions {
			value, err := assetVersionValue(version, args.IncludePayload)
			if err != nil {
				return toolError(err), nil
			}
			values = append(values, value)
		}
		return jsonResult(values), nil
	})

	srv.AddTool(mcpgo.NewTool("coordimap_explore_topology",
		mcpgo.WithDescription("Explore bounded stored crawl-observation topology; results are not live provider state."),
		mcpgo.WithString("internal_id", mcpgo.Required()),
		mcpgo.WithString("type"),
		mcpgo.WithString("data_source_id"),
		mcpgo.WithString("direction", mcpgo.Enum("incoming", "outgoing", "both")),
		mcpgo.WithNumber("relation_type"),
		mcpgo.WithNumber("max_depth"),
		mcpgo.WithNumber("max_nodes"),
		mcpgo.WithNumber("max_relationships"),
	), func(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		var args struct {
			InternalID       string `json:"internal_id"`
			Type             string `json:"type"`
			DataSourceID     string `json:"data_source_id"`
			Direction        string `json:"direction"`
			RelationType     *int   `json:"relation_type"`
			MaxDepth         *int   `json:"max_depth"`
			MaxNodes         *int   `json:"max_nodes"`
			MaxRelationships *int   `json:"max_relationships"`
		}
		if err := request.BindArguments(&args); err != nil {
			return toolError(err), nil
		}
		if _, err := resolveAsset(ctx, store, args.InternalID, args.Type, args.DataSourceID); err != nil {
			return toolError(err), nil
		}
		direction, err := normalizedDirection(args.Direction)
		if err != nil {
			return toolError(err), nil
		}
		depth, err := normalizedLimit(args.MaxDepth, 2, 4)
		if err != nil {
			return toolError(err), nil
		}
		nodes, err := normalizedLimit(args.MaxNodes, 100, 250)
		if err != nil {
			return toolError(err), nil
		}
		relationships, err := normalizedLimit(args.MaxRelationships, 200, 500)
		if err != nil {
			return toolError(err), nil
		}
		var topology ports.Topology
		if err := read(ctx, store, func(query ports.QueryRepository) error {
			var err error
			topology, err = query.ExploreTopology(ctx, ports.TopologySearch{InternalID: args.InternalID, DataSourceID: args.DataSourceID, RelationType: args.RelationType, Direction: direction, MaxDepth: depth, MaxNodes: nodes, MaxRelationships: relationships})
			return err
		}); err != nil {
			return toolError(err), nil
		}
		return jsonResult(topology), nil
	})

	srv.AddTool(mcpgo.NewTool("coordimap_find_relationship_path",
		mcpgo.WithDescription("Find one bounded shortest path in stored crawl observations; results are not live provider state."),
		mcpgo.WithString("from_internal_id", mcpgo.Required()),
		mcpgo.WithString("from_type"),
		mcpgo.WithString("from_data_source_id"),
		mcpgo.WithString("to_internal_id", mcpgo.Required()),
		mcpgo.WithString("to_type"),
		mcpgo.WithString("to_data_source_id"),
		mcpgo.WithString("direction", mcpgo.Enum("incoming", "outgoing", "both")),
		mcpgo.WithNumber("relation_type"),
		mcpgo.WithNumber("max_hops"),
	), func(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		var args struct {
			FromInternalID   string `json:"from_internal_id"`
			FromType         string `json:"from_type"`
			FromDataSourceID string `json:"from_data_source_id"`
			ToInternalID     string `json:"to_internal_id"`
			ToType           string `json:"to_type"`
			ToDataSourceID   string `json:"to_data_source_id"`
			Direction        string `json:"direction"`
			RelationType     *int   `json:"relation_type"`
			MaxHops          *int   `json:"max_hops"`
		}
		if err := request.BindArguments(&args); err != nil {
			return toolError(err), nil
		}
		if _, err := resolveAsset(ctx, store, args.FromInternalID, args.FromType, args.FromDataSourceID); err != nil {
			return toolError(err), nil
		}
		if _, err := resolveAsset(ctx, store, args.ToInternalID, args.ToType, args.ToDataSourceID); err != nil {
			return toolError(err), nil
		}
		direction, err := normalizedDirection(args.Direction)
		if err != nil {
			return toolError(err), nil
		}
		hops, err := normalizedLimit(args.MaxHops, 6, 10)
		if err != nil {
			return toolError(err), nil
		}
		var path ports.RelationshipPath
		if err := read(ctx, store, func(query ports.QueryRepository) error {
			var err error
			path, err = query.FindRelationshipPath(ctx, ports.PathSearch{FromInternalID: args.FromInternalID, ToInternalID: args.ToInternalID, RelationType: args.RelationType, Direction: direction, MaxHops: hops})
			return err
		}); err != nil {
			return toolError(err), nil
		}
		return jsonResult(path), nil
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
	srv.AddResourceTemplate(mcpgo.NewResourceTemplate("coordimap://topology/{internal_id}", "Coordimap topology", mcpgo.WithTemplateMIMEType("application/json")), func(ctx context.Context, request mcpgo.ReadResourceRequest) ([]mcpgo.ResourceContents, error) {
		internalID, err := resourceID(request.Params.URI, "topology")
		if err != nil {
			return nil, err
		}
		if _, err := resolveAsset(ctx, store, internalID, "", ""); err != nil {
			return nil, err
		}
		var topology ports.Topology
		if err := read(ctx, store, func(query ports.QueryRepository) error {
			var err error
			topology, err = query.ExploreTopology(ctx, ports.TopologySearch{InternalID: internalID, Direction: "both", MaxDepth: 2, MaxNodes: 100, MaxRelationships: 200})
			return err
		}); err != nil {
			return nil, err
		}
		return resourceJSON(request.Params.URI, topology)
	})
	srv.AddResourceTemplate(mcpgo.NewResourceTemplate("coordimap://asset-versions/{internal_id}", "Coordimap asset versions", mcpgo.WithTemplateMIMEType("application/json")), func(ctx context.Context, request mcpgo.ReadResourceRequest) ([]mcpgo.ResourceContents, error) {
		internalID, err := resourceID(request.Params.URI, "asset-versions")
		if err != nil {
			return nil, err
		}
		asset, err := resolveAsset(ctx, store, internalID, "", "")
		if err != nil {
			return nil, err
		}
		var versions []ports.AssetVersion
		if err := read(ctx, store, func(query ports.QueryRepository) error {
			var err error
			versions, err = query.GetAssetVersions(ctx, ports.AssetVersionSearch{InternalID: asset.InternalID, Type: asset.Type, DataSourceID: asset.DataSourceID, Limit: 10})
			return err
		}); err != nil {
			return nil, err
		}
		values := make([]map[string]any, 0, len(versions))
		for _, version := range versions {
			value, err := assetVersionValue(version, false)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		return resourceJSON(request.Params.URI, values)
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

func resolveAsset(ctx context.Context, store ports.Store, internalID, elementType, dataSourceID string) (ports.Asset, error) {
	var assets []ports.Asset
	if err := read(ctx, store, func(query ports.QueryRepository) error {
		var err error
		assets, err = query.GetAssets(ctx, internalID, elementType, dataSourceID)
		return err
	}); err != nil {
		return ports.Asset{}, err
	}
	if len(assets) == 0 {
		return ports.Asset{}, fmt.Errorf("asset not found")
	}
	if len(assets) > 1 {
		return ports.Asset{}, fmt.Errorf("asset is ambiguous; supply type and/or data_source_id")
	}
	return assets[0], nil
}

func assetVersionValue(version ports.AssetVersion, includePayload bool) (map[string]any, error) {
	result := map[string]any{
		"hash":         version.Hash,
		"crawl_run_id": version.CrawlRunID,
		"first_seen":   version.FirstSeen,
		"last_seen":    version.LastSeen,
	}
	if !includePayload {
		return result, nil
	}
	if version.RawJSON != nil {
		var payload any
		if err := json.Unmarshal([]byte(*version.RawJSON), &payload); err != nil {
			return nil, fmt.Errorf("decode stored JSON version: %w", err)
		}
		result["raw_json"] = payload
		return result, nil
	}
	result["raw_data_base64"] = base64.StdEncoding.EncodeToString(version.RawData)
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

func normalizedDirection(direction string) (string, error) {
	if direction == "" {
		return "both", nil
	}
	if direction != "incoming" && direction != "outgoing" && direction != "both" {
		return "", fmt.Errorf("direction must be incoming, outgoing, or both")
	}
	return direction, nil
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
