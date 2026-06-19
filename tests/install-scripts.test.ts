import { accessSync, constants, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const repoRoot = process.cwd();

function readRootScript(name: string): string {
  return readFileSync(join(repoRoot, name), "utf8");
}

describe("interactive installer scripts", () => {
  it("provides Unix and PowerShell installer entry points", () => {
    expect(readRootScript("install.sh")).toContain("#!/usr/bin/env bash");
    expect(readRootScript("install.ps1")).toContain("Set-StrictMode");
    accessSync(join(repoRoot, "install.sh"), constants.X_OK);
  });

  it("does not assign PowerShell automatic OS variables", () => {
    const contents = readRootScript("install.ps1");

    expect(contents).not.toMatch(/\$(IsMacOS|IsWindows|IsLinux)\s*=/i);
  });

  it("uses confirm-or-wait gates for installs and downloads", () => {
    for (const scriptName of ["install.sh", "install.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("Do you want me to do it");
      expect(contents).toContain("I will wait");
      expect(contents).toContain("Press Enter");
      expect(contents).toContain("failed");
    }
  });

  it("prints an up-front installer task checklist with green checkmarks", () => {
    for (const scriptName of ["install.sh", "install.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("Install tasks");
      expect(contents).toContain("✓");
      expect(contents).toContain("node");
      expect(contents).toContain("Gemma 4 E2B web model");
      expect(contents).toContain("smoke executable sidecar");
      expect(contents).toContain("Green");
    }
  });

  it("wraps current install actions in a boxed choice prompt", () => {
    for (const scriptName of ["install.sh", "install.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("+");
      expect(contents).toContain("| Task:");
      expect(contents).toContain("| Description:");
      expect(contents).toContain("| Choices:");
      expect(contents).toContain("[Y] Yes");
      expect(contents).toContain("[N] No");
      expect(contents).toContain("[M] Manual & wait");
    }
  });

  it("manual wait prompts describe the expected result", () => {
    for (const scriptName of ["install.sh", "install.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("Expected result:");
      expect(contents).toContain("Press Enter after the expected result is true");
    }
  });

  it("checks dependencies and prints package-manager commands before running them", () => {
    for (const scriptName of ["install.sh", "install.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("node");
      expect(contents).toContain("npm");
      expect(contents).toContain("go");
      expect(contents).toContain("git");
      expect(contents).toContain("curl");
      expect(contents).toContain("Playwright Chromium");
      expect(contents).toContain("litert-lm");
      expect(contents).toContain("llama-server");
      expect(contents).toContain("npx playwright install chromium");
      expect(contents).toContain("uv tool install litert-lm");
    }
  });

  it("offers selectable llama.cpp runtime downloads with local runtime folders", () => {
    for (const scriptName of ["install.sh", "install.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("native/llama-runtimes");
      expect(contents).toContain("llama-win-cpu-x64");
      expect(contents).toContain("llama-win-cuda-13.3-x64");
      expect(contents).toContain("cudart-llama-bin-win-cuda-13.3-x64.zip");
      expect(contents).toContain("llama-b9724-bin-macos-arm64.tar.gz");
      expect(contents).toContain("sha256:");
    }
  });

  it("uses a checkbox-style llama.cpp runtime selector", () => {
    for (const scriptName of ["install.sh", "install.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("[ ]");
      expect(contents).toContain("[x]");
      expect(contents).toContain("Toggle numbers");
      expect(contents).toContain("a: toggle all");
      expect(contents).toContain("c: continue");
      expect(contents).toContain("s: skip");
      expect(contents).not.toContain("llama.cpp runtime choice [all/");
    }
  });

  it("checks known model paths and provides Hugging Face URLs", () => {
    for (const scriptName of ["install.sh", "install.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("models/litert/gemma-4-E2B-it-web.litertlm");
      expect(contents).toContain("models/litert/gemma-4-E2B-it.litertlm");
      expect(contents).toContain(
        "models/llamacpp/gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf",
      );
      expect(contents).toContain(
        "models/llamacpp/Qwen3-Embedding-0.6B-Q8_0.gguf",
      );
      expect(contents).toContain("huggingface.co");
      expect(contents).toContain("HF_TOKEN");
    }
  });

  it("can override model downloads with a Nextcloud public share parameter", () => {
    for (const scriptName of ["install.sh", "install.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("modelsNextcloud");
      expect(contents).toContain("public.php/webdav");
      expect(contents).toContain("Authorization");
      expect(contents).toContain("nextcloud.example");
    }
  });

  it("maps repository model paths to Models-folder Nextcloud share paths", () => {
    expect(readRootScript("install.sh")).toContain("${relative_path#models/}");
    expect(readRootScript("install.ps1")).toContain(
      "-replace \"^models[\\\\/]\", \"\"",
    );
  });

  it("installs npm dependencies, builds artifacts, runs smoke checks, and prints a summary", () => {
    for (const scriptName of ["install.sh", "install.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("npm install");
      expect(contents).toContain("npx playwright install chromium");
      expect(contents).toContain("npm run build");
      expect(contents).toContain("npm run build:sidecar");
      expect(contents).toContain("npm run smoke");
      expect(contents).toContain("npm run smoke:executable");
      expect(contents).toContain("Summary");
      expect(contents).toContain("launch-all");
    }
  });

  it("uses separate PowerShell smoke server stdout and stderr log files", () => {
    const contents = readRootScript("install.ps1");

    expect(contents).toContain("$StdoutLogPath");
    expect(contents).toContain("$StderrLogPath");
    expect(contents).toContain("-RedirectStandardOutput $StdoutLogPath");
    expect(contents).toContain("-RedirectStandardError $StderrLogPath");
    expect(contents).not.toContain(
      "-RedirectStandardOutput $LogPath -RedirectStandardError $LogPath",
    );
  });

  it("uses a Windows executable npm shim for PowerShell Start-Process", () => {
    const contents = readRootScript("install.ps1");

    expect(contents).toContain("Get-NpmStartProcessSpec");
    expect(contents).toContain("npm.cmd");
    expect(contents).toContain("npm.exe");
    expect(contents).toContain("$NpmSpec = Get-NpmStartProcessSpec");
    expect(contents).toContain("Start-Process -FilePath $NpmSpec.FilePath");
    expect(contents).toContain("-ArgumentList $NpmArguments");
    expect(contents).not.toContain('Start-Process -FilePath "npm"');
  });
});
