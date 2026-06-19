import { existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, isAbsolute, join, sep } from "node:path";
import { describe, expect, it } from "vitest";

describe("smoke runtime helpers", () => {
  it("creates disposable smoke workspaces under the platform temp directory", async () => {
    const { createSmokeWorkspace } = await import("./smokeRuntime.mjs");
    const workspace = await createSmokeWorkspace("litert-test-");

    try {
      expect(isAbsolute(workspace.root)).toBe(true);
      expect(workspace.root.startsWith(tmpdir() + sep)).toBe(true);
      expect(workspace.path("image.png")).toBe(join(workspace.root, "image.png"));
      expect(dirname(workspace.path("image.png"))).toBe(workspace.root);
      expect(existsSync(workspace.root)).toBe(true);
    } finally {
      await workspace.cleanup();
    }

    expect(existsSync(workspace.root)).toBe(false);
  });

  it("uses platform-aware Chromium GPU launch flags", async () => {
    const { createChromiumGpuArgs } = await import("./smokeRuntime.mjs");

    expect(createChromiumGpuArgs("darwin")).toEqual([
      "--enable-unsafe-webgpu",
      "--enable-features=WebGPU",
      "--use-angle=metal",
    ]);
    expect(createChromiumGpuArgs("win32")).toEqual([
      "--enable-unsafe-webgpu",
      "--enable-features=WebGPU",
      "--use-angle=d3d11",
    ]);
    expect(createChromiumGpuArgs("linux")).toEqual([
      "--enable-unsafe-webgpu",
      "--enable-features=WebGPU",
      "--use-angle=vulkan",
    ]);
  });
});
