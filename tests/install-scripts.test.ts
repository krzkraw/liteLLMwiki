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

  it("uses confirm-or-wait gates for installs and downloads", () => {
    for (const scriptName of ["install.sh", "install.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("Do you want me to do it");
      expect(contents).toContain("I will wait");
      expect(contents).toContain("Press Enter");
      expect(contents).toContain("failed");
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
      expect(contents).toContain("litert-lm");
      expect(contents).toContain("llama-server");
      expect(contents).toContain("uv tool install litert-lm");
    }
  });

  it("checks known model paths and provides Hugging Face URLs", () => {
    for (const scriptName of ["install.sh", "install.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("models/gemma-4-E2B-it-web.litertlm");
      expect(contents).toContain("models/gemma-4-E2B-it.litertlm");
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

  it("installs npm dependencies, builds artifacts, runs smoke checks, and prints a summary", () => {
    for (const scriptName of ["install.sh", "install.ps1"]) {
      const contents = readRootScript(scriptName);

      expect(contents).toContain("npm install");
      expect(contents).toContain("npm run build");
      expect(contents).toContain("npm run build:sidecar");
      expect(contents).toContain("npm run smoke");
      expect(contents).toContain("npm run smoke:executable");
      expect(contents).toContain("Summary");
      expect(contents).toContain("launch-all");
    }
  });
});
