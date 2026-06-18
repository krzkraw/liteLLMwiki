import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

describe("native runner paths", () => {
  it("places sidecar release artifacts under the demo web UI tree", async () => {
    const { resolveNativeRunnerRoot, resolveRepoRoot, resolveSidecarSourceRoot } =
      await import("./nativeRunnerPaths.mjs");

    expect(resolveNativeRunnerRoot("/repo/demo")).toBe(
      resolve("/repo/demo/native/sidecar"),
    );
    expect(resolveRepoRoot("/repo/demo")).toBe(resolve("/repo"));
    expect(resolveSidecarSourceRoot("/repo")).toBe(
      resolve("/repo/native/sidecar"),
    );
  });

  it("builds sidecar release commands for macOS/Linux and Windows", async () => {
    const { createNativeRunnerBuildCommand } = await import(
      "./build-native-runner.mjs"
    );
    const outDir = resolve("/repo/demo/native/sidecar");
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
