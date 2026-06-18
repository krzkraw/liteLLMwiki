# WebUI Native Runner

This directory is the webUI-adjacent home for native sidecar release artifacts.
The browser cannot start the sidecar process by itself, but once this sidecar is
started manually, the web UI controls the managed LiteRT-LM runtime through
WebSocket messages at `/sidecar/v1/ws`.

Build or refresh the artifacts from `demo/`:

```bash
npm run build:sidecar
```

That creates:

```text
native/sidecar/litert-sidecar-darwin-arm64/litert-sidecar
native/sidecar/litert-sidecar-darwin-amd64/litert-sidecar
native/sidecar/litert-sidecar-windows-amd64/litert-sidecar.exe
native/sidecar/litert-sidecar-windows-arm64/litert-sidecar.exe
```

macOS arm64 example:

```bash
./native/sidecar/litert-sidecar-darwin-arm64/litert-sidecar \
  -runtime-exe /path/to/litert-lm
```

Windows amd64 example from the `demo` directory:

```powershell
.\native\sidecar\litert-sidecar-windows-amd64\litert-sidecar.exe `
  -runtime-exe C:\path\to\litert-lm.exe
```

The generated binary directories are ignored by git; the source of truth remains
`../native/sidecar` plus this webUI-local build output location.
