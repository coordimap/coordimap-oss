export interface TopologyRoot {
  internal_id: string;
  type: string;
  data_source_id: string;
  name: string;
  status: string;
  last_seen: string;
}

export interface TopologyNode {
  internal_id: string;
  type: string | null;
  data_source_id: string | null;
  name: string | null;
  status: string | null;
  last_seen: string | null;
}

export interface TopologyEdge {
  data_source_id: string;
  source_internal_id: string;
  destination_internal_id: string;
  relationship_type: string;
  relation_type: number;
  first_seen: string;
  last_seen: string;
}

export interface TopologyPayload {
  root: TopologyRoot;
  nodes: TopologyNode[];
  edges: TopologyEdge[];
  max_depth: 2;
  truncated: boolean;
}

export interface GraphNode extends TopologyNode {
  layer: 0 | 1 | 2;
  unresolved: boolean;
  disconnected: boolean;
  x: number;
  y: number;
  fx?: number;
  fy?: number;
}

export interface NormalizedGraph {
  rootID: string;
  nodes: GraphNode[];
  edges: TopologyEdge[];
  truncated: boolean;
}

export function isTopologyPayload(value: unknown): value is TopologyPayload {
  if (typeof value !== "object" || value === null) return false;
  const payload = value as Record<string, unknown>;
  const root = payload.root as Record<string, unknown> | undefined;
  return (
    typeof root === "object" && root !== null && typeof root.internal_id === "string" &&
    Array.isArray(payload.nodes) && Array.isArray(payload.edges)
  );
}

export function normalizeTopology(payload: TopologyPayload): NormalizedGraph {
  const byID = new Map<string, TopologyNode>();
  for (const node of payload.nodes) {
    if (node && typeof node.internal_id === "string" && node.internal_id !== "") byID.set(node.internal_id, node);
  }
  byID.set(payload.root.internal_id, {
    internal_id: payload.root.internal_id,
    type: payload.root.type,
    data_source_id: payload.root.data_source_id,
    name: payload.root.name,
    status: payload.root.status,
    last_seen: payload.root.last_seen,
  });

  const adjacency = new Map<string, Set<string>>();
  const ensure = (id: string) => {
    if (!adjacency.has(id)) adjacency.set(id, new Set());
  };
  for (const edge of payload.edges) {
    if (!edge || !edge.source_internal_id || !edge.destination_internal_id) continue;
    ensure(edge.source_internal_id);
    ensure(edge.destination_internal_id);
    adjacency.get(edge.source_internal_id)?.add(edge.destination_internal_id);
    adjacency.get(edge.destination_internal_id)?.add(edge.source_internal_id);
    for (const id of [edge.source_internal_id, edge.destination_internal_id]) {
      if (!byID.has(id)) byID.set(id, { internal_id: id, type: null, data_source_id: null, name: null, status: null, last_seen: null });
    }
  }

  const distances = new Map<string, number>([[payload.root.internal_id, 0]]);
  const queue = [payload.root.internal_id];
  for (let index = 0; index < queue.length; index += 1) {
    const id = queue[index];
    const distance = distances.get(id) ?? 2;
    for (const neighbor of adjacency.get(id) ?? []) {
      if (!distances.has(neighbor)) {
        distances.set(neighbor, distance + 1);
        queue.push(neighbor);
      }
    }
  }

  const nodes = [...byID.values()].map((node, index) => {
    const knownDistance = distances.get(node.internal_id);
    const disconnected = knownDistance === undefined;
    const layer = Math.min(2, knownDistance ?? 2) as 0 | 1 | 2;
    return {
      ...node,
      layer,
      unresolved: node.type === null && node.name === null,
      disconnected,
      x: layer === 0 ? 0 : Math.cos(index * 2.4) * layer * 180,
      y: layer === 0 ? 0 : Math.sin(index * 2.4) * layer * 180,
      ...(layer === 0 ? { fx: 0, fy: 0 } : {}),
    };
  });
  return { rootID: payload.root.internal_id, nodes, edges: payload.edges, truncated: payload.truncated };
}
