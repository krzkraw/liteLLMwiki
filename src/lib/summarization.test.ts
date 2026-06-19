import { describe, expect, it } from "bun:test";
import type { FolderManifest } from "./folderIndex";
import {
  createDeterministicSummarizer,
  createProviderSummarizer,
  summarizeFolderManifest,
} from "./summarization";

function createManifest(): FolderManifest {
  return {
    id: "manifest-demo",
    rootName: "demo",
    sourceKind: "file-list",
    files: [
      {
        id: "file-src-main-ts",
        path: "src/main.ts",
        sourceKind: "file-list",
        name: "main.ts",
        extension: ".ts",
        mimeType: "text/plain",
        sizeBytes: 24,
        lastModified: 1000,
        hash: "abc",
        status: "indexed",
        chunkIds: ["chunk-1"],
      },
    ],
    chunks: [
      {
        id: "chunk-1",
        fileId: "file-src-main-ts",
        path: "src/main.ts",
        index: 0,
        startOffset: 0,
        endOffset: 24,
        text: "export const answer = 42;",
        hash: "chunkhash",
        bytes: 24,
      },
    ],
    stats: {
      totalFiles: 1,
      indexedFiles: 1,
      ignoredFiles: 0,
      errorFiles: 0,
      totalBytes: 24,
      indexedBytes: 24,
      chunkCount: 1,
    },
  };
}

describe("summarization", () => {
  it("summarizes chunks, files, and folder without model inference", async () => {
    const result = await summarizeFolderManifest(
      createManifest(),
      createDeterministicSummarizer(),
    );

    expect(result.folderSummary.text).toContain("1 indexed file");
    expect(result.fileSummaries[0].summary).toContain("answer");
    expect(result.chunkSummaries[0].entities).toContain("answer");
  });

  it("wraps a provider in the summarizer contract", async () => {
    const prompts: string[] = [];
    const summarizer = createProviderSummarizer({
      async generateText(prompt) {
        prompts.push(prompt);
        return `Summary ${prompts.length}\nEntities: Provider, answer`;
      },
    });

    const result = await summarizeFolderManifest(createManifest(), summarizer);

    expect(prompts).toHaveLength(3);
    expect(result.chunkSummaries[0].summary).toContain("Summary 1");
    expect(result.fileSummaries[0].entities).toEqual(["Provider", "answer"]);
    expect(result.folderSummary.topics).toEqual(["Provider", "answer"]);
  });

  it("ignores chunks that are not attached to indexed files", async () => {
    const manifest = createManifest();
    manifest.chunks.push({
      id: "chunk-orphan",
      fileId: "file-ignored",
      path: "ignored.ts",
      index: 0,
      startOffset: 0,
      endOffset: 20,
      text: "const hiddenProvider = true;",
      hash: "orphanhash",
      bytes: 20,
    });
    manifest.chunks.push({
      id: "chunk-stale",
      fileId: "file-src-main-ts",
      path: "src/main.ts",
      index: 1,
      startOffset: 24,
      endOffset: 52,
      text: "const hiddenProvider = true;",
      hash: "stalehash",
      bytes: 28,
    });

    const result = await summarizeFolderManifest(
      manifest,
      createDeterministicSummarizer(),
    );

    expect(result.chunkSummaries.map((summary) => summary.chunkId)).toEqual([
      "chunk-1",
    ]);
    expect(result.folderSummary.topics).not.toContain("hiddenProvider");
  });

  it("normalizes provider none values and multiline entity lists", async () => {
    const responses = [
      "Chunk summary\nEntities: none",
      "File summary\nEntities:\n- Runtime\n- Provider",
      "Folder summary\nTopics:\n- Runtime\n- Provider",
    ];
    const summarizer = createProviderSummarizer({
      async generateText() {
        return responses.shift() ?? "Summary\nEntities: none";
      },
    });

    const result = await summarizeFolderManifest(createManifest(), summarizer);

    expect(result.chunkSummaries[0].entities).toEqual([]);
    expect(result.fileSummaries[0].entities).toEqual(["Runtime", "Provider"]);
    expect(result.folderSummary.topics).toEqual(["Runtime", "Provider"]);
  });
});
