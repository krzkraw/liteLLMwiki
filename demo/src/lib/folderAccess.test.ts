import { describe, expect, it } from "vitest";
import {
  collectFilesFromDirectory,
  collectFilesFromFileList,
  detectFolderCapabilities,
  normalizeRelativePath,
} from "./folderAccess";

describe("folderAccess", () => {
  it("normalizes Windows separators to browser graph paths", () => {
    expect(normalizeRelativePath("src\\lib\\App.tsx")).toBe("src/lib/App.tsx");
  });

  it("removes leading relative and absolute path prefixes", () => {
    expect(normalizeRelativePath("./demo\\src/App.tsx")).toBe(
      "demo/src/App.tsx",
    );
    expect(normalizeRelativePath("/demo/src/App.tsx")).toBe(
      "demo/src/App.tsx",
    );
  });

  it("collects webkitRelativePath values from file inputs", () => {
    const file = new File(["hello"], "App.tsx", { type: "text/plain" });
    Object.defineProperty(file, "webkitRelativePath", {
      value: "demo/src/App.tsx",
    });

    expect(collectFilesFromFileList([file])).toEqual([
      { path: "demo/src/App.tsx", file, sourceKind: "file-list" },
    ]);
  });

  it("falls back to file names when no relative path is present", () => {
    const file = new File(["hello"], "App.tsx", { type: "text/plain" });

    expect(collectFilesFromFileList([file])).toEqual([
      { path: "App.tsx", file, sourceKind: "file-list" },
    ]);
  });

  it("detects directory picker capability from a supplied window-like object", () => {
    expect(
      detectFolderCapabilities({ showDirectoryPicker: async () => undefined }),
    ).toEqual({
      hasShowDirectoryPicker: true,
      hasWebkitDirectory: false,
    });
  });

  it("collects recursive directory handle files under the selected folder name", async () => {
    const mainFile = new File(["main"], "main.ts", { type: "text/plain" });
    const readmeFile = new File(["readme"], "README.md", { type: "text/markdown" });
    const srcHandle = {
      kind: "directory",
      name: "src",
      async *entries() {
        yield [
          "main.ts",
          {
            kind: "file",
            name: "main.ts",
            getFile: async () => mainFile,
          },
        ];
      },
    } as unknown as FileSystemDirectoryHandle;
    const rootHandle = {
      kind: "directory",
      name: "demo",
      async *entries() {
        yield ["src", srcHandle];
        yield [
          "README.md",
          {
            kind: "file",
            name: "README.md",
            getFile: async () => readmeFile,
          },
        ];
      },
    } as unknown as FileSystemDirectoryHandle;

    await expect(collectFilesFromDirectory(rootHandle)).resolves.toEqual([
      { path: "demo/src/main.ts", file: mainFile, sourceKind: "directory-handle" },
      { path: "demo/README.md", file: readmeFile, sourceKind: "directory-handle" },
    ]);
  });
});
