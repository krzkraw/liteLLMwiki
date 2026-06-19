import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

describe("model file script paths", () => {
  it("defaults web model tools to the repository-local models directory", async () => {
    const { defaultWebModelRelativePath, resolveWebModelPath } = await import(
      "./modelFiles.mjs"
    );

    expect(defaultWebModelRelativePath).toBe(
      "models/gemma-4-E2B-it-web.litertlm",
    );
    expect(resolveWebModelPath("/repo")).toBe(
      resolve("/repo", "models/gemma-4-E2B-it-web.litertlm"),
    );
  });

  it("honors an explicit model path argument", async () => {
    const { resolveWebModelPath } = await import("./modelFiles.mjs");

    expect(resolveWebModelPath("/repo", "../models/custom.litertlm")).toBe(
      resolve("/repo", "../models/custom.litertlm"),
    );
  });
});
