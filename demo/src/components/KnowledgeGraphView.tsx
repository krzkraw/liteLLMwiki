import type {
  KnowledgeGraph,
  KnowledgeGraphEdge,
  KnowledgeGraphNode,
} from "../lib/knowledgeGraph";

export interface KnowledgeGraphViewProps {
  graph: KnowledgeGraph;
}

interface GraphPoint {
  x: number;
  y: number;
}

const width = 720;
const height = 420;
const centerX = width / 2;
const centerY = 208;
const radiusX = 288;
const radiusY = 136;

const nodeFillByKind: Record<KnowledgeGraphNode["kind"], string> = {
  folder: "#dbeafe",
  file: "#dcfce7",
  chunk: "#fef3c7",
  entity: "#fce7f3",
  topic: "#ede9fe",
};

function positionForNode(index: number, count: number): GraphPoint {
  if (count <= 1) {
    return { x: centerX, y: centerY };
  }

  const angle = -Math.PI / 2 + (index * 2 * Math.PI) / count;

  return {
    x: Math.round(centerX + Math.cos(angle) * radiusX),
    y: Math.round(centerY + Math.sin(angle) * radiusY),
  };
}

function radiusForGraph(count: number): number {
  if (count > 18) {
    return 16;
  }

  if (count > 12) {
    return 22;
  }

  return 34;
}

function labelForNode(node: KnowledgeGraphNode): string {
  if (node.label.length <= 28) {
    return node.label;
  }

  return `${node.label.slice(0, 25).trimEnd()}...`;
}

function graphDescription(graph: KnowledgeGraph): string {
  const nodeLabels = graph.nodes.map((node) => node.label).join(", ");
  const edgeLabels = graph.edges
    .map((edge) => edge.label || edge.kind)
    .join(", ");

  return [
    `Graph nodes: ${nodeLabels || "none"}.`,
    `Graph edges: ${edgeLabels || "none"}.`,
  ].join(" ");
}

function edgeCoordinates(
  edge: KnowledgeGraphEdge,
  positions: Map<string, GraphPoint>,
) {
  const source = positions.get(edge.source);
  const target = positions.get(edge.target);

  if (!source || !target) {
    return null;
  }

  return { source, target };
}

export function KnowledgeGraphView({ graph }: KnowledgeGraphViewProps) {
  if (graph.nodes.length === 0) {
    return (
      <section
        className="knowledge-graph empty"
        data-testid="knowledge-graph"
        aria-label="Knowledge graph"
      >
        <p>No graph yet</p>
      </section>
    );
  }

  const positions = new Map(
    graph.nodes.map((node, index) => [
      node.id,
      positionForNode(index, graph.nodes.length),
    ]),
  );
  const nodeRadius = radiusForGraph(graph.nodes.length);
  const showVisibleLabels = graph.nodes.length <= 12;
  const descriptionId = "knowledge-graph-description";

  return (
    <section
      className="knowledge-graph"
      data-testid="knowledge-graph"
      aria-label="Knowledge graph"
    >
      <p id={descriptionId} className="sr-only">
        {graphDescription(graph)}
      </p>
      <svg
        className="graph-canvas"
        viewBox={`0 0 ${width} ${height}`}
        role="img"
        aria-label={`${graph.nodes.length} nodes and ${graph.edges.length} edges`}
        aria-describedby={descriptionId}
      >
        <g className="graph-edges">
          {graph.edges.map((edge) => {
            const coordinates = edgeCoordinates(edge, positions);

            if (!coordinates) {
              return null;
            }

            return (
              <line
                key={edge.id}
                data-edge-id={edge.id}
                x1={coordinates.source.x}
                y1={coordinates.source.y}
                x2={coordinates.target.x}
                y2={coordinates.target.y}
                stroke="#94a3b8"
                strokeWidth="2"
                strokeLinecap="round"
              />
            );
          })}
        </g>
        <g className="graph-nodes">
          {graph.nodes.map((node) => {
            const position = positions.get(node.id) ?? { x: centerX, y: centerY };

            return (
              <g
                key={node.id}
                data-node-id={node.id}
                transform={`translate(${position.x} ${position.y})`}
              >
                <circle
                  r={nodeRadius}
                  fill={nodeFillByKind[node.kind]}
                  stroke="#334155"
                  strokeWidth="2"
                />
                {showVisibleLabels ? (
                  <text
                    y={nodeRadius + 20}
                    textAnchor="middle"
                    fill="#0f172a"
                    fontSize="13"
                    fontFamily="system-ui, sans-serif"
                  >
                    {labelForNode(node)}
                  </text>
                ) : null}
                <title>{`${node.kind}: ${node.label}`}</title>
              </g>
            );
          })}
        </g>
      </svg>
    </section>
  );
}
