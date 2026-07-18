import { App } from "@modelcontextprotocol/ext-apps";
import { useEffect, useMemo, useState } from "react";
import { type GraphNode } from "./graph";
import { applyToolResult, type BridgeState } from "./topologyResult";
import { TopologyCanvas } from "./TopologyCanvas";

function useTopologyBridge() {
  const [state, setState] = useState<BridgeState>({ graph: null, error: false });
  useEffect(() => {
    const bridge = new App({ name: "coordimap-topology-app", version: "0.1.0" });
    const receive = (result: { structuredContent?: unknown }) => {
      setState((current) => applyToolResult(current, result));
    };
    bridge.addEventListener("toolresult", receive);
    void bridge.connect().catch(() => setState((current) => ({ ...current, error: true })));
    return () => bridge.removeEventListener("toolresult", receive);
  }, []);
  return state;
}

function nodeLabel(node: GraphNode) { return node.name || node.internal_id; }
function timestamp(value: string | null) { return value ? new Date(value).toLocaleString() : "Not observed"; }

export function AppShell() {
  const { graph, error } = useTopologyBridge();
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [focusID, setFocusID] = useState<string | null>(null);
  const [hoveredID, setHoveredID] = useState<string | null>(null);
  const [reducedMotion] = useState(() => window.matchMedia("(prefers-reduced-motion: reduce)").matches);
  const selected = useMemo(() => graph?.nodes.find((node) => node.internal_id === (selectedID ?? graph.rootID)) ?? null, [graph, selectedID]);
  const directEdges = useMemo(() => selected && graph ? graph.edges.filter((edge) => edge.source_internal_id === selected.internal_id || edge.destination_internal_id === selected.internal_id).length : 0, [graph, selected]);
  const hovered = graph?.nodes.find((node) => node.internal_id === hoveredID) ?? null;
  const select = (id: string) => { setSelectedID(id); setFocusID(id); };

  if (!graph) return <main className="app-shell"><div className="empty-state" role="status">{error ? "Unable to render topology data." : "Waiting for topology data…"}</div></main>;
  return <main className="app-shell">
    <header className="signal-header"><div><p className="eyebrow">Stored topology · two hops</p><h1>{graph.nodes.find((node) => node.internal_id === graph.rootID)?.name || graph.rootID}</h1></div><button className="reset-control" type="button" onClick={() => select(graph.rootID)}>Reset view</button></header>
    <div className="topology-layout">
      <section className="canvas-panel" aria-label="Topology visualization">
        {graph.truncated && <p className="truncation" role="status">Topology truncated at the configured node or relationship limit.</p>}
        {graph.edges.length === 0 && <p className="empty-relationships">No relationships observed</p>}
        {hovered && <p className="graph-tooltip">{nodeLabel(hovered)}{hovered.type ? ` · ${hovered.type}` : ""}</p>}
        <TopologyCanvas graph={graph} selectedID={selected?.internal_id ?? null} onSelect={select} onHover={setHoveredID} reducedMotion={reducedMotion} focusID={focusID} />
      </section>
      <aside className="inspector" aria-label="Topology node inspector">
        <section className="detail-panel"><p className="eyebrow">Selected signal</p>{selected && <><h2>{nodeLabel(selected)}</h2><dl><dt>Internal ID</dt><dd>{selected.internal_id}</dd>{selected.type && <><dt>Type</dt><dd>{selected.type}</dd></>}{selected.data_source_id && <><dt>Data source</dt><dd>{selected.data_source_id}</dd></>}{selected.status && <><dt>Status</dt><dd>{selected.status}</dd></>}<dt>Last seen</dt><dd>{timestamp(selected.last_seen)}</dd><dt>Direct edges</dt><dd>{directEdges}</dd></dl>{selected.unresolved && <p className="unresolved-note">Unresolved endpoint — only its observed ID is available.</p>}</>}</section>
        <section className="node-list"><p className="eyebrow">Nodes · {graph.nodes.length}</p><div>{graph.nodes.map((node) => <button key={node.internal_id} className={node.internal_id === selected?.internal_id ? "node-button active" : "node-button"} type="button" onClick={() => select(node.internal_id)}><span className={`layer-dot layer-${node.layer}`} aria-hidden="true" />{nodeLabel(node)}</button>)}</div></section>
      </aside>
    </div>
    <div className="sr-only" aria-live="polite">{graph.truncated ? "Topology truncated at the configured node or relationship limit." : selected ? `Selected ${nodeLabel(selected)}` : ""}</div>
  </main>;
}
