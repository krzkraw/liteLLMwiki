import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";
import { KnowledgeGraphView } from "./KnowledgeGraphView";
import type { KnowledgeGraph } from "../lib/knowledgeGraph";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT =
  true;

let root: Root | null = null;
let container: HTMLDivElement | null = null;

function getByTestId<T extends Element = Element>(testId: string): T {
  const element = container?.querySelector(`[data-testid="${testId}"]`);

  if (!element) {
    throw new Error(`Unable to find element with data-testid="${testId}".`);
  }

  return element as T;
}

async function renderGraph(graph: KnowledgeGraph) {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);

  await act(async () => {
    root?.render(<KnowledgeGraphView graph={graph} />);
  });
}

describe("KnowledgeGraphView", () => {
  afterEach(() => {
    act(() => {
      root?.unmount();
    });
    container?.remove();
    root = null;
    container = null;
  });

  it("shows an empty state when the graph has no nodes", async () => {
    await renderGraph({ nodes: [], edges: [] });

    expect(getByTestId("knowledge-graph").textContent).toContain("No graph yet");
    expect(container?.querySelector("svg")).toBeNull();
  });

  it("renders a deterministic SVG graph from node order", async () => {
    const graph: KnowledgeGraph = {
      nodes: [
        { id: "folder-demo", kind: "folder", label: "demo" },
        { id: "file-main", kind: "file", label: "src/main.ts" },
        { id: "entity-provider", kind: "entity", label: "Provider" },
      ],
      edges: [
        {
          id: "contains:folder-demo->file-main",
          source: "folder-demo",
          target: "file-main",
          kind: "contains",
          label: "contains",
        },
        {
          id: "mentions:file-main->entity-provider",
          source: "file-main",
          target: "entity-provider",
          kind: "mentions",
          label: "mentions",
        },
      ],
    };

    await renderGraph(graph);

    const wrapper = getByTestId("knowledge-graph");
    const svg = wrapper.querySelector("svg");
    const nodes = Array.from(wrapper.querySelectorAll("[data-node-id]"));
    const edges = Array.from(wrapper.querySelectorAll("[data-edge-id]"));

    expect(svg).not.toBeNull();
    expect(svg?.getAttribute("viewBox")).toBe("0 0 720 420");
    expect(nodes.map((node) => node.getAttribute("data-node-id"))).toEqual([
      "folder-demo",
      "file-main",
      "entity-provider",
    ]);
    expect(nodes.map((node) => node.getAttribute("transform"))).toEqual([
      "translate(360 72)",
      "translate(609 276)",
      "translate(111 276)",
    ]);
    expect(edges.map((edge) => edge.getAttribute("data-edge-id"))).toEqual([
      "contains:folder-demo->file-main",
      "mentions:file-main->entity-provider",
    ]);
    expect(wrapper.textContent).toContain("demo");
    expect(wrapper.textContent).toContain("Provider");
    expect(wrapper.textContent).toContain(
      "Graph nodes: demo, src/main.ts, Provider",
    );
    expect(wrapper.textContent).toContain(
      "Graph edges: contains, mentions",
    );
  });

  it("keeps dense graph labels from overlapping the canvas", async () => {
    const graph: KnowledgeGraph = {
      nodes: Array.from({ length: 20 }, (_, index) => ({
        id: `entity-${index}`,
        kind: "entity",
        label: `Entity ${index}`,
      })),
      edges: [],
    };

    await renderGraph(graph);

    const wrapper = getByTestId("knowledge-graph");
    const circles = Array.from(wrapper.querySelectorAll("circle"));

    expect(circles).toHaveLength(20);
    expect(circles.every((circle) => circle.getAttribute("r") === "16")).toBe(
      true,
    );
    expect(wrapper.querySelectorAll("text")).toHaveLength(0);
  });
});
