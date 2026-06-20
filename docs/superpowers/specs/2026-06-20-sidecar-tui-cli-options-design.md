# Sidecar TUI CLI Options Design

Date: 2026-06-20
Status: Approved design, pending implementation plan
Selected approach: Add Bubbles for option modals and keep Harmonica out until animations are requested

## Objective

Extend the sidecar Bubble Tea TUI launch workflow so every visible runner action
is mouse-clickable, the Launch Wizard has four panels, and runner creation uses
a command preview assembled from selected runtime, backend, model, and CLI
option overrides.

## Requirements

### R1: Mouse-Clickable Runner Actions

WHEN a runner tab is visible
THE SYSTEM SHALL make every visible runner action clickable with the mouse:
Start, Stop, Restart, Edit Cmd, and Close.

WHEN a runner action is available in the footer and in the runner body
THE SYSTEM SHALL route both click targets to the same controller method.

### R2: Four Launch Wizard Panels

WHEN the Launch Wizard opens
THE SYSTEM SHALL show these panels:

- Runtime/backend selection
- Local model selection
- CLI options
- Command preview

WHEN terminal width is narrow
THE SYSTEM SHALL stack panels without losing clickable targets.

WHEN terminal width is wide
THE SYSTEM SHALL keep the existing masonry-style panel layout.

### R3: CLI Option Buttons

WHEN runtime, backend, and model choices are selected
THE SYSTEM SHALL render applicable CLI options as clickable buttons using short
flag labels where a short flag exists.

WHEN an option is not applicable to the selected runtime or role
THE SYSTEM SHALL hide it from the Launch Wizard options panel.

WHEN an option already has an override
THE SYSTEM SHALL render it with its current override value in the button label
or adjacent compact text.

### R4: Option Modal

WHEN the user clicks a CLI option button
THE SYSTEM SHALL open a modal containing:

- short flag label
- full flag name
- one or two sentence description
- default value
- enum values when known
- input field
- Save button
- Clear button
- X close button

WHEN the user clicks Clear
THE SYSTEM SHALL remove that option override.

WHEN the user saves an empty value for a boolean flag
THE SYSTEM SHALL include the bare flag form when that option supports it.

WHEN the user cancels or closes the modal
THE SYSTEM SHALL leave existing overrides unchanged.

### R5: Command Preview And Start

WHEN any wizard choice or option override changes
THE SYSTEM SHALL update the Command preview panel immediately.

WHEN the user clicks START or presses Enter
THE SYSTEM SHALL create the runner with a command override matching the Command
preview.

WHEN the selected runtime is LiteRT
THE SYSTEM SHALL keep the existing `litert-lm serve` behavior and only offer
server-supported options: `--host`, `--port`, and `--verbose`.

WHEN the selected runtime is llama.cpp
THE SYSTEM SHALL build a `llama-server` command that includes selected
low-memory, GPU, embedding, reranking, reasoning, and speculative overrides.

## Approved Dependencies

Add `github.com/charmbracelet/bubbles v1.0.0` for modal text input and future
list/viewport use. Do not add Harmonica in this change; animations can be added
later if they serve a concrete interaction.

## llama.cpp Option Catalog

The catalog is static source code, not parsed live from `--help`. The source
comments or docs must cite the current official docs and the local installed
`llama-server --help` used during design.

### Memory And Low-VRAM CUDA

- `-c`, `--ctx-size`: context size; lower values reduce KV cache memory.
- `-ctk`, `--cache-type-k`: KV cache K type; enum `f32`, `f16`, `bf16`,
  `q8_0`, `q4_0`, `q4_1`, `iq4_nl`, `q5_0`, `q5_1`; default `f16`.
- `-ctv`, `--cache-type-v`: KV cache V type; same enum and default as K.
- `-fa`, `--flash-attn`: Flash Attention mode; enum `on`, `off`, `auto`;
  default `auto`.
- `-ngl`, `--gpu-layers`: layers to offload; values include integer, `auto`,
  and `all`; default `auto`.
- `-fit`, `--fit`: memory fitting; enum `on`, `off`; default `on`.
- `-fitt`, `--fit-target`: per-device MiB margin for fit; default `1024`.
- `-fitc`, `--fit-ctx`: minimum context size fit may use; default `4096`.
- `-b`, `--batch-size`: logical batch size; default `2048`.
- `-ub`, `--ubatch-size`: physical batch size; default `512`.
- `-np`, `--parallel`: server slots; default `-1` auto.
- `-kvo`, `--kv-offload`: KV cache offload; enabled by default.
- `--no-kv-offload`: keep KV off device when GPU memory is too tight.
- `--no-mmap`: disable memory-mapped model loading.
- `--mlock`: keep model in RAM instead of swap or compression.
- `-cram`, `--cache-ram`: prompt cache RAM MiB; default `8192`.
- `--cache-prompt`: prompt caching; enabled by default.
- `--cache-reuse`: KV shifting reuse chunk size; default `0`.

### CUDA And Multi-GPU

- `-dev`, `--device`: restrict offload devices; use with `--list-devices`.
- `--list-devices`: print visible devices and exit.
- `-mg`, `--main-gpu`: primary GPU index; default `0`.
- `-sm`, `--split-mode`: enum `none`, `layer`, `row`, `tensor`; default
  `layer`.
- `-ts`, `--tensor-split`: comma-separated split proportions.
- `--no-host`: bypass host buffer.
- `--op-offload`: host operation offload; default enabled.
- `--no-op-offload`: disable host operation offload.
- `-cmoe`, `--cpu-moe`: keep all MoE weights on CPU.
- `-ncmoe`, `--n-cpu-moe`: keep first N MoE layers on CPU.

### Intel iGPU And SYCL

Expose the same memory controls as CUDA plus `-dev`, `-mg`, and `-ngl`.
Modal descriptions SHALL mention that SYCL depends on Intel GPU drivers and
oneAPI runtime, shared memory pressure, and that FP16 SYCL builds are generally
recommended by upstream docs for better performance in most cases.

### MTP And Speculative Decoding

- `--spec-type`: enum `none`, `draft-simple`, `draft-mtp`, `ngram-cache`,
  `ngram-simple`, `ngram-map-k`, `ngram-map-k4v`, `ngram-mod`; default `none`.
- `--spec-default`: use default speculative decoding config.
- `--spec-draft-n-max`: max draft tokens; default `3`.
- `--spec-draft-n-min`: minimum draft tokens; default `0`.
- `-md`, `--model-draft`: local draft model path.
- `--spec-draft-hf`: Hugging Face draft model repo.
- `-ngld`, `--gpu-layers-draft`: draft model GPU layers; default `auto`.
- `-devd`, `--device-draft`: draft model offload devices.
- `-ctkd`, `--cache-type-k-draft`: draft K cache type; same KV enum.
- `-ctvd`, `--cache-type-v-draft`: draft V cache type; same KV enum.
- `--spec-ngram-mod-n-match`: ngram-mod lookup length; default `24`.
- `--spec-ngram-mod-n-min`: ngram-mod minimum tokens; default `48`.
- `--spec-ngram-mod-n-max`: ngram-mod maximum tokens; default `64`.

### Embedding And Reranking

- `--embedding`, `--embeddings`: embedding mode.
- `--pooling`: enum `none`, `mean`, `cls`, `last`, `rank`.
- `--rerank`, `--reranking`: enable reranking endpoint.
- `--embd-normalize`: embedding normalization; default `2`.

### Reasoning And Server

- `-rea`, `--reasoning`: enum `on`, `off`, `auto`; default `auto`.
- `--reasoning-budget`: thinking token budget; default `-1`.
- `-to`, `--timeout`: server read/write timeout seconds; default `3600`.

## LiteRT Option Catalog

LiteRT Launch Wizard server options are intentionally small:

- `--host`: server host; default `0.0.0.0` in `litert-lm serve`.
- `--port`: server port; default `9379` in `litert-lm serve`.
- `--verbose`: verbose logging.

LiteRT run-only options such as `--backend`, `--max-num-tokens`,
`--enable-speculative-decoding`, sampling flags, cache mode, vision/audio
backend, and attachments are documented source references only. They are not
offered in the server Launch Wizard until the sidecar supports run-mode
runners.

## Data Model

Add a wizard-local override map keyed by canonical option ID. Each option entry
contains:

- ID
- runtime
- roles
- backend families
- short flag
- long flag
- value kind: bool, int, float, string, enum
- default text
- enum values
- description
- command emission rule

The first implementation keeps overrides in TUI memory. Persisting launch
presets is out of scope.

## Command Generation

The command builder SHALL produce argv first, then render the preview from argv.
Do not build command strings by concatenating shell fragments.

Base llama.cpp command:

```text
llama-server -m <model> --alias <model-id> --host <host> --port <port>
```

Role defaults:

- embedding adds `--embedding`
- reranking adds `--embedding --pooling rank --reranking`

Backend defaults:

- GPU-like backends keep existing default `--n-gpu-layers 999` unless the user
  overrides `-ngl`.

Option overrides append after defaults. An override for the same flag replaces
the default flag value instead of duplicating it.

## Hit Testing

Replace fragile fixed X constants for new controls with a simple button hit
registry built while rendering each panel. Each registry entry stores action
ID, row, start column, end column, and optional payload.

The registry SHALL cover:

- runner body controls
- footer runner controls
- wizard runtime/backend buttons
- wizard model rows
- wizard option buttons
- wizard START
- modal Save, Clear, and X buttons

## Testing

Focused Go tests SHALL prove:

- runner body action clicks call the same controller actions as footer clicks
- wizard renders four panels
- option buttons open the modal
- modal Save applies an override
- modal Clear removes an override
- command preview changes after an override
- START creates a runner with the preview command as command override
- llama.cpp option catalog includes KV quantization and MTP/speculative options
- LiteRT wizard does not expose run-only options for server launch
- CUDA and SYCL option groups include low-VRAM controls

Verification commands:

```bash
cd native/sidecar && go test ./internal/tui ./internal/supervisor
bun test tests/install-scripts.test.ts
bash -n configure.sh install.sh
```

Full real-runtime e2e remains useful but is not required for the TUI option
implementation because local model load time can exceed short test timeouts.

## Sources

- Official llama.cpp server README
- Official llama.cpp speculative decoding docs
- Official llama.cpp multi-GPU docs
- Official llama.cpp SYCL backend docs
- Local installed `llama-server --help`
- Google LiteRT-LM CLI usage docs
- Local installed `litert-lm --help`, `litert-lm run --help`, and
  `litert-lm serve --help`

## Out Of Scope

- Animated transitions
- Persistent option presets
- Dynamic parsing of every future CLI flag from `--help`
- LiteRT run-mode runner support
- Browser UI changes
