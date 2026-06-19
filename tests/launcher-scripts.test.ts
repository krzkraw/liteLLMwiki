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
      expect(contents).toContain("--litert-launch-inline");
      expect(contents).toContain("osascript");
      expect(contents).toContain("gnome-terminal");
      expect(contents).toContain("xterm");
    }

    for (const scriptName of ["launch-webui.ps1", "launch-sidecar.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("Start-LiteRTTerminal");
      expect(contents).toContain("osascript");
      expect(contents).toContain("gnome-terminal");
      expect(contents).toContain("xterm");
      expect(contents).toContain("[switch]$Inline");
      expect(contents).toContain("Start-Process");
      expect(contents).toContain("-NoExit");
      expect(contents).toContain("-Inline");
    }
  });

  it("does not let inherited inline environment state bypass terminal launch", () => {
    for (const scriptName of [
      "launch-webui.sh",
      "launch-sidecar.sh",
      "launch-all.sh",
    ]) {
      const contents = readRootScript(scriptName);

      expect(contents).not.toContain("LITERT_LAUNCH_INLINE");
    }

    for (const scriptName of [
      "launch-webui.ps1",
      "launch-sidecar.ps1",
      "launch-all.ps1",
    ]) {
      const contents = readRootScript(scriptName);

      expect(contents).not.toContain("$env:LITERT_LAUNCH_INLINE");
    }
  });

  it("launches web UI and sidecar TUI in separate terminals", () => {
    const shellLauncher = readRootScript("launch-all.sh");
    expect(shellLauncher).toContain("launch_terminal");
    expect(shellLauncher).toContain("launch-webui");
    expect(shellLauncher).toContain("launch-sidecar");
    expect(shellLauncher).toContain("--litert-launch-inline");
    expect(shellLauncher).toContain("LITERT_SIDECAR_TUI=1");
    expect(shellLauncher).not.toContain("SIDECAR_HEADLESS");
    expect(shellLauncher).not.toContain("cleanup");

    const powershellLauncher = readRootScript("launch-all.ps1");
    expect(powershellLauncher).toContain("Start-LiteRTTerminal");
    expect(powershellLauncher).toContain("launch-webui");
    expect(powershellLauncher).toContain("launch-sidecar");
    expect(powershellLauncher).toContain('"-Inline"');
    expect(powershellLauncher).toContain("-Tui");
    expect(powershellLauncher).not.toContain("SIDECAR_HEADLESS");
    expect(powershellLauncher).not.toContain("-Headless");
    expect(powershellLauncher).not.toContain("cleanup");
  });

  it("launch-all opens the web UI first and the sidecar TUI last", () => {
    const shellLauncher = readRootScript("launch-all.sh");
    expect(shellLauncher.indexOf("launch-webui.sh")).toBeLessThan(
      shellLauncher.indexOf("launch-sidecar.sh"),
    );

    const powershellLauncher = readRootScript("launch-all.ps1");
    expect(powershellLauncher.indexOf("launch-webui.ps1")).toBeLessThan(
      powershellLauncher.indexOf("launch-sidecar.ps1"),
    );
  });

  it("prefers separate terminal windows over tab handoff", () => {
    for (const scriptName of ["launch-webui.sh", "launch-sidecar.sh"]) {
      const contents = readRootScript(scriptName);

      expect(contents).not.toContain("--new-tab");
    }

    for (const scriptName of ["launch-webui.ps1", "launch-sidecar.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("wt.exe");
      expect(contents).toContain("new-window");
      expect(contents).not.toContain("--new-tab");
    }
  });

  it("documents that launch-all preserves the sidecar TUI instead of headless mode", () => {
    for (const scriptName of ["launch-all.sh", "launch-all.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("Sidecar TUI");
      expect(contents).not.toContain("SIDECAR_HEADLESS");
      expect(contents).not.toContain("-Headless");
    }
  });

  it("forces terminal-launched sidecars to stay interactive unless headless is explicit", () => {
    const shellLauncher = readRootScript("launch-sidecar.sh");
    expect(shellLauncher).toContain("LITERT_SIDECAR_TUI=1");
    expect(shellLauncher).toContain('${LITERT_SIDECAR_TUI:-}');

    const powershellLauncher = readRootScript("launch-sidecar.ps1");
    expect(powershellLauncher).toContain("[switch]$Tui");
    expect(powershellLauncher).toContain('"-Tui"');
    expect(powershellLauncher).toContain("-not $Tui");
  });
});
