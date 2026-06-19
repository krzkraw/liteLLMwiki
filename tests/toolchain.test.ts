import { existsSync, readFileSync } from "fs";
import { join } from "path";
import { describe, expect, it } from "bun:test";

const repoRoot = process.cwd();
const oldPackageRunner = "n" + "pm";
const oldBundler = "v" + "ite";
const oldTestRunner = "v" + "itest";
const oldLockfile = ["package", "lock"].join("-") + ".json";
const forbiddenPackageRunnerCommand =
  /\b(?:npm|npx|pnpm|yarn)\s+(?:install|run|test|exec|x|dlx)\b/i;

function readRootFile(name: string): string {
  return readFileSync(join(repoRoot, name), "utf8");
}

describe("Bun and Rspack toolchain contract", () => {
  it("uses Bun scripts and Rspack without old package, bundler, or test dependencies", () => {
    const packageJson = JSON.parse(readRootFile("package.json")) as {
      scripts: Record<string, string>;
      dependencies?: Record<string, string>;
      devDependencies?: Record<string, string>;
      packageManager?: string;
    };
    const dependencies = {
      ...packageJson.dependencies,
      ...packageJson.devDependencies,
    };

    expect(packageJson.packageManager?.startsWith("bun@")).toBe(true);
    expect(packageJson.scripts.dev).toContain("rspack");
    expect(packageJson.scripts.dev).toContain("bun --bun");
    expect(packageJson.scripts.build).toContain("rspack");
    expect(packageJson.scripts.build).toContain("bun --bun tsc");
    expect(packageJson.scripts.build).toContain("bun --bun rspack");
    expect(packageJson.scripts.preview).toContain("bun");
    expect(packageJson.scripts.test).toBe("bun test");
    expect(Object.keys(dependencies)).toContain("@rspack/core");
    expect(Object.keys(dependencies)).toContain("@rspack/cli");
    expect(Object.keys(dependencies)).toContain("@rspack/dev-server");
    expect(Object.keys(dependencies)).not.toContain(oldBundler);
    expect(Object.keys(dependencies)).not.toContain(oldTestRunner);
    expect(Object.keys(dependencies)).not.toContain(`@${oldBundler}js/plugin-react`);
  });

  it("uses a Bun lockfile instead of the old package-manager lockfile", () => {
    expect(existsSync(join(repoRoot, "bun.lock"))).toBe(true);
    expect(existsSync(join(repoRoot, oldLockfile))).toBe(false);
  });

  it("documents Bun as the only JavaScript command runner", () => {
    expect(readRootFile("AGENTS.md")).toContain("Do not use npm");

    for (const fileName of [
      "AGENTS.md",
      "README.md",
      "install.sh",
      "install.ps1",
      "launch-webui.sh",
      "launch-webui.ps1",
      "launch-sidecar.sh",
      "launch-sidecar.ps1",
      "launch-all.sh",
      "launch-all.ps1",
      "package.json",
    ]) {
      expect(readRootFile(fileName)).not.toMatch(forbiddenPackageRunnerCommand);
    }
  });

  it("uses the platform WebSocket client for the mock sidecar smoke test", () => {
    const contents = readRootFile("scripts/mock-openai-compatible-server.test.ts");

    expect(contents).toContain("new WebSocket(");
    expect(contents).not.toContain("Bun.connect");
    expect(contents).not.toContain("encodeClientWebSocketTextFrame");
  });
});
