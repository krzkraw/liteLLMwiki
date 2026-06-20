package runtimeconfig

import (
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
