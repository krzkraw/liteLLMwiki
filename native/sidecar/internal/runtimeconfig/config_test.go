package runtimeconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingConfigKeepsBackendsVisible(t *testing.T) {
	t.Parallel()

	status, err := Load(filepath.Join(t.TempDir(), "backends.json"))
	if err != nil {
		t.Fatalf("load missing config: %v", err)
	}

	if status.Configured() {
		t.Fatal("missing config should not be marked configured")
	}
	for _, backend := range []string{"cpu", "gpu", "npu"} {
		if !status.Visible("litert", backend) {
			t.Fatalf("litert/%s hidden without config", backend)
		}
	}
}

func TestLoadRuntimeBackendResults(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "backends.json")
	contents := `{
  "version": 1,
  "runtimes": {
    "litert": {
      "cpu": {"working": true},
      "npu": {"working": false}
    },
    "llamacpp": {
      "cpu": {"working": true},
      "cuda13": {"working": false}
    }
  }
}`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	status, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if !status.Configured() {
		t.Fatal("existing config should be marked configured")
	}
	if !status.Visible("litert", "cpu") {
		t.Fatal("working litert cpu backend should remain visible")
	}
	if status.Visible("litert", "npu") {
		t.Fatal("not-working litert npu backend should be hidden")
	}
	if status.Visible("llamacpp", "cuda13") {
		t.Fatal("not-working llama cuda13 backend should be hidden")
	}
	if !status.Visible("llamacpp", "openvino") {
		t.Fatal("unmentioned llama backend should remain visible")
	}
}

func TestDefaultPathUsesRuntimeConfigDirectory(t *testing.T) {
	t.Parallel()

	got := DefaultPath("/repo")
	want := filepath.Join("/repo", "native", "runtime-config", "backends.json")
	if got != want {
		t.Fatalf("default path = %q, want %q", got, want)
	}
}

func TestSetBackendWorkingCreatesAndPreservesBackendResults(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config", "backends.json")
	if err := SetBackendWorking(path, "llamacpp", "cuda13", false); err != nil {
		t.Fatalf("set missing backend config: %v", err)
	}

	status, err := Load(path)
	if err != nil {
		t.Fatalf("load created config: %v", err)
	}
	if status.Visible("llamacpp", "cuda13") {
		t.Fatal("created config should hide disabled llama cuda13 backend")
	}

	contents := `{
  "version": 1,
  "runtimes": {
    "litert": {
      "gpu": {
        "working": true,
        "command": "litert-lm serve --backend gpu",
        "model": "models/litert/main/gemma.litertlm",
        "testedAt": "2026-06-20T10:00:00Z",
        "output": "probe ok"
      }
    }
  }
}`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	if err := SetBackendWorking(path, " LiteRT ", " GPU ", false); err != nil {
		t.Fatalf("set existing backend config: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}
	var decoded fileConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode updated config: %v", err)
	}
	result := decoded.Runtimes["litert"]["gpu"]
	if result.Working {
		t.Fatal("updated backend should be disabled")
	}
	if result.Command != "litert-lm serve --backend gpu" {
		t.Fatalf("command = %q, want preserved command", result.Command)
	}
	if result.Model != "models/litert/main/gemma.litertlm" {
		t.Fatalf("model = %q, want preserved model", result.Model)
	}
	if result.Tested != "2026-06-20T10:00:00Z" {
		t.Fatalf("testedAt = %q, want preserved testedAt", result.Tested)
	}
	if result.Output != "probe ok" {
		t.Fatalf("output = %q, want preserved output", result.Output)
	}
}
