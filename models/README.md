# Local Models

Put externally hosted model files under runtime and role folders when needed for
local runs.

Expected model paths:

```text
litert/browser/gemma-4-E2B-it-web.litertlm
litert/main/gemma-4-E2B-it.litertlm
litert/embedding/embeddinggemma-300M_seq2048_mixed-precision.tflite
llamacpp/main/gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf
llamacpp/main/Qwen3.5-2B-IQ4_NL.gguf
llamacpp/main/Qwen3.5-0.8B-UD-Q8_K_XL.gguf
llamacpp/embedding/Qwen3-Embedding-0.6B-q8_0.gguf
llamacpp/embedding/Qwen3-Embedding-0.6B-iq4_nl.gguf
llamacpp/reranking/Qwen3-Reranker-0.6B-Q4_K_M.gguf
```

Actual model binaries, partial downloads, and split model chunks are ignored by
Git and must not be committed.
