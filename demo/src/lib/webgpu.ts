export type WebGpuState = "checking" | "missing" | "blocked" | "ready";

export interface WebGpuStatus {
  state: WebGpuState;
  label: string;
  detail: string;
}

export function describeWebGpuStatus(
  hasNavigatorGpu: boolean,
  adapterName: string | null | undefined,
): WebGpuStatus {
  if (!hasNavigatorGpu) {
    return {
      state: "missing",
      label: "WebGPU unavailable",
      detail: "This browser does not expose navigator.gpu.",
    };
  }

  if (adapterName === null) {
    return {
      state: "blocked",
      label: "WebGPU blocked",
      detail: "navigator.gpu exists, but requestAdapter() returned null.",
    };
  }

  return {
    state: "ready",
    label: "WebGPU ready",
    detail: `Adapter: ${adapterName || "unknown GPU adapter"}`,
  };
}

export async function detectWebGpu(): Promise<WebGpuStatus> {
  if (!("gpu" in navigator) || !navigator.gpu) {
    return describeWebGpuStatus(false, undefined);
  }

  const adapter = await navigator.gpu.requestAdapter({
    powerPreference: "high-performance",
  });

  if (!adapter) {
    return describeWebGpuStatus(true, null);
  }

  const adapterWithInfo = adapter as GPUAdapter & {
    info?: GPUAdapterInfo;
    requestAdapterInfo?: () => Promise<GPUAdapterInfo>;
  };

  const info =
    adapterWithInfo.info ??
    (adapterWithInfo.requestAdapterInfo
      ? await adapterWithInfo.requestAdapterInfo()
      : undefined);

  return describeWebGpuStatus(true, info?.device || info?.description || info?.vendor);
}
