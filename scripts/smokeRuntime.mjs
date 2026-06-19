import { existsSync } from "fs";
import { mkdtemp, rm } from "fs/promises";
import { tmpdir } from "os";
import { join } from "path";

export async function createSmokeWorkspace(prefix) {
  const root = await mkdtemp(join(tmpdir(), prefix));

  return {
    root,
    path(name) {
      return join(root, name);
    },
    async cleanup() {
      await rm(root, { recursive: true, force: true });
    },
  };
}

export function createChromiumGpuArgs(platform = process.platform) {
  const args = ["--enable-unsafe-webgpu", "--enable-features=WebGPU"];

  if (platform === "darwin") {
    args.push("--use-angle=metal");
  } else if (platform === "win32") {
    args.push("--use-angle=d3d11");
  } else if (platform === "linux") {
    args.push("--use-angle=vulkan");
  }

  return args;
}

export function createSmokeChromiumLaunchOptions(browserType, launchOptions = {}) {
  if (launchOptions.channel || launchOptions.executablePath) {
    return { ...launchOptions };
  }

  const executablePath = browserType.executablePath();
  if (!existsSync(executablePath)) {
    throw new Error(
      [
        `Playwright Chromium executable is missing at ${executablePath}.`,
        "Run: bunx playwright install chromium",
      ].join("\n"),
    );
  }

  return { ...launchOptions, executablePath };
}

export async function launchSmokeChromium(browserType, launchOptions = {}) {
  try {
    return await browserType.launch(
      createSmokeChromiumLaunchOptions(browserType, launchOptions),
    );
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (
      message.includes("Executable doesn't exist") ||
      message.includes("chromium_headless_shell")
    ) {
      throw new Error(
        [
          "Playwright Chromium is not ready for smoke tests.",
          "Run: bunx playwright install chromium",
          message,
        ].join("\n"),
      );
    }

    throw error;
  }
}
