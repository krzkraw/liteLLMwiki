import { spawn } from "node:child_process";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import {
  resolveDemoRoot,
  resolveNativeRunnerRoot,
  resolveRepoRoot,
  resolveSidecarSourceRoot,
} from "./nativeRunnerPaths.mjs";

export function createNativeRunnerBuildCommand({
  platform = process.platform,
  sidecarRoot = resolveSidecarSourceRoot(resolveRepoRoot(resolveDemoRoot())),
  outDir = resolveNativeRunnerRoot(resolveDemoRoot()),
  powershell = process.env.PWSH || "powershell",
} = {}) {
  if (platform === "win32") {
    return {
      command: powershell,
      args: [
        "-NoProfile",
        "-ExecutionPolicy",
        "Bypass",
        "-File",
        join(sidecarRoot, "scripts", "build-release.ps1"),
        "-OutDir",
        outDir,
      ],
    };
  }

  return {
    command: "bash",
    args: [join(sidecarRoot, "scripts", "build-release.sh"), outDir],
  };
}

export async function buildNativeRunner({
  outDir = process.argv[2]
    ? resolve(process.cwd(), process.argv[2])
    : resolveNativeRunnerRoot(resolveDemoRoot()),
} = {}) {
  const command = createNativeRunnerBuildCommand({ outDir });
  const child = spawn(command.command, command.args, {
    stdio: "inherit",
  });

  const code = await new Promise((resolveCode, reject) => {
    child.once("error", reject);
    child.once("exit", (exitCode) => resolveCode(exitCode ?? 1));
  });

  if (code !== 0) {
    throw new Error(`Native runner build failed with exit code ${code}.`);
  }
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  buildNativeRunner().catch((error) => {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  });
}
