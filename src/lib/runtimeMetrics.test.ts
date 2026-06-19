import { describe, expect, it } from "bun:test";
import {
  createInitialRuntimeMetrics,
  estimateTokenCount,
  resetRuntimeMetrics,
  updateRuntimeMetrics,
} from "./runtimeMetrics";

describe("runtimeMetrics", () => {
  it("starts empty and reports null throughput before tokens arrive", () => {
    expect(createInitialRuntimeMetrics()).toEqual({
      startedAtMs: null,
      updatedAtMs: null,
      tokenCount: 0,
      tokensPerSecond: null,
    });
  });

  it("updates token count and throughput from streamed chunks", () => {
    let metrics = createInitialRuntimeMetrics();

    metrics = updateRuntimeMetrics(metrics, "Hello local runtime", 1000);
    metrics = updateRuntimeMetrics(metrics, " again", 2000);

    expect(metrics.tokenCount).toBe(4);
    expect(metrics.startedAtMs).toBe(1000);
    expect(metrics.updatedAtMs).toBe(2000);
    expect(metrics.tokensPerSecond).toBe(4);
  });

  it("counts punctuation groups as token-like chunks", () => {
    expect(estimateTokenCount("Hello, world!")).toBe(4);
  });

  it("resets metrics between generations", () => {
    const metrics = updateRuntimeMetrics(
      createInitialRuntimeMetrics(),
      "token",
      1000,
    );

    expect(resetRuntimeMetrics(metrics)).toEqual(createInitialRuntimeMetrics());
  });
});
