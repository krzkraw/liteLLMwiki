# Public Assets

Static browser assets live here. The browser-compatible Gemma 4 E2B web model
belongs in the repository-local model directory instead:

```text
../models/litert/browser/gemma-4-E2B-it-web.litertlm
```

The app defaults to:

```text
/models/litert/browser/gemma-4-E2B-it-web.litertlm
```

The model source is:

```text
https://huggingface.co/litert-community/gemma-4-E2B-it-litert-lm
```

Download and verify:

```bash
bun run download:model
bun run check:model
```

You can also choose the `.litertlm` model directly from disk in the app. That
avoids relying on the repository-local model route.
