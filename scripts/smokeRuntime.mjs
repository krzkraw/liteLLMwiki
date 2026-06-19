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
    return { timeout: 30_000, ...launchOptions };
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

  return { timeout: 30_000, ...launchOptions, executablePath };
}

function isChromiumInstallError(error) {
  const message = error instanceof Error ? error.message : String(error);

  return (
    message.includes("Executable doesn't exist") ||
    message.includes("chromium_headless_shell")
  );
}

function createSmokeChromiumLaunchPlan(browserType, launchOptions = {}, options = {}) {
  const platform = options.platform ?? process.platform;
  const firstOptions = createSmokeChromiumLaunchOptions(browserType, launchOptions);
  const plan = [{ label: "Playwright Chromium", options: firstOptions }];

  if (
    platform === "win32" &&
    !launchOptions.channel &&
    !launchOptions.executablePath
  ) {
    plan.push(
      {
        label: "Google Chrome channel",
        options: { timeout: 30_000, ...launchOptions, channel: "chrome" },
      },
      {
        label: "Microsoft Edge channel",
        options: { timeout: 30_000, ...launchOptions, channel: "msedge" },
      },
    );
  }

  return plan;
}

function createLaunchFailureError(attempts) {
  return new Error(
    [
      "Smoke Chromium could not be launched.",
      "Tried:",
      ...attempts.map(
        ({ label, error }) =>
          `- ${label}: ${error instanceof Error ? error.message : String(error)}`,
      ),
      "Run: bunx playwright install chromium",
      "If bundled Chromium hangs on Windows, install Chrome or Edge and rerun.",
    ].join("\n"),
  );
}

export async function launchSmokeChromium(
  browserType,
  launchOptions = {},
  options = {},
) {
  const failures = [];

  for (const attempt of createSmokeChromiumLaunchPlan(
    browserType,
    launchOptions,
    options,
  )) {
    try {
      return await browserType.launch(attempt.options);
    } catch (error) {
      if (isChromiumInstallError(error)) {
        throw new Error(
          [
            "Playwright Chromium is not ready for smoke tests.",
            "Run: bunx playwright install chromium",
            error instanceof Error ? error.message : String(error),
          ].join("\n"),
        );
      }

      failures.push({ ...attempt, error });
    }
  }

  if (failures.length > 1) {
    throw createLaunchFailureError(failures);
  }

  const [failure] = failures;
  if (failure) {
    throw failure.error;
  }
  throw createLaunchFailureError(failures);
}
