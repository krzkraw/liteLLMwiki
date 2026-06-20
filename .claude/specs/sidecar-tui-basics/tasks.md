# Sidecar TUI Basics Tasks

## T-1: Simplify TUI Shell

- **Status**: completed
- **Requirements**: US-1, US-2, US-4
- **Description**: Render only Dashboard, Launch Wizard, and created runner tabs. Replace the old runtime status with per-runtime active/idle status and add a bottom action bar.
- **Verification**: `go test ./internal/tui`

## T-2: Simplify Dashboard

- **Status**: completed
- **Requirements**: US-2, US-3
- **Description**: Replace the dashboard with one status widget for runners by runtime, runners by role, and local models by role.
- **Verification**: `go test ./internal/tui`

## T-3: Add Mouse-Driven Wizard

- **Status**: completed
- **Requirements**: US-5
- **Description**: Support clickable runtime, variant, role, model, and START controls. Create and start role-numbered runner tabs.
- **Verification**: `go test ./internal/tui`

## T-4: Make Interactive Startup Lazy

- **Status**: completed
- **Requirements**: US-1
- **Description**: Add a supervisor opt-out for the default LiteRT runner and use it in interactive TUI mode while preserving headless autostart.
- **Verification**: `go test ./internal/supervisor ./cmd/litert-sidecar`

## T-5: Complete Mouse Actions For Basic Runner Tabs

- **Status**: completed
- **Requirements**: US-4
- **Description**: Make runner tab bottom-bar actions clickable so mouse start, stop, and restart use the same shared runner controller methods as keyboard actions.
- **Verification**: `go test ./internal/tui`

## T-6: Make Body Layout Responsive

- **Status**: completed
- **Requirements**: US-6
- **Description**: Replace oversized single panels and appended dropdown content with responsive body sections that stack full-width on narrow terminals and masonry-balance into two columns on wide terminals.
- **Verification**: `go test ./internal/tui`

## T-7: Reintroduce Secondary Views Deliberately

- **Status**: pending
- **Requirements**: none
- **Description**: Future task. Chat, model downloads, logs, and settings can return after the basic runner workflow is legible.
- **Verification**: Not started.
