---
name: tui-model-screenshot
description: Use when a G0LiteLLaMa visible TUI change needs a real screenshot from Bubble Tea/Lipgloss model output without relying on manual terminal screenshots.
---

# TUI Model Screenshot

## Overview

Capture screenshots from the actual Go TUI render: `Model.View().Content`
ANSI output converted to PNG. This is for visual evidence only; keep
`bun run e2e:tui` for rendered behavior verification.

## When To Use

- A visible TUI edit needs the AGENTS-required screenshot.
- Terminal, tmux, Ghostty, or browser screenshots are noisy or unavailable.
- You need a deterministic screenshot of a model state.

Do not use this as a replacement for `@microsoft/tui-test` verification.

## Steps

1. Add a temporary `G0LiteLLaMa/internal/tui/zz_screenshot_test.go`.
2. Build the real `Model`, set width/height/state, and write
   `model.View().Content` to `/tmp/g0litellama-screens/name.ansi`.
3. Run only that test.
4. Convert ANSI to PNG:

```bash
python3 skills/tui-model-screenshot/render_ansi_png.py \
  /tmp/g0litellama-screens/name.ansi \
  /tmp/g0litellama-screens/name.png
```

5. Inspect/show the PNG.
6. Delete `zz_screenshot_test.go` before verification and commit.

## Temporary Test Shape

```go
package tui

import (
	"os"
	"path/filepath"
	"testing"

	"g0litellama/internal/server"
)

func TestDumpTUIScreen(t *testing.T) {
	model := NewModel(ModelOptions{
		RunnerController:  newFakeRunnerController(nil),
		Logs:              server.NewLogBroadcaster(8),
		Catalog:           testCatalogWithPresentModels(t),
		LlamaRuntimeRoot:  testLlamaRuntimeRoot(t, "llama-win-cuda-13.3-x64"),
		BackendConfigPath: filepath.Join(t.TempDir(), "missing-backends.json"),
		ManagedScreen:     true,
	})
	model.width = 120
	model.height = 34
	model.setActiveTab("wizard")

	outDir := "/tmp/g0litellama-screens"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "wizard.ansi"),
		[]byte(model.View().Content), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

## Checks

Before finishing, run:

```bash
test ! -e G0LiteLLaMa/internal/tui/zz_screenshot_test.go
git diff --check
```
