import { describe, expect, it } from "bun:test";
import type { SourceFile } from "./folderAccess";
import { indexFolder, shouldIgnorePath } from "./folderIndex";

describe("folderIndex", () => {
  it("ignores generated, binary, media, and model paths", () => {
    expect(shouldIgnorePath("node_modules/react/index.js")).toBe(
      "ignored-directory",
    );
    expect(shouldIgnorePath("dist/app.js")).toBe("ignored-directory");
    expect(shouldIgnorePath("models/litert/browser/gemma-4-E2B-it-web.litertlm")).toBe(
      "model-file",
    );
    expect(shouldIgnorePath("image.png")).toBe("binary-or-media");
  });

  it("ignores path segments and extensions case-insensitively", () => {
    expect(shouldIgnorePath("src/Node_Modules/react/index.js")).toBe(
      "ignored-directory",
    );
    expect(shouldIgnorePath(".VENV/lib/site.py")).toBe("ignored-directory");
    expect(shouldIgnorePath("assets/PHOTO.PNG")).toBe("binary-or-media");
    expect(shouldIgnorePath("models/Gemma-4-E2B-it-web.LITERTLM")).toBe(
      "model-file",
    );
  });

  it("builds stable file and chunk records for text files", async () => {
    const files: SourceFile[] = [
      {
        path: "src/main.ts",
        sourceKind: "file-list",
        file: new File(["export const answer = 42;\n"], "main.ts", {
          type: "text/plain",
          lastModified: 1000,
        }),
      },
    ];

    const manifest = await indexFolder(files, {
      chunkSize: 12,
      maxFileBytes: 1024,
    });
    const secondManifest = await indexFolder(files, {
      chunkSize: 12,
      maxFileBytes: 1024,
    });

    expect(manifest.files).toHaveLength(1);
    expect(manifest.files[0]).toMatchObject({
      path: "src/main.ts",
      status: "indexed",
      chunkIds: manifest.chunks.map((chunk) => chunk.id),
    });
    expect(manifest.files[0].hash).toMatch(/^[a-f0-9]+$/);
    expect(manifest.chunks.map((chunk) => chunk.path)).toEqual([
      "src/main.ts",
      "src/main.ts",
    ]);
    expect(manifest.chunks.map((chunk) => chunk.text)).toEqual([
      "export const",
      "answer = 42;",
    ]);
    expect(manifest.chunks.map((chunk) => chunk.id)).toEqual(
      secondManifest.chunks.map((chunk) => chunk.id),
    );
    expect(manifest.stats).toMatchObject({
      totalFiles: 1,
      indexedFiles: 1,
      ignoredFiles: 0,
      chunkCount: 2,
    });
  });

  it("exposes fields used by summarization and graph tasks", async () => {
    const files: SourceFile[] = [
      {
        path: "src/main.ts",
        sourceKind: "file-list",
        file: new File(["export const answer = 42;\n"], "main.ts", {
          type: "text/plain",
          lastModified: 1000,
        }),
      },
    ];

    const manifest = await indexFolder(files, {
      chunkSize: 12,
      maxFileBytes: 1024,
      rootName: "demo",
    });

    expect(manifest).toMatchObject({
      id: expect.stringMatching(/^manifest:/),
      rootName: "demo",
      sourceKind: "file-list",
    });
    expect(manifest.files[0]).toMatchObject({
      extension: ".ts",
      mimeType: "text/plain",
      sizeBytes: 26,
    });
    expect(manifest.chunks[0]).toMatchObject({
      startOffset: 0,
      endOffset: 12,
      hash: expect.stringMatching(/^[a-f0-9]+$/),
    });
  });

  it("skips files larger than the configured maximum", async () => {
    const files: SourceFile[] = [
      {
        path: "src/large.txt",
        sourceKind: "file-list",
        file: new File(["large file text"], "large.txt", {
          type: "text/plain",
        }),
      },
    ];

    const manifest = await indexFolder(files, {
      chunkSize: 10,
      maxFileBytes: 4,
    });

    expect(manifest.files).toEqual([
      expect.objectContaining({
        path: "src/large.txt",
        status: "ignored",
        ignoreReason: "too-large",
        chunkIds: [],
      }),
    ]);
    expect(manifest.chunks).toEqual([]);
    expect(manifest.stats).toMatchObject({
      indexedFiles: 0,
      ignoredFiles: 1,
      chunkCount: 0,
    });
  });
});
