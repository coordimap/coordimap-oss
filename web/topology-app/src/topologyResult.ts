import { isTopologyPayload, normalizeTopology, type NormalizedGraph } from "./graph";

export type BridgeState = { graph: NormalizedGraph | null; error: boolean };

export function applyToolResult(current: BridgeState, result: unknown): BridgeState {
  const structured = (result as { structuredContent?: unknown } | null)?.structuredContent;
  if (!isTopologyPayload(structured)) return { ...current, error: true };
  return { graph: normalizeTopology(structured), error: false };
}
