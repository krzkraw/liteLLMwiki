import { resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { describe, expect, it } from "vitest";

describe("native runner paths", () => {
  it("places sidecar release artifacts outside the sidecar source tree", async () => {
    const {
      resolveAppRoot,
      resolveNativeRunnerRoot,
      resolveRepoRoot,
      resolveSidecarSourceRoot,
    } = await import("./nativeRunnerPaths.mjs");

    const appRoot = resolve(process.cwd());
    const scriptPath = resolve(appRoot, "scripts/nativeRunnerPaths.mjs");
    expect(resolveAppRoot(pathToFileURL(scriptPath).href)).toBe(appRoot);
    expect(resolveNativeRunnerRoot(appRoot)).toBe(
      resolve(appRoot, "native/sidecar-artifacts"),
    );
    expect(resolveRepoRoot(appRoot)).toBe(appRoot);
    expect(resolveSidecarSourceRoot(appRoot)).toBe(
      resolve(appRoot, "native/sidecar"),
    );
  });

  it("builds sidecar release commands for macOS/Linux and Windows", async () => {
    const { createNativeRunnerBuildCommand } = await import(
      "./build-native-runner.mjs"
    );
    const outDir = resolve("/repo/native/sidecar-artifacts");
    const sidecarRoot = resolve("/repo/native/sidecar");

    expect(
      createNativeRunnerBuildCommand({
        platform: "darwin",
        sidecarRoot,
        outDir,
      }),
    ).toEqual({
      command: "bash",
      args: [resolve(sidecarRoot, "scripts/build-release.sh"), outDir],
    });

    expect(
      createNativeRunnerBuildCommand({
        platform: "win32",
        sidecarRoot,
        outDir,
        powershell: "pwsh",
      }),
    ).toEqual({
      command: "pwsh",
      args: [
        "-NoProfile",
        "-ExecutionPolicy",
        "Bypass",
        "-File",
        resolve(sidecarRoot, "scripts/build-release.ps1"),
        "-OutDir",
        outDir,
      ],
    });
  });
});
