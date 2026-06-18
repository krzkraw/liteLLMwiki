import { describe, expect, it } from "vitest";
import type { FolderManifest } from "./folderIndex";
import { buildKnowledgeGraph } from "./knowledgeGraph";
import type { SummaryResult } from "./summarization";

describe("knowledgeGraph", () => {
  it("builds folder, file, and entity graph nodes", () => {
    const manifest = {
      id: "manifest-demo",
      rootName: "demo",
      sourceKind: "file-list",
      files: [
        {
          id: "file-a",
          path: "src/a.ts",
          sourceKind: "file-list",
          name: "a.ts",
          extension: ".ts",
          mimeType: "text/plain",
          sizeBytes: 1,
          lastModified: 1000,
          status: "indexed",
          chunkIds: [],
        },
      ],
      chunks: [],
      stats: {
        totalFiles: 1,
        indexedFiles: 1,
        ignoredFiles: 0,
        errorFiles: 0,
        totalBytes: 1,
        indexedBytes: 1,
        chunkCount: 0,
      },
    } as FolderManifest;
    const summaries = {
      chunkSummaries: [],
      fileSummaries: [
        {
          fileId: "file-a",
          path: "src/a.ts",
          summary: "Uses Provider",
          entities: ["Provider"],
        },
      ],
      folderSummary: { text: "Demo folder", topics: ["Provider"] },
    } as SummaryResult;

    const graph = buildKnowledgeGraph(manifest, summaries);

    expect(graph.nodes.map((node) => node.id).sort()).toEqual([
      "entity-provider",
      "file-a",
      "folder-demo",
      "topic-provider",
    ]);
    expect(graph.edges.some((edge) => edge.kind === "contains")).toBe(true);
    expect(graph.edges.some((edge) => edge.kind === "mentions")).toBe(true);
  });

  it("sorts graph nodes and edges stably", () => {
    const manifest = {
      id: "manifest-demo",
      rootName: "demo",
      sourceKind: "file-list",
      files: [
        {
          id: "file-z",
          path: "z.ts",
          sourceKind: "file-list",
          name: "z.ts",
          extension: ".ts",
          mimeType: "text/plain",
          sizeBytes: 1,
          lastModified: 1000,
          status: "indexed",
          chunkIds: ["chunk-z"],
        },
      ],
      chunks: [
        {
          id: "chunk-z",
          fileId: "file-z",
          path: "z.ts",
          index: 0,
          text: "Provider Runtime",
          startOffset: 0,
          endOffset: 16,
          hash: "chunkhash",
          bytes: 16,
        },
      ],
      stats: {
        totalFiles: 1,
        indexedFiles: 1,
        ignoredFiles: 0,
        errorFiles: 0,
        totalBytes: 1,
        indexedBytes: 1,
        chunkCount: 1,
      },
    } as FolderManifest;
    const summaries = {
      chunkSummaries: [
        {
          chunkId: "chunk-z",
          fileId: "file-z",
          path: "z.ts",
          summary: "Mentions Runtime and Provider",
          entities: ["Runtime", "Provider"],
        },
      ],
      fileSummaries: [
        {
          fileId: "file-z",
          path: "z.ts",
          summary: "Uses Runtime and Provider",
          entities: ["Runtime", "Provider"],
        },
      ],
      folderSummary: { text: "Demo folder", topics: ["Runtime", "Provider"] },
    } as SummaryResult;

    const graph = buildKnowledgeGraph(manifest, summaries);

    expect(graph.nodes.map((node) => node.id)).toEqual(
      graph.nodes.map((node) => node.id).sort(),
    );
    expect(graph.edges.map((edge) => edge.id)).toEqual(
      graph.edges.map((edge) => edge.id).sort(),
    );
  });

  it("ignores chunks that indexed files do not reference", () => {
    const manifest = {
      id: "manifest-demo",
      rootName: "demo",
      sourceKind: "file-list",
      files: [
        {
          id: "file-a",
          path: "src/a.ts",
          sourceKind: "file-list",
          name: "a.ts",
          extension: ".ts",
          mimeType: "text/plain",
          sizeBytes: 10,
          lastModified: 1000,
          status: "indexed",
          chunkIds: ["chunk-attached"],
        },
      ],
      chunks: [
        {
          id: "chunk-attached",
          fileId: "file-a",
          path: "src/a.ts",
          index: 0,
          text: "Provider",
          startOffset: 0,
          endOffset: 8,
          hash: "attached",
          bytes: 8,
        },
        {
          id: "chunk-stale",
          fileId: "file-a",
          path: "src/a.ts",
          index: 1,
          text: "StaleEntity",
          startOffset: 8,
          endOffset: 19,
          hash: "stale",
          bytes: 11,
        },
      ],
      stats: {
        totalFiles: 1,
        indexedFiles: 1,
        ignoredFiles: 0,
        errorFiles: 0,
        totalBytes: 10,
        indexedBytes: 10,
        chunkCount: 2,
      },
    } as FolderManifest;
    const summaries = {
      chunkSummaries: [
        {
          chunkId: "chunk-attached",
          fileId: "file-a",
          path: "src/a.ts",
          summary: "Provider",
          entities: ["Provider"],
        },
        {
          chunkId: "chunk-stale",
          fileId: "file-a",
          path: "src/a.ts",
          summary: "StaleEntity",
          entities: ["StaleEntity"],
        },
      ],
      fileSummaries: [],
      folderSummary: { text: "Demo folder", topics: [] },
    } as SummaryResult;

    const graph = buildKnowledgeGraph(manifest, summaries);

    expect(graph.nodes.map((node) => node.id)).toContain("chunk-attached");
    expect(graph.nodes.map((node) => node.id)).not.toContain("chunk-stale");
    expect(graph.nodes.map((node) => node.label)).not.toContain("StaleEntity");
  });

  it("keeps distinct entity and topic labels when slugs collide", () => {
    const manifest = {
      id: "manifest-demo",
      rootName: "demo",
      sourceKind: "file-list",
      files: [
        {
          id: "file-a",
          path: "src/a.ts",
          sourceKind: "file-list",
          name: "a.ts",
          extension: ".ts",
          mimeType: "text/plain",
          sizeBytes: 1,
          lastModified: 1000,
          status: "indexed",
          chunkIds: [],
        },
      ],
      chunks: [],
      stats: {
        totalFiles: 1,
        indexedFiles: 1,
        ignoredFiles: 0,
        errorFiles: 0,
        totalBytes: 1,
        indexedBytes: 1,
        chunkCount: 0,
      },
    } as FolderManifest;
    const summaries = {
      chunkSummaries: [],
      fileSummaries: [
        {
          fileId: "file-a",
          path: "src/a.ts",
          summary: "Languages",
          entities: ["C++", "C#"],
        },
      ],
      folderSummary: { text: "Demo folder", topics: ["C++", "C#"] },
    } as SummaryResult;

    const graph = buildKnowledgeGraph(manifest, summaries);
    const entityNodes = graph.nodes.filter((node) => node.kind === "entity");
    const topicNodes = graph.nodes.filter((node) => node.kind === "topic");

    expect(entityNodes.map((node) => node.label).sort()).toEqual(["C#", "C++"]);
    expect(new Set(entityNodes.map((node) => node.id))).toHaveLength(2);
    expect(topicNodes.map((node) => node.label).sort()).toEqual(["C#", "C++"]);
    expect(new Set(topicNodes.map((node) => node.id))).toHaveLength(2);
  });
});
