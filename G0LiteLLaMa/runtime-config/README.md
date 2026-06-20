# Runtime Backend Configuration

`configure.sh` and `configure.ps1` write local runtime/backend test results to
`backends.json` in this directory. The file is ignored because it records
machine-specific runtime, backend, model, command, and output details.
Configure marks a backend working only after the probe gets a usable model
response: LiteRT must return non-error `litert-lm run` output, and llama.cpp
starts with reasoning off and must return non-empty chat-completion content
from `/v1/chat/completions`.

The G0LiteLLaMa TUI reads `backends.json` when it exists. A backend marked
`"working": false` is hidden from the Launch Wizard. Missing config, missing
runtime entries, and missing backend entries keep the existing default wizard
behavior.
