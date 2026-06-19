import { dirname, resolve } from "path";
import { fileURLToPath } from "url";

export function resolveAppRoot(scriptUrl = import.meta.url) {
  return resolve(dirname(fileURLToPath(scriptUrl)), "..");
}

export function resolveRepoRoot(appRoot = resolveAppRoot()) {
  return appRoot;
}

export function resolveNativeRunnerRoot(repoRoot = resolveRepoRoot()) {
  return resolve(repoRoot, "native", "sidecar-artifacts");
}

export function resolveSidecarSourceRoot(repoRoot = resolveRepoRoot()) {
  return resolve(repoRoot, "native", "sidecar");
}
