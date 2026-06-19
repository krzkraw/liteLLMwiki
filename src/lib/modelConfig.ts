export const gemma4E2bRepo = "litert-community/gemma-4-E2B-it-litert-lm";
export const gemma4E2bWebFilename = "gemma-4-E2B-it-web.litertlm";
export const gemma4E2bNativeFilename = "gemma-4-E2B-it.litertlm";
export const defaultModelPath = `/models/${gemma4E2bWebFilename}`;
export const defaultWasmPath = "/vendor/litert-lm/core/wasm";

export type ModelProbeState = "idle" | "checking" | "ready" | "missing" | "blocked";

export function getSuggestedModelUrl(): string {
  return `https://huggingface.co/${gemma4E2bRepo}/resolve/main/${gemma4E2bWebFilename}`;
}

export function formatBytes(bytes: number): string {
  const units = ["bytes", "KiB", "MiB", "GiB", "TiB"];
  let value = bytes;
  let unitIndex = 0;

  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }

  if (unitIndex === 0) {
    return `${value} bytes`;
  }

  return `${value.toFixed(2)} ${units[unitIndex]}`;
}

export function getModelStatusMessage(
  state: Exclude<ModelProbeState, "idle" | "checking">,
  sizeBytes?: number,
): string {
  if (state === "ready") {
    return sizeBytes
      ? `Model reachable, reported size ${formatBytes(sizeBytes)}.`
      : "Model reachable.";
  }

  if (state === "missing") {
    return "Model file not found at the configured path.";
  }

  return "Model check failed. The path may require auth, CORS, or a reachable local file.";
}

export interface ModelResponseLike {
  ok: boolean;
  status: number;
  contentType: string | null;
  contentLength: string | null;
  modelPath: string;
}

export interface SelectedModelFileLike {
  name: string;
  size: number;
}

export function classifySelectedModelFile(file: SelectedModelFileLike): {
  state: Exclude<ModelProbeState, "idle" | "checking">;
  sizeBytes?: number;
  message: string;
} {
  if (!file.name.toLowerCase().endsWith(".litertlm")) {
    return {
      state: "blocked",
      message: "Select a .litertlm model file.",
    };
  }

  if (file.name === gemma4E2bNativeFilename) {
    return {
      state: "blocked",
      message: "Select the Gemma 4 E2B web .litertlm model for browser WebGPU.",
    };
  }

  return {
    state: "ready",
    sizeBytes: file.size,
    message: `Selected local model file, reported size ${formatBytes(file.size)}.`,
  };
}

export function classifyModelResponse(response: ModelResponseLike): {
  state: Exclude<ModelProbeState, "idle" | "checking">;
  sizeBytes?: number;
} {
  if (response.ok) {
    if (
      response.modelPath.endsWith(".litertlm") &&
      response.contentType?.toLowerCase().includes("text/html")
    ) {
      return { state: "missing" };
    }

    return {
      state: "ready",
      sizeBytes: response.contentLength ? Number(response.contentLength) : undefined,
    };
  }

  return { state: response.status === 404 ? "missing" : "blocked" };
}

export async function probeModelPath(modelPath: string): Promise<{
  state: Exclude<ModelProbeState, "idle" | "checking">;
  sizeBytes?: number;
}> {
  try {
    const response = await fetch(modelPath, { method: "HEAD" });
    return classifyModelResponse({
      ok: response.ok,
      status: response.status,
      contentType: response.headers.get("content-type"),
      contentLength: response.headers.get("content-length"),
      modelPath,
    });
  } catch {
    return { state: "blocked" };
  }
}
