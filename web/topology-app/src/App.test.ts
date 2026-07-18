import { describe, expect, it } from "vitest";
import { applyToolResult } from "./topologyResult";
import { normalizeTopology, type TopologyPayload } from "./graph";

const payload: TopologyPayload = {
  root: { internal_id: "root", type: "service", data_source_id: "ds", name: "Root", status: "Green", last_seen: "2026-01-01T00:00:00Z" },
  nodes: [],
  edges: [],
  max_depth: 2,
  truncated: false,
};

describe("applyToolResult", () => {
  it("keeps the last valid graph and reports malformed content", () => {
    const current = { graph: normalizeTopology(payload), error: false };
    expect(applyToolResult(current, { structuredContent: { root: {} } })).toEqual({ ...current, error: true });
  });

  it("accepts synthetic structured content", () => {
    const next = applyToolResult({ graph: null, error: true }, { structuredContent: payload });
    expect(next.error).toBe(false);
    expect(next.graph?.rootID).toBe("root");
  });
});
