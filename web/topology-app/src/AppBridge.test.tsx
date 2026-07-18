import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TopologyPayload } from "./graph";

const bridge = vi.hoisted(() => ({ instances: [] as Array<{ listeners: Map<string, (result: { structuredContent?: unknown }) => void> }> }));

vi.mock("@modelcontextprotocol/ext-apps", () => ({
  App: class {
    listeners = new Map<string, (result: { structuredContent?: unknown }) => void>();
    constructor() { bridge.instances.push(this); }
    addEventListener(type: string, listener: (result: { structuredContent?: unknown }) => void) { this.listeners.set(type, listener); }
    removeEventListener(type: string) { this.listeners.delete(type); }
    connect() { return Promise.resolve(); }
  },
}));
vi.mock("./TopologyCanvas", () => ({
  TopologyCanvas: ({ graph, reducedMotion }: { graph: { rootID: string }; reducedMotion: boolean }) => <div data-testid="canvas" data-root={graph.rootID} data-reduced-motion={String(reducedMotion)} />,
}));

import { AppShell } from "./App";

const payload: TopologyPayload = {
  root: { internal_id: "root", type: "service", data_source_id: "ds", name: "Root", status: "Green", last_seen: "2026-01-01T00:00:00Z" },
  nodes: [], edges: [], max_depth: 2, truncated: false,
};

afterEach(() => { bridge.instances.length = 0; document.body.innerHTML = ""; });

describe("MCP Apps topology bridge", () => {
  it("populates the canvas model and root details while honoring reduced motion", async () => {
    Object.defineProperty(window, "matchMedia", { configurable: true, value: () => ({ matches: true }) });
    const element = document.createElement("div"); document.body.append(element);
    const root = createRoot(element);
    await act(async () => { root.render(<AppShell />); });
    await act(async () => {
      bridge.instances[0].listeners.get("toolresult")?.({ structuredContent: payload });
    });
    expect(element.querySelector("[data-testid=canvas]")?.getAttribute("data-root")).toBe("root");
    expect(element.querySelector("[data-testid=canvas]")?.getAttribute("data-reduced-motion")).toBe("true");
    expect(element.textContent).toContain("Root");
    await act(async () => root.unmount());
  });
});
