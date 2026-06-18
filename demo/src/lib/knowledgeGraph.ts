import type { FolderManifest } from "./folderIndex";
import type { SummaryResult } from "./summarization";

export type KnowledgeGraphNodeKind = "folder" | "file" | "chunk" | "entity" | "topic";

export type KnowledgeGraphEdgeKind =
  | "contains"
  | "mentions"
  | "relates_to"
  | "summarizes"
  | "co_occurs"
  | "references";

export interface KnowledgeGraphNode {
  id: string;
  kind: KnowledgeGraphNodeKind;
  label: string;
  weight?: number;
  sourceIds?: string[];
}

export interface KnowledgeGraphEdge {
  id: string;
  source: string;
  target: string;
  kind: KnowledgeGraphEdgeKind;
  label?: string;
  weight?: number;
}

export interface KnowledgeGraph {
  nodes: KnowledgeGraphNode[];
  edges: KnowledgeGraphEdge[];
}

function slugify(value: string): string {
  return value
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

function normalizedLabelForSlug(value: string): string {
  return value
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .trim()
    .replace(/\s+/g, "-")
    .replace(/^-+|-+$/g, "");
}

function stableLabelHash(value: string): string {
  let hash = 0x811c9dc5;

  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }

  return hash.toString(36).padStart(6, "0").slice(0, 6);
}

function graphId(prefix: string, label: string): string {
  const slug = slugify(label) || "item";

  if (normalizedLabelForSlug(label) === slug) {
    return `${prefix}-${slug}`;
  }

  return `${prefix}-${slug}-${stableLabelHash(label)}`;
}

function edgeId(
  kind: KnowledgeGraphEdgeKind,
  source: string,
  target: string,
): string {
  return `${kind}:${source}->${target}`;
}

function chunkRef(fileId: string, chunkId: string): string {
  return `${fileId}\0${chunkId}`;
}

function mergeSourceIds(
  left: string[] | undefined,
  right: string[] | undefined,
): string[] | undefined {
  const values = [...(left ?? []), ...(right ?? [])];

  if (values.length === 0) {
    return undefined;
  }

  return Array.from(new Set(values)).sort();
}

function addNode(
  nodes: Map<string, KnowledgeGraphNode>,
  node: KnowledgeGraphNode,
) {
  const existing = nodes.get(node.id);

  if (!existing) {
    nodes.set(node.id, {
      ...node,
      sourceIds: node.sourceIds ? [...node.sourceIds].sort() : undefined,
    });
    return;
  }

  nodes.set(node.id, {
    ...existing,
    weight: Math.max(existing.weight ?? 0, node.weight ?? 0) || undefined,
    sourceIds: mergeSourceIds(existing.sourceIds, node.sourceIds),
  });
}

function addEdge(
  edges: Map<string, KnowledgeGraphEdge>,
  edge: Omit<KnowledgeGraphEdge, "id">,
) {
  const id = edgeId(edge.kind, edge.source, edge.target);
  const existing = edges.get(id);

  if (!existing) {
    edges.set(id, { id, ...edge });
    return;
  }

  edges.set(id, {
    ...existing,
    weight: Math.max(existing.weight ?? 0, edge.weight ?? 0) || undefined,
  });
}

export function buildKnowledgeGraph(
  manifest: FolderManifest,
  summaries: SummaryResult,
): KnowledgeGraph {
  const nodes = new Map<string, KnowledgeGraphNode>();
  const edges = new Map<string, KnowledgeGraphEdge>();
  const folderId = graphId("folder", manifest.rootName);
  const indexedFiles = manifest.files.filter((file) => file.status === "indexed");
  const indexedFileIds = new Set(indexedFiles.map((file) => file.id));
  const validChunkRefs = new Set(
    indexedFiles.flatMap((file) =>
      file.chunkIds.map((chunkId) => chunkRef(file.id, chunkId)),
    ),
  );

  addNode(nodes, {
    id: folderId,
    kind: "folder",
    label: manifest.rootName,
    weight: manifest.stats.indexedFiles,
    sourceIds: [manifest.id],
  });

  for (const file of indexedFiles) {
    addNode(nodes, {
      id: file.id,
      kind: "file",
      label: file.path,
      weight: file.sizeBytes,
      sourceIds: [file.id],
    });
    addEdge(edges, {
      source: folderId,
      target: file.id,
      kind: "contains",
      label: "contains",
      weight: 1,
    });
  }

  for (const chunk of manifest.chunks.filter((item) =>
    validChunkRefs.has(chunkRef(item.fileId, item.id)),
  )) {
    addNode(nodes, {
      id: chunk.id,
      kind: "chunk",
      label: `${chunk.path} #${chunk.index + 1}`,
      weight: chunk.bytes,
      sourceIds: [chunk.id, chunk.fileId],
    });
    addEdge(edges, {
      source: chunk.fileId,
      target: chunk.id,
      kind: "contains",
      label: "contains",
      weight: 1,
    });
  }

  for (const topic of summaries.folderSummary.topics) {
    const topicId = graphId("topic", topic);

    addNode(nodes, {
      id: topicId,
      kind: "topic",
      label: topic,
      weight: 1,
      sourceIds: [manifest.id],
    });
    addEdge(edges, {
      source: folderId,
      target: topicId,
      kind: "relates_to",
      label: "topic",
      weight: 1,
    });
  }

  for (const fileSummary of summaries.fileSummaries) {
    if (!indexedFileIds.has(fileSummary.fileId)) {
      continue;
    }

    for (const entity of fileSummary.entities) {
      const entityId = graphId("entity", entity);

      addNode(nodes, {
        id: entityId,
        kind: "entity",
        label: entity,
        weight: 1,
        sourceIds: [fileSummary.fileId],
      });
      addEdge(edges, {
        source: fileSummary.fileId,
        target: entityId,
        kind: "mentions",
        label: "mentions",
        weight: 1,
      });
    }
  }

  for (const chunkSummary of summaries.chunkSummaries) {
    if (
      !indexedFileIds.has(chunkSummary.fileId) ||
      !validChunkRefs.has(chunkRef(chunkSummary.fileId, chunkSummary.chunkId))
    ) {
      continue;
    }

    for (const entity of chunkSummary.entities) {
      const entityId = graphId("entity", entity);

      addNode(nodes, {
        id: entityId,
        kind: "entity",
        label: entity,
        weight: 1,
        sourceIds: [chunkSummary.chunkId, chunkSummary.fileId],
      });
      addEdge(edges, {
        source: chunkSummary.chunkId,
        target: entityId,
        kind: "mentions",
        label: "mentions",
        weight: 1,
      });
    }
  }

  return {
    nodes: Array.from(nodes.values()).sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
    edges: Array.from(edges.values()).sort((left, right) =>
      left.id.localeCompare(right.id),
    ),
  };
}
