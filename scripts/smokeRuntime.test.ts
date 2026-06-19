import { existsSync } from "node:fs";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
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

  it("launches smoke Chromium through the regular Playwright executable", async () => {
    const { launchSmokeChromium } = await import("./smokeRuntime.mjs");
    const root = await mkdtemp(join(tmpdir(), "litert-chromium-test-"));
    const executablePath = join(root, "chrome");
    const launches: unknown[] = [];

    try {
      await writeFile(executablePath, "");
      const browser = await launchSmokeChromium(
        {
          executablePath: () => executablePath,
          launch: async (options: unknown) => {
            launches.push(options);
            return { close: async () => undefined };
          },
        },
        { headless: true, args: ["--smoke-flag"] },
      );

      await browser.close();
      expect(launches).toEqual([
        {
          executablePath,
          headless: true,
          args: ["--smoke-flag"],
        },
      ]);
    } finally {
      await rm(root, { recursive: true, force: true });
    }
  });

  it("reports the Playwright install command when Chromium is missing", async () => {
    const { launchSmokeChromium } = await import("./smokeRuntime.mjs");
    const missingPath = join(tmpdir(), "missing-playwright-chromium");

    await expect(
      launchSmokeChromium({
        executablePath: () => missingPath,
        launch: async () => {
          throw new Error("launch should not run");
        },
      }),
    ).rejects.toThrow(/npx playwright install chromium/);
  });
});
