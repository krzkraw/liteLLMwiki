import { describe, expect, it } from "vitest";
import { describeWebGpuStatus } from "./webgpu";

describe("describeWebGpuStatus", () => {
  it("reports missing browser WebGPU API", () => {
    expect(describeWebGpuStatus(false, undefined)).toEqual({
      state: "missing",
      label: "WebGPU unavailable",
      detail: "This browser does not expose navigator.gpu.",
    });
  });

  it("reports adapter availability", () => {
    expect(describeWebGpuStatus(true, "Apple GPU")).toEqual({
      state: "ready",
      label: "WebGPU ready",
      detail: "Adapter: Apple GPU",
    });
  });

  it("reports browsers that expose WebGPU but cannot allocate an adapter", () => {
    expect(describeWebGpuStatus(true, null)).toEqual({
      state: "blocked",
      label: "WebGPU blocked",
      detail: "navigator.gpu exists, but requestAdapter() returned null.",
    });
  });
});
