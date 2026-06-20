# Sidecar TUI Basics Requirements

## US-1: Focused TUI Shell

**As a** local runner operator
**I want** the terminal UI to show only the dashboard, launch wizard, and created runners
**So that** basic local runtime work is legible before adding secondary tools back.

### Acceptance Criteria

1. WHEN the interactive TUI starts
   THE SYSTEM SHALL show Dashboard and Launch Wizard tabs without Chat, Models, Logs, or Settings tabs.
2. WHEN the interactive TUI starts
   THE SYSTEM SHALL not create or start a default runtime runner.
3. WHEN headless mode starts
   THE SYSTEM SHALL preserve the legacy default runner startup used by automation.

## US-2: Runtime-Aware Status

**As a** local runner operator
**I want** LiteRT and llama.cpp status shown separately
**So that** I can see which runtime family is actually in use.

### Acceptance Criteria

1. WHEN no runner is running for a runtime family
   THE SYSTEM SHALL show that runtime family as idle.
2. WHEN any runner is running for a runtime family
   THE SYSTEM SHALL show that runtime family as active.
3. WHEN the dashboard is visible
   THE SYSTEM SHALL show live runner counts by runtime and by role.

## US-3: Dashboard Model Role Picker

**As a** local runner operator
**I want** model availability grouped by role
**So that** I can choose a launch path from local files.

### Acceptance Criteria

1. WHEN the dashboard is visible
   THE SYSTEM SHALL show one status widget with model counts for Main, Embedding, and Reranking.
2. WHEN a model role count is clicked
   THE SYSTEM SHALL open a local model list for that role.
3. WHEN rendering the dashboard
   THE SYSTEM SHALL not render system-health, topology, signal-board, route-map, backend-card, recent-activity, or hotkey panels.

## US-4: htop-Style Actions

**As a** local runner operator
**I want** actions always visible at the bottom
**So that** I do not need to scan panels for controls.

### Acceptance Criteria

1. WHEN any tab is visible
   THE SYSTEM SHALL render global actions and current-tab actions in a bottom action bar.
2. WHEN F1 is pressed or clicked
   THE SYSTEM SHALL toggle a bottom-left global actions menu.
3. WHEN a runner tab is visible
   THE SYSTEM SHALL let the user click the bottom-bar runner actions to start, stop, and restart that runner.

## US-5: Clickable Launch Wizard

**As a** local runner operator
**I want** to click through runtime, variant, role, model, and START
**So that** runner creation works like a compact config screen.

### Acceptance Criteria

1. WHEN the Launch Wizard is visible
   THE SYSTEM SHALL show runtime, variant, role, local model list, and START controls.
2. WHEN LiteRT is selected
   THE SYSTEM SHALL show cpu, gpu, and npu variants.
3. WHEN llama.cpp is selected
   THE SYSTEM SHALL show cpu, gpu, openvino, cuda13, cuda12, and sycl variant groups backed by installed runtime folders.
4. WHEN START is activated
   THE SYSTEM SHALL create and start a runner.
5. WHEN a runner is created
   THE SYSTEM SHALL insert a runner tab after Launch Wizard with an LR/LM runtime prefix, M/E/R role letter, and role-scoped number.

## US-6: Responsive Low-Box Layout

**As a** local runner operator
**I want** the TUI body to use fewer, wider panels
**So that** the interface stays legible in small terminals and does not waste wide terminals.

### Acceptance Criteria

1. WHEN the terminal is narrow
   THE SYSTEM SHALL render body panels as a single full-width stack.
2. WHEN the terminal is wide
   THE SYSTEM SHALL render body panels in two masonry-balanced columns.
3. WHEN Dashboard model details, Launch Wizard sections, or runner details are visible
   THE SYSTEM SHALL use the responsive layout rules without reintroducing the old diagnostic panel cluster.
