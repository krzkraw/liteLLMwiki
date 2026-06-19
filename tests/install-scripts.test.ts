import { accessSync, constants, readFileSync } from "fs";
import { join } from "path";
import { describe, expect, it } from "bun:test";

const repoRoot = process.cwd();
const oldPackageRunner = "n" + "pm";

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
      expect(contents).toContain("bun");
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

      expect(contents).toContain("bun");
      expect(contents).not.toContain(`${oldPackageRunner} install`);
      expect(contents).toContain("go");
      expect(contents).toContain("git");
      expect(contents).toContain("curl");
      expect(contents).toContain("Playwright Chromium");
      expect(contents).toContain("litert-lm");
      expect(contents).toContain("llama-server");
      expect(contents).toContain("bunx playwright install chromium");
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

      expect(contents).toContain("models/litert/browser/gemma-4-E2B-it-web.litertlm");
      expect(contents).toContain("models/litert/main/gemma-4-E2B-it.litertlm");
      expect(contents).toContain(
        "models/llamacpp/main/gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf",
      );
      expect(contents).toContain(
        "models/llamacpp/embedding/Qwen3-Embedding-0.6B-q8_0.gguf",
      );
      expect(contents).toContain(
        "models/llamacpp/embedding/Qwen3-Embedding-0.6B-iq4_nl.gguf",
      );
      expect(contents).toContain(
        "models/llamacpp/reranking/Qwen3-Reranker-0.6B-Q4_K_M.gguf",
      );
      expect(contents).toContain("models/llamacpp/main/Qwen3.5-2B-IQ4_NL.gguf");
      expect(contents).toContain(
        "models/llamacpp/main/Qwen3.5-0.8B-UD-Q8_K_XL.gguf",
      );
      expect(contents).toContain("Voodisss/Qwen3-Reranker-0.6B-GGUF-llama_cpp");
      expect(contents).toContain("Mungert/Qwen3-Embedding-0.6B-GGUF");
      expect(contents).toContain("unsloth/Qwen3.5-2B-GGUF");
      expect(contents).toContain("unsloth/Qwen3.5-0.8B-GGUF");
      expect(contents).toContain("huggingface.co");
      expect(contents).toContain("HF_TOKEN");
    }
  });

  it("uses checkbox model downloads with requested defaults", () => {
    for (const scriptName of ["install.sh", "install.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("Select models to download");
      expect(contents).toContain("Default selected");
      expect(contents).toContain("gemma4-litert");
      expect(contents).toContain("gemma4-web-litert");
      expect(contents).toContain("embeddinggemma-litert");
      expect(contents).toContain("qwen3-reranker-q4km");
      expect(contents).toContain("[ ]");
      expect(contents).toContain("[x]");
      expect(contents).toContain("Toggle numbers");
      expect(contents).toContain("c: continue");
      expect(contents).not.toContain("Qwen3-Embedding-0.6B-Q8_0.gguf");
      expect(contents).not.toContain("Qwen/Qwen3-Embedding-0.6B-GGUF");
    }
  });

  it("runs selected checkbox downloads without per-item confirmation prompts", () => {
    const shell = readRootScript("install.sh");
    const powershell = readRootScript("install.ps1");

    expect(shell).toContain("Selected from checkbox; downloading now.");
    expect(shell).toContain("Selected runtimes will be downloaded now.");
    expect(shell).not.toContain('prompt_task_choice "llama.cpp runtime"');
    expect(shell).not.toContain(
      '"Download the model file or place it manually in the expected local path."',
    );

    expect(powershell).toContain("Selected from checkbox; downloading now.");
    expect(powershell).toContain("Selected runtimes will be downloaded now.");
    expect(powershell).not.toContain("$RuntimeChoice = Read-TaskChoice");
    expect(powershell).not.toContain(
      '-Description "Download the model file or place it manually in the expected local path."',
    );
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

  it("installs Bun dependencies, builds artifacts, runs smoke checks, and prints a summary", () => {
    for (const scriptName of ["install.sh", "install.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("bun install");
      expect(contents).toContain("bunx playwright install chromium");
      expect(contents).toContain("bun run build");
      expect(contents).toContain("bun run build:sidecar");
      expect(contents).toContain("bun run smoke");
      expect(contents).toContain("bun run smoke:executable");
      expect(contents).not.toContain(`${oldPackageRunner} run`);
      expect(contents).toContain("Summary");
      expect(contents).toContain("launch-all");
    }
  });

  it("keeps browser smoke launch failures recoverable during install", () => {
    const shell = readRootScript("install.sh");
    const powershell = readRootScript("install.ps1");

    expect(shell).toContain("run_smoke_or_wait");
    expect(shell).toContain("Smoke browser automation failed.");
    expect(shell).toContain('add_summary "SKIP: $label"');
    expect(shell).toContain('run_smoke_or_wait "smoke UI"');
    expect(shell).toContain('expected_result="smoke command completes successfully"');
    expect(shell).toContain("[N] No - continue without this smoke check");

    expect(powershell).toContain("Invoke-SmokeOrWait");
    expect(powershell).toContain("Smoke browser automation failed.");
    expect(powershell).toContain('Add-Summary "SKIP: $Label"');
    expect(powershell).toContain('Invoke-SmokeOrWait -Label "smoke UI"');
    expect(powershell).toContain('$ExpectedResult = "smoke command completes successfully"');
    expect(powershell).toContain("[N] No - continue without this smoke check");
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

  it("uses a Windows executable Bun shim for PowerShell Start-Process", () => {
    const contents = readRootScript("install.ps1");

    expect(contents).toContain("Get-BunStartProcessSpec");
    expect(contents).toContain("bun.exe");
    expect(contents).toContain("$BunSpec = Get-BunStartProcessSpec");
    expect(contents).toContain("Start-Process -FilePath $BunSpec.FilePath");
    expect(contents).toContain("-ArgumentList $BunArguments");
    expect(contents).not.toContain('Start-Process -FilePath "bun"');
  });

  it("isolates the PowerShell smoke dev server from the interactive console", () => {
    const contents = readRootScript("install.ps1");

    expect(contents).toContain("Stop-ProcessTree");
    expect(contents).toContain("taskkill.exe");
    expect(contents).toContain("/T");
    expect(contents).toContain("/F");
    expect(contents).toContain("$DevServerStdinPath");
    expect(contents).toContain("$StdinPath");
    expect(contents).toContain("-RedirectStandardInput $StdinPath");
    expect(contents).toContain("$script:DevServerProcess = $null");
    expect(contents).not.toContain("-NoNewWindow");
  });
});
