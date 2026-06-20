# llama.cpp Runtimes

The interactive installers place downloaded llama.cpp runtime archives here.
Each runtime lives in its own ignored folder, for example:

```text
llama-win-cpu-x64/
llama-win-cuda-13.3-x64/
llama-macos-arm64/
```

CUDA runtime folders also receive the matching `cudart` DLL archive contents.
The launch scripts add the selected runtime folder to `PATH` before starting the
G0LiteLLaMa so runner creation can resolve `llama-server` or `llama-server.exe`.

Downloaded runtime binaries and DLLs are local artifacts and must not be
committed.
