import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

export function resolveDemoRoot(scriptUrl = import.meta.url) {
  return resolve(dirname(fileURLToPath(scriptUrl)), "..");
}

export function resolveRepoRoot(demoRoot = resolveDemoRoot()) {
  return resolve(demoRoot, "..");
}

export function resolveNativeRunnerRoot(demoRoot = resolveDemoRoot()) {
  return resolve(demoRoot, "native", "sidecar");
}

export function resolveSidecarSourceRoot(repoRoot = resolveRepoRoot()) {
  return resolve(repoRoot, "native", "sidecar");
}
