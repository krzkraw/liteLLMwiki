import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "loop-factory"


class LoopFactoryCLITest(unittest.TestCase):
    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp(prefix="loop-factory-test-"))
        specs = self.tmp / "factory" / "specs"
        (specs / "inbox").mkdir(parents=True)
        (specs / "active").mkdir()
        (specs / "archive").mkdir()
        self.spec = specs / "inbox" / "2026-06-21-chat.md"
        self.spec.write_text(
            """---
id: chat-clickable-streaming
title: Clickable streaming chat TUI
status: inbox
verification:
  - cd G0LiteLLaMa && go test ./...
  - bun run e2e:tui
---

# Clickable streaming chat TUI

## Goal

Make chat clickable.

## Acceptance Criteria

- Input accepts punctuation.
- Chat streams responses.

# Grill Gate

- Enter sends.
- Shift+Enter inserts a newline.
""",
            encoding="utf-8",
        )

    def tearDown(self):
        shutil.rmtree(self.tmp)

    def run_cli(self, *args):
        return subprocess.run(
            [sys.executable, str(CLI), *args],
            cwd=self.tmp,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )

    def test_scan_lists_queue_counts_and_inbox_spec(self):
        result = self.run_cli("scan")

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("inbox: 1", result.stdout)
        self.assertIn("active: 0", result.stdout)
        self.assertIn("chat-clickable-streaming", result.stdout)

    def test_dispatch_renders_codex_prompt_without_staging(self):
        result = self.run_cli("dispatch", "--agent", "codex", "--limit", "1")

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Spec: chat-clickable-streaming", result.stdout)
        self.assertIn("Implement only the acceptance criteria", result.stdout)
        self.assertTrue(self.spec.exists())

    def test_dispatch_stage_moves_spec_to_active(self):
        result = self.run_cli(
            "dispatch", "--agent", "codex", "--limit", "1", "--stage"
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(self.spec.exists())
        active = self.tmp / "factory" / "specs" / "active" / self.spec.name
        self.assertTrue(active.exists())
        self.assertIn("staged: factory/specs/active/2026-06-21-chat.md", result.stdout)

    def test_loop_prompt_renders_builtin_review_gate(self):
        result = self.run_cli(
            "loops", "prompt", "independent-review-gate", "--agent", "codex"
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("independent-review-gate", result.stdout)
        self.assertIn("Review the active spec implementation", result.stdout)


if __name__ == "__main__":
    unittest.main()
