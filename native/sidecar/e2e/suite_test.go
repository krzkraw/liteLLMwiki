package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanRuntimeBackendCombosReportsMissingConfigSkip(t *testing.T) {
	t.Parallel()

	plan, err := PlanRuntimeBackendCombos(SuiteOptions{
		RepoRoot:          t.TempDir(),
		BackendConfigPath: filepath.Join(t.TempDir(), "backends.json"),
	})
	if err != nil {
		t.Fatalf("plan runtime backend combos: %v", err)
	}
	if plan.ReadyCount() != 0 {
		t.Fatalf("ready combo count = %d, want 0", plan.ReadyCount())
	}
	if !strings.Contains(plan.SkipReason(), "backend config not found") {
		t.Fatalf("skip reason = %q, want missing backend config detail", plan.SkipReason())
	}
}

func TestPlanRuntimeBackendCombosEnumeratesWorkingCombosWithSkipReasons(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	configPath := filepath.Join(repoRoot, "native", "runtime-config", "backends.json")
	config := `{
  "version": 1,
  "runtimes": {
    "litert": {
      "cpu": {"working": true},
      "gpu": {"working": false}
    },
    "llamacpp": {
      "cuda13": {"working": true}
    }
  }
}`
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	plan, err := PlanRuntimeBackendCombos(SuiteOptions{
		RepoRoot:          repoRoot,
		BackendConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("plan runtime backend combos: %v", err)
	}

	if len(plan.Combos) != 2 {
		t.Fatalf("plan combos = %#v, want two working combos", plan.Combos)
	}
	if got := plan.Combos[0].Name(); got != "litert/cpu/main" {
		t.Fatalf("first combo name = %q", got)
	}
	if !strings.Contains(plan.Combos[0].SkipReason, "missing model") {
		t.Fatalf("litert skip reason = %q, want missing model detail", plan.Combos[0].SkipReason)
	}
	if got := plan.Combos[1].Name(); got != "llamacpp/cuda13/main" {
		t.Fatalf("second combo name = %q", got)
	}
	if !strings.Contains(plan.Combos[1].SkipReason, "no installed runtime variant") {
		t.Fatalf("llama skip reason = %q, want missing runtime variant detail", plan.Combos[1].SkipReason)
	}
	if plan.ReadyCount() != 0 {
		t.Fatalf("ready combo count = %d, want 0", plan.ReadyCount())
	}
}

func TestPlanRuntimeBackendCombosMarksReadyComboWhenPrerequisitesExist(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	configPath := filepath.Join(repoRoot, "native", "runtime-config", "backends.json")
	config := `{
  "version": 1,
  "runtimes": {
    "litert": {
      "cpu": {"working": true}
    }
  }
}`
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	modelPath := filepath.Join(repoRoot, "models", "litert", "main", "gemma-4-E2B-it.litertlm")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatalf("create model directory: %v", err)
	}
	if err := os.WriteFile(modelPath, []byte("fake model"), 0o644); err != nil {
		t.Fatalf("write model: %v", err)
	}
	executable := filepath.Join(t.TempDir(), "litert-lm")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}

	plan, err := PlanRuntimeBackendCombos(SuiteOptions{
		RepoRoot:          repoRoot,
		BackendConfigPath: configPath,
		LiteRTExecutable:  executable,
	})
	if err != nil {
		t.Fatalf("plan runtime backend combos: %v", err)
	}
	if plan.ReadyCount() != 1 {
		t.Fatalf("ready combo count = %d, want 1", plan.ReadyCount())
	}
	if plan.Combos[0].SkipReason != "" {
		t.Fatalf("ready combo skip reason = %q, want empty", plan.Combos[0].SkipReason)
	}
	if plan.Combos[0].Executable != executable {
		t.Fatalf("combo executable = %q, want %q", plan.Combos[0].Executable, executable)
	}
}

func TestPlanRuntimeBackendCombosPrefersConfiguredModel(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	configuredModelRelative := filepath.Join("custom", "configured.litertlm")
	configuredModel := filepath.Join(repoRoot, configuredModelRelative)
	if err := os.MkdirAll(filepath.Dir(configuredModel), 0o755); err != nil {
		t.Fatalf("create configured model directory: %v", err)
	}
	if err := os.WriteFile(configuredModel, []byte("configured model"), 0o644); err != nil {
		t.Fatalf("write configured model: %v", err)
	}
	catalogModel := filepath.Join(repoRoot, "models", "litert", "main", "gemma-4-E2B-it.litertlm")
	if err := os.MkdirAll(filepath.Dir(catalogModel), 0o755); err != nil {
		t.Fatalf("create catalog model directory: %v", err)
	}
	if err := os.WriteFile(catalogModel, []byte("catalog model"), 0o644); err != nil {
		t.Fatalf("write catalog model: %v", err)
	}

	configPath := filepath.Join(repoRoot, "native", "runtime-config", "backends.json")
	config := `{
  "version": 1,
  "runtimes": {
    "litert": {
      "cpu": {"working": true, "model": "` + filepath.ToSlash(configuredModelRelative) + `"}
    }
  }
}`
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	executable := filepath.Join(t.TempDir(), "litert-lm")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}

	plan, err := PlanRuntimeBackendCombos(SuiteOptions{
		RepoRoot:          repoRoot,
		BackendConfigPath: configPath,
		LiteRTExecutable:  executable,
	})
	if err != nil {
		t.Fatalf("plan runtime backend combos: %v", err)
	}
	if plan.ReadyCount() != 1 {
		t.Fatalf("ready combo count = %d, want 1", plan.ReadyCount())
	}
	if got := plan.Combos[0].ModelPath; got != configuredModel {
		t.Fatalf("combo model path = %q, want configured model %q", got, configuredModel)
	}
	if plan.Combos[0].ModelID == "" {
		t.Fatal("configured model combo should keep a non-empty model id")
	}
}
