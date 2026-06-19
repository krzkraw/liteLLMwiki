import { accessSync, constants, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const repoRoot = process.cwd();

function readRootScript(name: string): string {
  return readFileSync(join(repoRoot, name), "utf8");
}

describe("root launcher scripts", () => {
  it("provides Unix and PowerShell entry points for web UI, sidecar, and both", () => {
    const scriptNames = [
      "launch-webui.sh",
      "launch-sidecar.sh",
      "launch-all.sh",
      "launch-webui.ps1",
      "launch-sidecar.ps1",
      "launch-all.ps1",
    ];

    for (const scriptName of scriptNames) {
      expect(readRootScript(scriptName).trim()).not.toHaveLength(0);
    }
  });

  it("keeps Unix launchers executable and rooted at the repository directory", () => {
    for (const scriptName of [
      "launch-webui.sh",
      "launch-sidecar.sh",
      "launch-all.sh",
    ]) {
      const scriptPath = join(repoRoot, scriptName);
      const contents = readRootScript(scriptName);

      accessSync(scriptPath, constants.X_OK);
      expect(contents).toContain("#!/usr/bin/env bash");
      expect(contents).toContain("set -euo pipefail");
      expect(contents).toContain('dirname "${BASH_SOURCE[0]}"');
    }
  });

  it("launches the web UI through the existing npm dev command", () => {
    for (const scriptName of ["launch-webui.sh", "launch-webui.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("WEBUI_HOST");
      expect(contents).toContain("WEBUI_PORT");
      expect(contents).toContain("npm");
      expect(contents).toContain("run");
      expect(contents).toContain("dev");
      expect(contents).toContain("--host");
      expect(contents).toContain("--port");
    }
  });

  it("launches the sidecar from artifacts or Go source with supported overrides", () => {
    for (const scriptName of ["launch-sidecar.sh", "launch-sidecar.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("SIDECAR_BIN");
      expect(contents).toContain("LITERT_LM_BIN");
      expect(contents).toContain("MODEL_FILE");
      expect(contents).toContain("MODEL_ID");
      expect(contents).toContain("litert-sidecar");
      expect(contents).toContain("go run");
      expect(contents).toContain("-runtime-exe");
      expect(contents).toContain("-model-file");
      expect(contents).toContain("-model-id");
      expect(contents).toContain("--headless");
    }
  });

  it("makes installed llama.cpp runtimes discoverable by the sidecar", () => {
    for (const scriptName of ["launch-sidecar.sh", "launch-sidecar.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toMatch(/native[\\/]+llama-runtimes/);
      expect(contents).toContain("LLAMA_RUNTIME");
      expect(contents).toContain("LLAMA_SERVER_BIN");
      expect(contents).toContain("llama-server");
    }
  });

  it("opens individual launchers in separate terminal windows", () => {
    for (const scriptName of ["launch-webui.sh", "launch-sidecar.sh"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("launch_terminal");
      expect(contents).toContain("LITERT_LAUNCH_INLINE");
      expect(contents).toContain("osascript");
      expect(contents).toContain("gnome-terminal");
      expect(contents).toContain("xterm");
    }

    for (const scriptName of ["launch-webui.ps1", "launch-sidecar.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("[switch]$Inline");
      expect(contents).toContain("Start-Process");
      expect(contents).toContain("-NoExit");
      expect(contents).toContain("-Inline");
    }
  });

  it("launches web UI and sidecar TUI in separate terminals", () => {
    for (const scriptName of ["launch-all.sh", "launch-all.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("launch-webui");
      expect(contents).toContain("launch-sidecar");
      expect(contents).not.toContain("SIDECAR_HEADLESS");
      expect(contents).not.toContain("-Headless");
      expect(contents).not.toContain("cleanup");
    }
  });
});
