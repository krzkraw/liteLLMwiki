import { describe, expect, it } from "vitest";
import { summarizeRuntimeAudit, type RuntimeAuditInput } from "./runtimeAudit";

describe("summarizeRuntimeAudit", () => {
  it("marks the runtime ready when WebGPU, WASM, and model preflight are ready", () => {
    const input: RuntimeAuditInput = {
      webGpuState: "ready",
      modelState: "ready",
      hasWebAssembly: true,
      crossOriginIsolated: false,
    };

    expect(summarizeRuntimeAudit(input)).toEqual({
      state: "ready",
      label: "Runtime ready",
      detail: "WebGPU, WASM, and the configured model path are available.",
    });
  });

  it("blocks model load when WebGPU is missing", () => {
    expect(
      summarizeRuntimeAudit({
        webGpuState: "missing",
        modelState: "ready",
        hasWebAssembly: true,
        crossOriginIsolated: false,
      }),
    ).toEqual({
      state: "blocked",
      label: "Runtime blocked",
      detail: "WebGPU is required for the browser Gemma provider.",
    });
  });

  it("asks for model preflight before loading", () => {
    expect(
      summarizeRuntimeAudit({
        webGpuState: "ready",
        modelState: "idle",
        hasWebAssembly: true,
        crossOriginIsolated: false,
      }),
    ).toEqual({
      state: "needs-model",
      label: "Model not checked",
      detail: "Run model preflight before loading the Gemma 4 E2B web model.",
    });
  });

  it("reports missing WebAssembly before trying LiteRT-LM", () => {
    expect(
      summarizeRuntimeAudit({
        webGpuState: "ready",
        modelState: "ready",
        hasWebAssembly: false,
        crossOriginIsolated: false,
      }),
    ).toEqual({
      state: "blocked",
      label: "Runtime blocked",
      detail: "WebAssembly is required to load the LiteRT-LM runtime.",
    });
  });
});
