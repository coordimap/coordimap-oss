import { Application, extend, useApplication, useTick } from "@pixi/react";
import { Container, Graphics } from "pixi.js";
import { Viewport } from "pixi-viewport";
import { forceCollide, forceLink, forceManyBody, forceRadial, forceSimulation, type Simulation } from "d3-force";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { GraphNode, NormalizedGraph, TopologyEdge } from "./graph";

extend({ Container, Graphics, Viewport });

type Props = { graph: NormalizedGraph; selectedID: string | null; onSelect: (id: string) => void; onHover: (id: string | null) => void; reducedMotion: boolean; focusID: string | null };
type PositionedEdge = TopologyEdge & { source: GraphNode; target: GraphNode };

function GraphLayer({ graph, selectedID, onSelect, onHover, reducedMotion, focusID }: Props) {
  const { app } = useApplication();
  const viewportRef = useRef<Viewport | null>(null);
  const simulation = useRef<Simulation<GraphNode, PositionedEdge> | null>(null);
  const [revision, setRevision] = useState(0);
  const [phase, setPhase] = useState(0);
  const nodes = useMemo(() => graph.nodes.map((node) => ({ ...node })), [graph]);
  const edges = useMemo(() => {
    const byID = new Map(nodes.map((node) => [node.internal_id, node]));
    return graph.edges.flatMap((edge) => {
      const source = byID.get(edge.source_internal_id);
      const target = byID.get(edge.destination_internal_id);
      return source && target ? [{ ...edge, source, target }] : [];
    });
  }, [graph.edges, nodes]);

  useEffect(() => {
    const layout = forceSimulation<GraphNode, PositionedEdge>(nodes)
      .force("link", forceLink<GraphNode, PositionedEdge>(edges).id((node) => node.internal_id).distance(110).strength(0.7))
      .force("charge", forceManyBody<GraphNode>().strength(-410))
      .force("collision", forceCollide<GraphNode>().radius((node) => node.layer === 0 ? 38 : node.layer === 1 ? 29 : 24).strength(0.95))
      .force("radial", forceRadial<GraphNode>((node) => [0, 180, 360][node.layer], 0, 0).strength(0.34))
      .alpha(1)
      .alphaDecay(reducedMotion ? 0.06 : 0.035)
      .alphaTarget(reducedMotion ? 0 : 0.015)
      .on("tick", () => setRevision((value) => value + 1));
    const root = nodes.find((node) => node.internal_id === graph.rootID);
    if (root) { root.fx = 0; root.fy = 0; }
    simulation.current = layout;
    if (reducedMotion) layout.on("end", () => layout.stop());
    return () => { layout.stop(); };
  }, [edges, graph.rootID, nodes, reducedMotion]);

  useEffect(() => {
    if (!focusID || !viewportRef.current) return;
    const node = nodes.find((candidate) => candidate.internal_id === focusID);
    if (node) viewportRef.current.animate({ position: { x: node.x, y: node.y }, time: 250 });
  }, [focusID, nodes]);

  useTick({ callback: () => { if (!reducedMotion) setPhase((value) => (value + 0.004) % 1); }, isEnabled: !reducedMotion });

  const drawEdges = useCallback((graphics: Graphics) => {
    graphics.clear();
    for (const edge of edges) {
      const sx = edge.source.x; const sy = edge.source.y; const tx = edge.target.x; const ty = edge.target.y;
      const bend = (edge.source.layer + edge.target.layer) % 2 === 0 ? 24 : -24;
      const mx = (sx + tx) / 2 + bend; const my = (sy + ty) / 2 - bend;
      graphics.moveTo(sx, sy).quadraticCurveTo(mx, my, tx, ty).stroke({ color: edge.relation_type === 3 ? 0x4dd6e7 : 0x9b7be8, alpha: 0.35, width: 1.5 });
      if (!reducedMotion) {
        const t = (phase + Math.abs(edge.relation_type % 10) / 10) % 1;
        const x = (1 - t) * (1 - t) * sx + 2 * (1 - t) * t * mx + t * t * tx;
        const y = (1 - t) * (1 - t) * sy + 2 * (1 - t) * t * my + t * t * ty;
        graphics.circle(x, y, 2.5).fill({ color: 0xb8eff3, alpha: 0.9 });
      }
    }
  }, [edges, phase, reducedMotion, revision]);

  const drawNode = useCallback((node: GraphNode, graphics: Graphics) => {
    graphics.clear();
    const radius = node.layer === 0 ? 28 : node.layer === 1 ? 20 : 16;
    const selected = selectedID === node.internal_id;
    const color = node.layer === 0 ? 0x52d9e8 : node.layer === 1 ? 0x91c7d0 : 0x9b7be8;
    if (node.layer === 0) graphics.circle(node.x, node.y, radius + 12).fill({ color: 0x42cbdc, alpha: 0.12 });
    graphics.circle(node.x + 4, node.y + 6, radius).fill({ color: 0x02050a, alpha: 0.62 });
    graphics.circle(node.x, node.y, radius).fill({ color, alpha: node.disconnected ? 0.34 : 0.82 });
    graphics.circle(node.x - radius * 0.28, node.y - radius * 0.3, radius * 0.32).fill({ color: 0xe2fbfd, alpha: 0.38 });
    graphics.circle(node.x, node.y, radius).stroke({ color: selected ? 0xf0fdff : 0xcaf7fb, alpha: selected ? 1 : 0.7, width: selected ? 3 : 1.5 });
    if (node.unresolved) graphics.circle(node.x, node.y, radius + 4).stroke({ color: 0xd2c5ff, alpha: 0.9, width: 1.5, cap: "round", join: "round" });
  }, [selectedID, revision]);

  return <viewport
    ref={(viewport: Viewport | null) => {
      if (!viewport || viewportRef.current === viewport) return;
      viewportRef.current = viewport;
      viewport.drag().pinch().wheel().decelerate().clampZoom({ minScale: 0.35, maxScale: 2.6 });
      viewport.moveCenter(0, 0);
    }}
    screenWidth={app.screen.width}
    screenHeight={app.screen.height}
    worldWidth={1600}
    worldHeight={1200}
    events={app.renderer.events}
  >
    <pixiGraphics draw={drawEdges} />
    {nodes.map((node) => <pixiGraphics key={node.internal_id} draw={(graphics: Graphics) => drawNode(node, graphics)} eventMode="static" cursor="pointer" onPointerTap={() => onSelect(node.internal_id)} onPointerOver={() => onHover(node.internal_id)} onPointerOut={() => onHover(null)} />)}
  </viewport>;
}

export function TopologyCanvas(props: Props) {
  const host = useRef<HTMLDivElement>(null);
  return <div className="topology-canvas" ref={host} aria-label="Interactive topology diagram"><Application resizeTo={host} background="#070A0F" antialias><GraphLayer {...props} /></Application></div>;
}
