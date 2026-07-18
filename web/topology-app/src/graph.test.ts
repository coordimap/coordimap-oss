import { describe, expect, it } from "vitest";
import { normalizeTopology, type TopologyPayload } from "./graph";

const payload: TopologyPayload = {
  root: { internal_id: "root", type: "service", data_source_id: "ds", name: "Root", status: "Green", last_seen: "2026-01-01T00:00:00Z" },
  nodes: [
    { internal_id: "child", type: "workload", data_source_id: "ds", name: "Child", status: "Green", last_seen: "2026-01-01T00:00:00Z" },
    { internal_id: "grandchild", type: "database", data_source_id: "ds", name: "Grandchild", status: "Green", last_seen: "2026-01-01T00:00:00Z" },
  ],
  edges: [
    { data_source_id: "ds", source_internal_id: "root", destination_internal_id: "child", relationship_type: "contains", relation_type: 3, first_seen: "2026-01-01T00:00:00Z", last_seen: "2026-01-01T00:00:00Z" },
    { data_source_id: "ds", source_internal_id: "child", destination_internal_id: "grandchild", relationship_type: "connects_to", relation_type: 100, first_seen: "2026-01-01T00:00:00Z", last_seen: "2026-01-01T00:00:00Z" },
    { data_source_id: "ds", source_internal_id: "child", destination_internal_id: "unresolved", relationship_type: "connects_to", relation_type: 100, first_seen: "2026-01-01T00:00:00Z", last_seen: "2026-01-01T00:00:00Z" },
  ],
  max_depth: 2,
  truncated: false,
};

describe("normalizeTopology", () => {
  it("derives two-hop layers and preserves unresolved endpoints", () => {
    const graph = normalizeTopology(payload);
    const byID = new Map(graph.nodes.map((node) => [node.internal_id, node]));
    expect(byID.get("root")?.layer).toBe(0);
    expect(byID.get("child")?.layer).toBe(1);
    expect(byID.get("grandchild")?.layer).toBe(2);
    expect(byID.get("unresolved")).toMatchObject({ layer: 2, unresolved: true });
  });

  it("inserts the root and clamps deeper paths", () => {
    const graph = normalizeTopology({ ...payload, nodes: [], edges: [
      ...payload.edges,
      { ...payload.edges[0], source_internal_id: "grandchild", destination_internal_id: "fourth" },
    ] });
    expect(graph.nodes.find((node) => node.internal_id === "root")?.layer).toBe(0);
    expect(graph.nodes.find((node) => node.internal_id === "fourth")?.layer).toBe(2);
  });
});
