import { describe, expect, it } from "bun:test";
import {
  classifyModelResponse,
  classifySelectedModelFile,
  defaultModelPath,
  defaultWasmPath,
  formatBytes,
  gemma4E2bWebFilename,
  getModelStatusMessage,
  getSuggestedModelUrl,
} from "./modelConfig";

async function readProjectFile(path: string): Promise<string> {
  const fs = (await import("" + "fs")) as {
    readFileSync: (path: string, encoding: "utf8") => string;
  };

  return fs.readFileSync(path, "utf8");
}

describe("Gemma 4 browser model config", () => {
  it("uses the Gemma 4 E2B web model by default", () => {
    expect(gemma4E2bWebFilename).toBe("gemma-4-E2B-it-web.litertlm");
    expect(defaultModelPath).toBe(
      "/models/litert/browser/gemma-4-E2B-it-web.litertlm",
    );
  });

  it("returns the LiteRT Community resolve URL for the web model", () => {
    expect(getSuggestedModelUrl()).toBe(
      "https://huggingface.co/litert-community/gemma-4-E2B-it-litert-lm/resolve/main/gemma-4-E2B-it-web.litertlm",
    );
  });

  it("uses local vendored LiteRT-LM WASM assets", () => {
    expect(defaultWasmPath).toBe("/vendor/litert-lm/core/wasm");
  });

  it("syncs LiteRT-LM WASM assets", async () => {
    const source = await readProjectFile("scripts/sync-wasm.mjs");

    expect(source).toContain("node_modules/@litert-lm/core/wasm");
    expect(source).toContain("public/vendor/litert-lm/core/wasm");
  });

  it("downloads the public Gemma 4 model without requiring Hugging Face auth", async () => {
    const source = await readProjectFile("scripts/download-model.mjs");

    expect(source).not.toContain("if (!token)");
    expect(source).toContain("const requestInit = token");
  });

  it("accepts the web model file for the browser provider", () => {
    expect(
      classifySelectedModelFile({
        name: "gemma-4-E2B-it-web.litertlm",
        size: 1_900_000_000,
      }),
    ).toMatchObject({ state: "ready" });
  });

  it("blocks the native model file for the browser provider", () => {
    expect(
      classifySelectedModelFile({
        name: "gemma-4-E2B-it.litertlm",
        size: 2_400_000_000,
      }),
    ).toEqual({
      state: "blocked",
      message: "Select the Gemma 4 E2B web .litertlm model for browser WebGPU.",
    });
  });
});

describe("formatBytes", () => {
  it("formats bytes using binary units", () => {
    expect(formatBytes(4_294_967_296)).toBe("4.00 GiB");
    expect(formatBytes(1_048_576)).toBe("1.00 MiB");
  });
});

describe("getModelStatusMessage", () => {
  it("describes a missing local model", () => {
    expect(getModelStatusMessage("missing")).toBe(
      "Model file not found at the configured path.",
    );
  });

  it("describes a reachable local model with its size", () => {
    expect(getModelStatusMessage("ready", 4_294_967_296)).toBe(
      "Model reachable, reported size 4.00 GiB.",
    );
  });
});

describe("classifyModelResponse", () => {
  it("treats dev-server HTML fallback for a litertlm path as a missing model", () => {
    expect(
      classifyModelResponse({
        ok: true,
        status: 200,
        contentType: "text/html",
        contentLength: "590",
        modelPath: "/models/litert/browser/gemma-4-E2B-it-web.litertlm",
      }),
    ).toEqual({ state: "missing" });
  });

  it("accepts non-HTML responses for litertlm files", () => {
    expect(
      classifyModelResponse({
        ok: true,
        status: 200,
        contentType: "application/octet-stream",
        contentLength: "4294967296",
        modelPath: "/models/litert/browser/gemma-4-E2B-it-web.litertlm",
      }),
    ).toEqual({ state: "ready", sizeBytes: 4_294_967_296 });
  });
});

describe("classifySelectedModelFile", () => {
  it("accepts a local litertlm file selected through the browser", () => {
    expect(
      classifySelectedModelFile({
        name: "gemma-4-E2B-it-web.litertlm",
        size: 4_294_967_296,
      }),
    ).toEqual({
      state: "ready",
      sizeBytes: 4_294_967_296,
      message: "Selected local model file, reported size 4.00 GiB.",
    });
  });

  it("rejects non-litertlm files", () => {
    expect(
      classifySelectedModelFile({
        name: "model.bin",
        size: 4_294_967_296,
      }),
    ).toEqual({
      state: "blocked",
      message: "Select a .litertlm model file.",
    });
  });
});
