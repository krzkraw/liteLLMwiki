import { describe, expect, it } from "bun:test";
import { crossOriginIsolationHeaders } from "./securityHeaders";

describe("crossOriginIsolationHeaders", () => {
  it("enables cross-origin isolation for local WebGPU and WASM model execution", () => {
    expect(crossOriginIsolationHeaders).toEqual({
      "Cross-Origin-Opener-Policy": "same-origin",
      "Cross-Origin-Embedder-Policy": "require-corp",
    });
  });
});
