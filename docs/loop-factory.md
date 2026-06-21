# Loop Factory

Loop Factory is the repo-local queue for spec-driven agent work.

## Queues

- `factory/specs/inbox/` - approved specs ready to dispatch.
- `factory/specs/active/` - specs currently being implemented or reviewed.
- `factory/specs/archive/` - completed specs after review/archive.

Each spec is a Markdown file with frontmatter. Keep the acceptance criteria
small enough that one agent can implement and verify them without product
guesswork.

## Normal Operation

Inspect queue state:

```bash
python3 bin/loop-factory scan
```

Render the next Codex implementation prompt without moving files:

```bash
python3 bin/loop-factory dispatch --agent codex --limit 1
```

Start implementation by staging one inbox spec into active:

```bash
python3 bin/loop-factory dispatch --agent codex --limit 1 --stage
```

Implement only the active spec acceptance criteria. Run the verification
commands listed in the active spec frontmatter. Leave the spec in
`factory/specs/active/` for review unless the user explicitly asks to archive.

## Built-In Loops

List loops:

```bash
python3 bin/loop-factory loops list
```

Inspect one loop:

```bash
python3 bin/loop-factory loops show independent-review-gate
```

Render a loop prompt:

```bash
python3 bin/loop-factory loops prompt independent-review-gate --agent codex
```

Use these IDs for normal spec flow:

- `spec-grill-gate`
- `active-spec-verify`
- `independent-review-gate`
- `backprop-spec-sync`

## TUI Visual Changes

For visible G0LiteLLaMa TUI changes, follow
`skills/tui-model-screenshot/SKILL.md` for deterministic screenshot evidence.
This does not replace `bun run e2e:tui`.

## Maintenance

Verify the CLI with:

```bash
python3 -m unittest tests.test_loop_factory
```
