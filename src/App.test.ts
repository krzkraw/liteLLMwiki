import { describe, expect, it } from "bun:test";
import {
  createFolderSelectionKey,
  createFolderSummaryKey,
  createSidecarStatusPanel,
  createModelSourceKey,
  createBackendOptions,
  canUseNativeAttachments,
  buildEffectiveSystemPrompt,
  createFolderSummarizerForProvider,
  createBrowserProviderLoadConfig,
  createExecutableProviderLoadConfig,
  createSidecarRuntimeConfig,
  getDisplayedFolderSummaryMode,
  resolveExecutableBackend,
  shouldApplyAsyncFolderResult,
  shouldApplyAsyncModelResult,
} from "./App";

describe("App model async guards", () => {
  it("creates different source keys for path and local file selections", () => {
    const localFile = new File(["model"], "gemma-4-E2B-it-web.litertlm", {
      lastModified: 1234,
    });

    expect(createModelSourceKey("/models/a.litertlm", null)).toBe(
      "path:/models/a.litertlm",
    );
    expect(createModelSourceKey("/models/b.litertlm", null)).toBe(
      "path:/models/b.litertlm",
    );
    expect(createModelSourceKey("/models/a.litertlm", localFile)).toBe(
      "file:gemma-4-E2B-it-web.litertlm:5:1234",
    );
  });

  it("rejects late async model results after a newer request or source change", () => {
    expect(
      shouldApplyAsyncModelResult({
        requestId: 2,
        latestRequestId: 2,
        sourceKey: "path:/models/a.litertlm",
        currentSourceKey: "path:/models/a.litertlm",
      }),
    ).toBe(true);

    expect(
      shouldApplyAsyncModelResult({
        requestId: 1,
        latestRequestId: 2,
        sourceKey: "path:/models/a.litertlm",
        currentSourceKey: "path:/models/a.litertlm",
      }),
    ).toBe(false);

    expect(
      shouldApplyAsyncModelResult({
        requestId: 2,
        latestRequestId: 2,
        sourceKey: "path:/models/a.litertlm",
        currentSourceKey: "file:gemma-4-E2B-it-web.litertlm:5:1234",
      }),
    ).toBe(false);
  });
});

describe("App chat option helpers", () => {
  it("builds an effective system prompt with optional thinking guidance", () => {
    expect(buildEffectiveSystemPrompt(" Be concise. ", false)).toBe("Be concise.");
    expect(buildEffectiveSystemPrompt("Be concise.", true)).toBe(
      "Be concise.\n\nThinking mode: reason internally when useful, then return only the final answer unless the user asks for reasoning.",
    );
  });

  it("maps advanced web provider options into browser load config", () => {
    expect(
      createBrowserProviderLoadConfig({
        model: { kind: "url", value: "/models/model.litertlm" },
        values: {
          wasmPath: "/wasm",
          maxNumTokens: 4096,
          maxOutputTokens: 256,
          engineBackend: "GPU",
          samplerBackend: "CPU",
          samplerType: "TOP_P",
          temperature: 0.2,
          topK: 8,
          topP: 0.75,
          seed: 123,
          applyPromptTemplateInSession: false,
          useExternalSampler: true,
          enableConstrainedDecoding: true,
          prefillPrefaceOnInit: true,
          filterChannelContentFromKvCache: true,
        },
        contextLength: 8192,
        systemPrompt: "System.",
      }),
    ).toMatchObject({
      wasmPath: "/wasm",
      maxNumTokens: 4096,
      maxOutputTokens: 256,
      engineBackend: "GPU",
      samplerBackend: "CPU",
      samplerType: "TOP_P",
      temperature: 0.2,
      topK: 8,
      topP: 0.75,
      seed: 123,
      applyPromptTemplateInSession: false,
      useExternalSampler: true,
      enableConstrainedDecoding: true,
      prefillPrefaceOnInit: true,
      filterChannelContentFromKvCache: true,
      systemPrompt: "System.",
    });
  });

  it("maps advanced web token options into browser load config", () => {
    expect(
      createBrowserProviderLoadConfig({
        model: { kind: "url", value: "/models/model.litertlm" },
        values: {
          stopTokenIds: "[[1,2],[3]]",
          startTokenId: "5",
          numOutputCandidates: "2",
        },
        contextLength: 8192,
        systemPrompt: "System.",
      }),
    ).toMatchObject({
      stopTokenIds: [
        [1, 2],
        [3],
      ],
      startTokenId: 5,
      numOutputCandidates: 2,
    });
  });

  it("parses semicolon and comma stop token ID text", () => {
    expect(
      createBrowserProviderLoadConfig({
        model: { kind: "url", value: "/models/model.litertlm" },
        values: {
          stopTokenIds: "1,2; 3",
          startTokenId: 0,
          numOutputCandidates: 0,
        },
        contextLength: 8192,
        systemPrompt: "System.",
      }).stopTokenIds,
    ).toEqual([[1, 2], [3]]);
  });

  it("omits invalid web token options before creating browser load config", () => {
    const configFromInvalidJson = createBrowserProviderLoadConfig({
      model: { kind: "url", value: "/models/model.litertlm" },
      values: {
        stopTokenIds: '[[""],[-1],[1.5]]',
        startTokenId: "1.5",
        numOutputCandidates: "-2",
      },
      contextLength: 8192,
      systemPrompt: "System.",
    });
    const configFromInvalidText = createBrowserProviderLoadConfig({
      model: { kind: "url", value: "/models/model.litertlm" },
      values: {
        stopTokenIds: "1, two; 3",
        startTokenId: "0",
        numOutputCandidates: "2.5",
      },
      contextLength: 8192,
      systemPrompt: "System.",
    });

    expect(configFromInvalidJson.stopTokenIds).toBeUndefined();
    expect(configFromInvalidJson.startTokenId).toBeUndefined();
    expect(configFromInvalidJson.numOutputCandidates).toBeUndefined();
    expect(configFromInvalidText.stopTokenIds).toBeUndefined();
    expect(configFromInvalidText.startTokenId).toBeUndefined();
    expect(configFromInvalidText.numOutputCandidates).toBeUndefined();
  });

  it("maps executable sidecar options into websocket runtime config", () => {
    expect(
      createSidecarRuntimeConfig({
        endpoint: "http://127.0.0.1:9379/v1",
        runtimeExe: "/opt/litert-lm",
        runtimeHost: "127.0.0.1",
        runtimePort: 9481,
        modelFile: "models/litert/gemma-4-E2B-it.litertlm",
        modelId: "gemma4-e2b",
        huggingfaceToken: "hf_secret",
        importModel: false,
        launchRuntime: true,
        runtimeVerbose: true,
      }),
    ).toEqual({
      runtimeExe: "/opt/litert-lm",
      runtimeHost: "127.0.0.1",
      runtimePort: 9481,
      modelFile: "models/litert/gemma-4-E2B-it.litertlm",
      modelId: "gemma4-e2b",
      huggingfaceToken: "hf_secret",
      importModel: false,
      launchRuntime: true,
      runtimeVerbose: true,
    });
  });

  it("maps executable options into provider load config", () => {
    expect(
      createExecutableProviderLoadConfig({
        endpoint: "http://127.0.0.1:9379/v1",
        backend: "gpu",
        values: {
          modelId: "gemma4-e2b",
          maxNumTokens: 4096,
          maxTokens: 256,
          visionBackend: "cpu",
          audioBackend: "gpu",
          topK: 8,
          topP: 0.75,
          temperature: 0.2,
          seed: 123,
          preset: "creative.json",
          noTemplate: true,
          filterChannelContentFromKvCache: true,
          enableSpeculativeDecoding: "false",
          cache: "memory",
          verbose: true,
          fromHuggingFaceRepo: "google/gemma",
          huggingfaceToken: "hf_secret",
        },
        contextLength: 8192,
        systemPrompt: "System.",
      }),
    ).toMatchObject({
      endpoint: "http://127.0.0.1:9379/v1",
      modelId: "gemma4-e2b",
      backend: "gpu",
      maxNumTokens: 4096,
      maxTokens: 256,
      systemPrompt: "System.",
      visionBackend: "cpu",
      audioBackend: "gpu",
      topK: 8,
      topP: 0.75,
      temperature: 0.2,
      seed: 123,
      preset: "creative.json",
      noTemplate: true,
      filterChannelContentFromKvCache: true,
      enableSpeculativeDecoding: "false",
      cache: "memory",
      verbose: true,
      fromHuggingFaceRepo: "google/gemma",
      huggingfaceToken: "hf_secret",
    });
  });
});

describe("App folder async guards", () => {
  it("creates stable source keys for selected folder files", () => {
    const files = [
      {
        path: "src/main.ts",
        file: new File(["main"], "main.ts", { lastModified: 1000 }),
        sourceKind: "file-list" as const,
      },
      {
        path: "src/app.ts",
        file: new File(["app"], "app.ts", { lastModified: 2000 }),
        sourceKind: "file-list" as const,
      },
    ];

    expect(createFolderSelectionKey(files)).toBe(
      "src/app.ts:3:2000|src/main.ts:4:1000",
    );
  });

  it("rejects late async folder results after a newer selection", () => {
    expect(
      shouldApplyAsyncFolderResult({
        requestId: 3,
        latestRequestId: 3,
        selectionKey: "a",
        currentSelectionKey: "a",
      }),
    ).toBe(true);

    expect(
      shouldApplyAsyncFolderResult({
        requestId: 2,
        latestRequestId: 3,
        selectionKey: "a",
        currentSelectionKey: "a",
      }),
    ).toBe(false);

    expect(
      shouldApplyAsyncFolderResult({
        requestId: 3,
        latestRequestId: 3,
        selectionKey: "a",
        currentSelectionKey: "b",
      }),
    ).toBe(false);

    expect(
      shouldApplyAsyncFolderResult({
        requestId: 3,
        latestRequestId: 3,
        selectionKey: "a",
        currentSelectionKey: "a",
        summaryKey: "provider:web:path:/models/a.litertlm",
        currentSummaryKey: "deterministic:web:path:/models/a.litertlm",
      }),
    ).toBe(false);
  });
});

describe("App folder summarizer selection", () => {
  it("includes provider mode and backend source in folder summary keys", () => {
    expect(
      createFolderSummaryKey({
        providerKind: "web",
        summaryMode: "provider",
        modelSourceKey: "path:/models/a.litertlm",
        executableEndpoint: "http://127.0.0.1:9379/v1",
        backend: "auto",
      }),
    ).toBe("provider:web:path:/models/a.litertlm");

    expect(
      createFolderSummaryKey({
        providerKind: "executable",
        summaryMode: "provider",
        modelSourceKey: "path:/models/a.litertlm",
        executableEndpoint: " http://127.0.0.1:9380/v1 ",
        backend: "cpu",
      }),
    ).toBe("provider:executable:http://127.0.0.1:9380/v1:cpu");
  });

  it("uses provider-backed summarization only when the selected provider is loaded", async () => {
    const calls: string[] = [];
    const controller = new AbortController();
    const seenSignals: Array<AbortSignal | undefined> = [];
    const provider = {
      async generateText(prompt: string, signal?: AbortSignal) {
        calls.push(prompt);
        seenSignals.push(signal);
        return "Provider summary\nEntities: Provider";
      },
    };

    const providerChoice = createFolderSummarizerForProvider(provider, true, {
      signal: controller.signal,
    });
    const fallbackChoice = createFolderSummarizerForProvider(provider, false);

    expect(providerChoice.mode).toBe("provider");
    expect(fallbackChoice.mode).toBe("deterministic");

    const fragment = await providerChoice.summarizer.summarizeChunk({
      id: "chunk",
      fileId: "file",
      path: "src/main.ts",
      index: 0,
      startOffset: 0,
      endOffset: 21,
      text: "export const value = 1;",
      hash: "hash",
      bytes: 21,
    });

    expect(fragment.summary).toBe("Provider summary");
    expect(fragment.entities).toEqual(["Provider"]);
    expect(calls).toHaveLength(1);
    expect(seenSignals).toEqual([controller.signal]);

    const fallbackFragment = await fallbackChoice.summarizer.summarizeChunk({
      id: "fallback-chunk",
      fileId: "file",
      path: "src/main.ts",
      index: 0,
      startOffset: 0,
      endOffset: 21,
      text: "export const fallbackValue = 1;",
      hash: "fallback-hash",
      bytes: 21,
    });

    expect(fallbackFragment.summary).toContain("fallbackValue");
    expect(calls).toHaveLength(1);
  });

  it("keeps the displayed mode tied to the summary that produced the graph", () => {
    expect(
      getDisplayedFolderSummaryMode({
        hasSummaryResult: true,
        summaryMode: "deterministic",
        currentMode: "provider",
      }),
    ).toBe("deterministic");

    expect(
      getDisplayedFolderSummaryMode({
        hasSummaryResult: false,
        summaryMode: "deterministic",
        currentMode: "provider",
      }),
    ).toBe("provider");
  });
});

describe("App executable provider helpers", () => {
  it("creates backend options from sidecar status and keeps cuda probe-only", () => {
    expect(
      createBackendOptions([
        { backend: "cpu", state: "available" },
        { backend: "gpu", state: "unavailable" },
        { backend: "npu", state: "unknown" },
        { backend: "cuda", state: "not-a-litert-backend" },
      ]),
    ).toEqual([
      { value: "auto", label: "Auto" },
      { value: "cpu", label: "CPU" },
      { value: "gpu", label: "GPU unavailable", disabled: true },
      { value: "npu", label: "NPU unknown", disabled: true },
      { value: "cuda", label: "CUDA probe-only", disabled: true },
    ]);
  });

  it("formats sidecar status for setup panel display", () => {
    expect(
      createSidecarStatusPanel({
        state: "available",
        detail: "ready",
        backends: [
          { backend: "cpu", state: "available" },
          { backend: "cuda", state: "not-a-litert-backend" },
        ],
        capabilities: {
          multimodal: { state: "available" },
        },
      }),
    ).toEqual({
      state: "ready",
      label: "Sidecar connected",
      detail: "ready",
    });
  });

  it("falls back to auto when selected executable backend is unavailable", () => {
    expect(
      resolveExecutableBackend("gpu", [
        { backend: "gpu", state: "unavailable" },
        { backend: "cpu", state: "available" },
      ]),
    ).toBe("auto");

    expect(
      resolveExecutableBackend("cpu", [
        { backend: "gpu", state: "unavailable" },
        { backend: "cpu", state: "available" },
      ]),
    ).toBe("cpu");
  });

  it("allows native attachments only for a loaded executable provider with multimodal capability", () => {
    expect(
      canUseNativeAttachments({
        providerKind: "executable",
        executableLoaded: true,
        multimodalState: "available",
      }),
    ).toBe(true);

    expect(
      canUseNativeAttachments({
        providerKind: "web",
        executableLoaded: true,
        multimodalState: "available",
      }),
    ).toBe(false);

    expect(
      canUseNativeAttachments({
        providerKind: "executable",
        executableLoaded: false,
        multimodalState: "available",
      }),
    ).toBe(false);

    expect(
      canUseNativeAttachments({
        providerKind: "executable",
        executableLoaded: true,
        multimodalState: "unavailable",
      }),
    ).toBe(false);
  });
});
