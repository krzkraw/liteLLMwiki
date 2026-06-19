import type { ModelProbeState } from "./modelConfig";
import type { WebGpuState } from "./webgpu";

export type RuntimeAuditState = "ready" | "needs-model" | "blocked";

export interface RuntimeAuditInput {
  webGpuState: WebGpuState;
  modelState: ModelProbeState;
  hasWebAssembly: boolean;
  crossOriginIsolated: boolean;
}

export interface RuntimeAuditSummary {
  state: RuntimeAuditState;
  label: string;
  detail: string;
}

export function summarizeRuntimeAudit(input: RuntimeAuditInput): RuntimeAuditSummary {
  if (!input.hasWebAssembly) {
    return {
      state: "blocked",
      label: "Runtime blocked",
      detail: "WebAssembly is required to load the LiteRT-LM runtime.",
    };
  }

  if (input.webGpuState !== "ready") {
    return {
      state: "blocked",
      label: "Runtime blocked",
      detail: "WebGPU is required for the browser Gemma provider.",
    };
  }

  if (input.modelState !== "ready") {
    return {
      state: "needs-model",
      label: "Model not checked",
      detail: "Run model preflight before loading the Gemma 4 E2B web model.",
    };
  }

  return {
    state: "ready",
    label: "Runtime ready",
    detail: input.crossOriginIsolated
      ? "WebGPU, WASM, isolation, and the configured model path are available."
      : "WebGPU, WASM, and the configured model path are available.",
  };
}
