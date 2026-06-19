export type NativeBackend = "auto" | "cpu" | "gpu" | "npu" | "cuda";
export type BackendState =
  | "available"
  | "unavailable"
  | "unknown"
  | "not-a-litert-backend";

export interface BackendStatus {
  backend: NativeBackend;
  state: BackendState;
  detail?: string;
}

export const nativeBackends: NativeBackend[] = ["auto", "cpu", "gpu", "npu", "cuda"];

export function isLiteRtBackend(backend: NativeBackend): boolean {
  return backend !== "cuda";
}

export function getDefaultBackendStatuses(): BackendStatus[] {
  return nativeBackends.map((backend) => ({
    backend,
    state: backend === "cuda" ? "not-a-litert-backend" : "unknown",
  }));
}
