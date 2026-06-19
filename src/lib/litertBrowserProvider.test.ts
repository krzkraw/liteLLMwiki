import { describe, expect, it, mock, spyOn } from "bun:test";
import { Backend, SamplerType } from "@litert-lm/core";
import {
  createLiteRtBrowserProvider,
  extractTextFromLiteRtMessage,
} from "./litertBrowserProvider";

type FakeLiteRtMessage = {
  role: string;
  content?: string | Array<{ type: string; text?: string }>;
};

function createAsyncChunks(chunks: FakeLiteRtMessage[]) {
  return async function* streamChunks() {
    for (const chunk of chunks) {
      yield chunk;
    }
  };
}

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });

  return { promise, resolve, reject };
}

async function waitForPendingPromises() {
  await new Promise((resolve) => setTimeout(resolve, 0));
}

function createProviderFakes(chunks: FakeLiteRtMessage[] = []) {
  const conversation = {
    sendMessageStreaming: mock(createAsyncChunks(chunks)),
    cancel: mock(),
    delete: mock().mockResolvedValue(undefined),
  };
  const engine = {
    createConversation: mock().mockResolvedValue(conversation),
    delete: mock().mockResolvedValue(undefined),
  };
  const loadLiteRtLm = mock().mockResolvedValue(undefined);
  const Engine = {
    create: mock().mockResolvedValue(engine),
  };

  return { loadLiteRtLm, Engine, engine, conversation };
}

const config = {
  model: { kind: "url" as const, value: "/models/litert/browser/gemma-4-E2B-it-web.litertlm" },
  wasmPath: "/vendor/litert-lm/core/wasm",
  maxNumTokens: 8192,
  maxOutputTokens: 1024,
  systemPrompt: "You are concise.",
};

describe("extractTextFromLiteRtMessage", () => {
  it("extracts text from LiteRT message content arrays", () => {
    expect(
      extractTextFromLiteRtMessage({
        role: "assistant",
        content: [
          { type: "text", text: "Hello" },
          { type: "text", text: " world" },
        ],
      }),
    ).toBe("Hello world");
  });

  it("extracts text from string content chunks", () => {
    expect(
      extractTextFromLiteRtMessage({ role: "assistant", content: "Hello" }),
    ).toBe("Hello");
  });
});

describe("createLiteRtBrowserProvider", () => {
  it("loads LiteRT-LM once with the configured WASM path", async () => {
    const { loadLiteRtLm, Engine } = createProviderFakes();
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });

    await provider.load(config);
    await provider.load(config);

    expect(loadLiteRtLm).toHaveBeenCalledTimes(1);
    expect(loadLiteRtLm).toHaveBeenCalledWith("/vendor/litert-lm/core/wasm");
  });

  it("creates one engine and one conversation per load", async () => {
    const { Engine, engine, loadLiteRtLm } = createProviderFakes();
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });

    await provider.load(config);

    expect(Engine.create).toHaveBeenCalledTimes(1);
    expect(Engine.create).toHaveBeenCalledWith({
      model: "/models/litert/browser/gemma-4-E2B-it-web.litertlm",
      mainExecutorSettings: { maxNumTokens: 8192 },
    });
    expect(engine.createConversation).toHaveBeenCalledTimes(1);
    expect(engine.createConversation).toHaveBeenCalledWith({
      sessionConfig: { maxOutputTokens: 1024 },
      preface: {
        messages: [{ role: "system", content: "You are concise." }],
      },
    });
  });

  it("passes max output tokens and sampler params to the conversation", async () => {
    const { Engine, engine, loadLiteRtLm } = createProviderFakes();
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });

    await provider.load({
      ...config,
      maxOutputTokens: 256,
      temperature: 0.7,
      topK: 32,
      topP: 0.9,
      seed: 123,
    });

    expect(engine.createConversation).toHaveBeenCalledWith({
      sessionConfig: {
        maxOutputTokens: 256,
        samplerParams: { temperature: 0.7, k: 32, p: 0.9, seed: 123 },
      },
      preface: {
        messages: [{ role: "system", content: "You are concise." }],
      },
    });
  });

  it("passes advanced web options to engine and conversation configs", async () => {
    const { Engine, engine, loadLiteRtLm } = createProviderFakes();
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });

    await provider.load({
      ...config,
      engineBackend: "GPU",
      samplerBackend: "CPU",
      samplerType: "TOP_P",
      applyPromptTemplateInSession: false,
      useExternalSampler: true,
      enableConstrainedDecoding: true,
      prefillPrefaceOnInit: true,
      filterChannelContentFromKvCache: true,
    });

    expect(Engine.create).toHaveBeenCalledWith({
      model: "/models/litert/browser/gemma-4-E2B-it-web.litertlm",
      backend: Backend.GPU,
      mainExecutorSettings: {
        maxNumTokens: 8192,
        samplerBackend: Backend.CPU,
      },
    });
    expect(engine.createConversation).toHaveBeenCalledWith({
      sessionConfig: {
        maxOutputTokens: 1024,
        samplerParams: { type: SamplerType.TOP_P },
        samplerBackend: Backend.CPU,
        applyPromptTemplateInSession: false,
        useExternalSampler: true,
      },
      preface: {
        messages: [{ role: "system", content: "You are concise." }],
      },
      enableConstrainedDecoding: true,
      prefillPrefaceOnInit: true,
      filterChannelContentFromKvCache: true,
    });
  });

  it("omits a zero sampler seed so the runtime default is used", async () => {
    const { Engine, engine, loadLiteRtLm } = createProviderFakes();
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });

    await provider.load({
      ...config,
      temperature: 0.7,
      seed: 0,
    });

    expect(engine.createConversation).toHaveBeenCalledWith({
      sessionConfig: {
        maxOutputTokens: 1024,
        samplerParams: { temperature: 0.7 },
      },
      preface: {
        messages: [{ role: "system", content: "You are concise." }],
      },
    });
  });

  it("passes advanced token options to the conversation session config", async () => {
    const { Engine, engine, loadLiteRtLm } = createProviderFakes();
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });

    await provider.load({
      ...config,
      stopTokenIds: [
        [1, 2],
        [3],
      ],
      startTokenId: 5,
      numOutputCandidates: 2,
    });

    expect(engine.createConversation).toHaveBeenCalledWith({
      sessionConfig: {
        maxOutputTokens: 1024,
        stopTokenIds: [
          [1, 2],
          [3],
        ],
        startTokenId: 5,
        numOutputCandidates: 2,
      },
      preface: {
        messages: [{ role: "system", content: "You are concise." }],
      },
    });
  });

  it("omits empty and zero advanced token options from session config", async () => {
    const { Engine, engine, loadLiteRtLm } = createProviderFakes();
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });

    await provider.load({
      ...config,
      stopTokenIds: [],
      startTokenId: 0,
      numOutputCandidates: 0,
    });

    expect(engine.createConversation).toHaveBeenCalledWith({
      sessionConfig: { maxOutputTokens: 1024 },
      preface: {
        messages: [{ role: "system", content: "You are concise." }],
      },
    });
  });

  it("streams token text through onToken and returns final text", async () => {
    const { Engine, loadLiteRtLm, conversation } = createProviderFakes([
      { role: "assistant", content: [{ type: "text", text: "Hel" }] },
      { role: "assistant", content: "lo" },
    ]);
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });
    const onToken = mock();

    await provider.load(config);
    const result = await provider.generate({ text: "Say hello", onToken });

    expect(conversation.sendMessageStreaming).toHaveBeenCalledWith("Say hello");
    expect(onToken).toHaveBeenNthCalledWith(1, "Hel", "Hel");
    expect(onToken).toHaveBeenNthCalledWith(2, "lo", "Hello");
    expect(result).toEqual({ text: "Hello" });
  });

  it("uses a temporary conversation for folder text generation", async () => {
    const chatConversation = {
      sendMessageStreaming: mock(
        createAsyncChunks([{ role: "assistant", content: "chat response" }]),
      ),
      cancel: mock(),
      delete: mock().mockResolvedValue(undefined),
    };
    const summaryConversation = {
      sendMessageStreaming: mock(
        createAsyncChunks([{ role: "assistant", content: "folder summary" }]),
      ),
      cancel: mock(),
      delete: mock().mockResolvedValue(undefined),
    };
    const engine = {
      createConversation: mock()
        .mockResolvedValueOnce(chatConversation)
        .mockResolvedValueOnce(summaryConversation),
      delete: mock().mockResolvedValue(undefined),
    };
    const loadLiteRtLm = mock().mockResolvedValue(undefined);
    const Engine = { create: mock().mockResolvedValue(engine) };
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });

    await provider.load(config);
    const text = await provider.generateText("Summarize folder");

    expect(text).toBe("folder summary");
    expect(engine.createConversation).toHaveBeenCalledTimes(2);
    expect(summaryConversation.sendMessageStreaming).toHaveBeenCalledWith(
      "Summarize folder",
    );
    expect(summaryConversation.delete).toHaveBeenCalledTimes(1);
    expect(chatConversation.sendMessageStreaming).not.toHaveBeenCalled();
    await provider.generate({ text: "Chat", onToken: mock() });
    expect(chatConversation.sendMessageStreaming).toHaveBeenCalledWith("Chat");
  });

  it("cancels active folder text generation during dispose without double deletion", async () => {
    const nextChunk = createDeferred<FakeLiteRtMessage>();
    const chatConversation = {
      sendMessageStreaming: mock(createAsyncChunks([])),
      cancel: mock(),
      delete: mock().mockResolvedValue(undefined),
    };
    const summaryConversation = {
      sendMessageStreaming: mock(async function* streamChunks() {
        yield await nextChunk.promise;
      }),
      cancel: mock(),
      delete: mock().mockResolvedValue(undefined),
    };
    const engine = {
      createConversation: mock()
        .mockResolvedValueOnce(chatConversation)
        .mockResolvedValueOnce(summaryConversation),
      delete: mock().mockResolvedValue(undefined),
    };
    const loadLiteRtLm = mock().mockResolvedValue(undefined);
    const Engine = { create: mock().mockResolvedValue(engine) };
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });

    await provider.load(config);
    const generation = provider.generateText("Summarize folder");
    await waitForPendingPromises();
    await provider.dispose();
    nextChunk.resolve({ role: "assistant", content: "late summary" });
    await generation;

    expect(summaryConversation.cancel).toHaveBeenCalledTimes(1);
    expect(summaryConversation.delete).toHaveBeenCalledTimes(1);
  });

  it("cancels concurrent folder text generations together", async () => {
    const firstChunk = createDeferred<FakeLiteRtMessage>();
    const secondChunk = createDeferred<FakeLiteRtMessage>();
    const chatConversation = {
      sendMessageStreaming: mock(createAsyncChunks([])),
      cancel: mock(),
      delete: mock().mockResolvedValue(undefined),
    };
    const firstSummaryConversation = {
      sendMessageStreaming: mock(async function* streamChunks() {
        yield await firstChunk.promise;
      }),
      cancel: mock(),
      delete: mock().mockResolvedValue(undefined),
    };
    const secondSummaryConversation = {
      sendMessageStreaming: mock(async function* streamChunks() {
        yield await secondChunk.promise;
      }),
      cancel: mock(),
      delete: mock().mockResolvedValue(undefined),
    };
    const engine = {
      createConversation: mock()
        .mockResolvedValueOnce(chatConversation)
        .mockResolvedValueOnce(firstSummaryConversation)
        .mockResolvedValueOnce(secondSummaryConversation),
      delete: mock().mockResolvedValue(undefined),
    };
    const loadLiteRtLm = mock().mockResolvedValue(undefined);
    const Engine = { create: mock().mockResolvedValue(engine) };
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });

    await provider.load(config);
    const firstGeneration = provider.generateText("Summarize first");
    const secondGeneration = provider.generateText("Summarize second");
    await waitForPendingPromises();
    provider.cancel();
    firstChunk.resolve({ role: "assistant", content: "first" });
    secondChunk.resolve({ role: "assistant", content: "second" });
    await Promise.allSettled([firstGeneration, secondGeneration]);

    expect(firstSummaryConversation.cancel).toHaveBeenCalledTimes(1);
    expect(secondSummaryConversation.cancel).toHaveBeenCalledTimes(1);
    expect(firstSummaryConversation.delete).toHaveBeenCalledTimes(1);
    expect(secondSummaryConversation.delete).toHaveBeenCalledTimes(1);
  });

  it("rejects pre-aborted generation and does not start streaming", async () => {
    const { Engine, loadLiteRtLm, conversation } = createProviderFakes();
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });
    const controller = new AbortController();
    controller.abort();

    await provider.load(config);

    await expect(
      provider.generate({
        text: "Do not start",
        onToken: mock(),
        signal: controller.signal,
      }),
    ).rejects.toThrow("Generation aborted");
    expect(conversation.cancel).toHaveBeenCalledTimes(1);
    expect(conversation.sendMessageStreaming).not.toHaveBeenCalled();
  });

  it("cancels immediately when aborted during streaming and removes the listener", async () => {
    const nextChunk = createDeferred<FakeLiteRtMessage>();
    const conversation = {
      sendMessageStreaming: mock(async function* streamChunks() {
        yield await nextChunk.promise;
      }),
      cancel: mock(),
      delete: mock().mockResolvedValue(undefined),
    };
    const engine = {
      createConversation: mock().mockResolvedValue(conversation),
      delete: mock().mockResolvedValue(undefined),
    };
    const loadLiteRtLm = mock().mockResolvedValue(undefined);
    const Engine = { create: mock().mockResolvedValue(engine) };
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });
    const controller = new AbortController();
    const addEventListener = spyOn(controller.signal, "addEventListener");
    const removeEventListener = spyOn(controller.signal, "removeEventListener");

    await provider.load(config);
    const generation = provider.generate({
      text: "Start",
      onToken: mock(),
      signal: controller.signal,
    });
    await waitForPendingPromises();

    controller.abort();

    expect(conversation.cancel).toHaveBeenCalledTimes(1);

    nextChunk.resolve({ role: "assistant", content: "late" });

    await expect(generation).rejects.toThrow("Generation aborted");
    const abortListener = addEventListener.mock.calls.find(
      ([eventName]) => eventName === "abort",
    )?.[1];
    expect(removeEventListener).toHaveBeenCalledWith("abort", abortListener);
  });

  it("cancel calls conversation.cancel", async () => {
    const { Engine, loadLiteRtLm, conversation } = createProviderFakes();
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });

    await provider.load(config);
    provider.cancel();

    expect(conversation.cancel).toHaveBeenCalledTimes(1);
  });

  it("clears failed WASM load cache and retries", async () => {
    const { Engine, loadLiteRtLm } = createProviderFakes();
    loadLiteRtLm
      .mockReset()
      .mockRejectedValueOnce(new Error("WASM failed"))
      .mockResolvedValueOnce(undefined);
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });

    await expect(provider.load(config)).rejects.toThrow("WASM failed");
    await provider.load(config);

    expect(loadLiteRtLm).toHaveBeenCalledTimes(2);
    expect(Engine.create).toHaveBeenCalledTimes(1);
  });

  it("throws a clear error for a different WASM path after loading", async () => {
    const { Engine, engine, loadLiteRtLm } = createProviderFakes([
      { role: "assistant", content: "still loaded" },
    ]);
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });

    await provider.load(config);

    await expect(
      provider.load({ ...config, wasmPath: "/other/litert-lm/wasm" }),
    ).rejects.toThrow(
      'LiteRT-LM WASM already loaded from "/vendor/litert-lm/core/wasm" and cannot be reloaded from "/other/litert-lm/wasm".',
    );
    expect(loadLiteRtLm).toHaveBeenCalledTimes(1);
    expect(Engine.create).toHaveBeenCalledTimes(1);
    expect(engine.delete).not.toHaveBeenCalled();
  });

  it("dispose deletes conversation then engine", async () => {
    const events: string[] = [];
    const conversation = {
      sendMessageStreaming: mock(),
      cancel: mock(),
      delete: mock().mockImplementation(async () => {
        events.push("conversation");
      }),
    };
    const engine = {
      createConversation: mock().mockResolvedValue(conversation),
      delete: mock().mockImplementation(async () => {
        events.push("engine");
      }),
    };
    const loadLiteRtLm = mock().mockResolvedValue(undefined);
    const Engine = { create: mock().mockResolvedValue(engine) };
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });

    await provider.load(config);
    await provider.dispose();

    expect(events).toEqual(["conversation", "engine"]);
  });

  it("reload disposes previous engine before creating a new one", async () => {
    const events: string[] = [];
    const firstConversation = {
      sendMessageStreaming: mock(),
      cancel: mock(),
      delete: mock().mockImplementation(async () => {
        events.push("first conversation deleted");
      }),
    };
    const firstEngine = {
      createConversation: mock().mockResolvedValue(firstConversation),
      delete: mock().mockImplementation(async () => {
        events.push("first engine deleted");
      }),
    };
    const secondConversation = {
      sendMessageStreaming: mock(),
      cancel: mock(),
      delete: mock().mockResolvedValue(undefined),
    };
    const secondEngine = {
      createConversation: mock().mockResolvedValue(secondConversation),
      delete: mock().mockResolvedValue(undefined),
    };
    const loadLiteRtLm = mock().mockResolvedValue(undefined);
    const Engine = {
      create: mock()
        .mockImplementationOnce(async () => {
          events.push("first engine created");
          return firstEngine;
        })
        .mockImplementationOnce(async () => {
          events.push("second engine created");
          return secondEngine;
        }),
    };
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });

    await provider.load(config);
    await provider.load(config);

    expect(events).toEqual([
      "first engine created",
      "first conversation deleted",
      "first engine deleted",
      "second engine created",
    ]);
  });

  it("deletes a stale load that completes after dispose", async () => {
    const wasmLoaded = createDeferred<void>();
    const conversation = {
      sendMessageStreaming: mock(createAsyncChunks([])),
      cancel: mock(),
      delete: mock().mockResolvedValue(undefined),
    };
    const engine = {
      createConversation: mock().mockResolvedValue(conversation),
      delete: mock().mockResolvedValue(undefined),
    };
    const loadLiteRtLm = mock().mockReturnValue(wasmLoaded.promise);
    const Engine = { create: mock().mockResolvedValue(engine) };
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });

    const load = provider.load(config);
    await waitForPendingPromises();
    await provider.dispose();
    wasmLoaded.resolve();
    await load;

    expect(conversation.delete).toHaveBeenCalledTimes(1);
    expect(engine.delete).toHaveBeenCalledTimes(1);
    await expect(
      provider.generate({ text: "Should fail", onToken: mock() }),
    ).rejects.toThrow("Load the browser Gemma provider before sending a message.");
  });

  it("deletes a stale load that completes after reload without publishing it", async () => {
    const firstConversation = {
      sendMessageStreaming: mock(
        createAsyncChunks([{ role: "assistant", content: "stale" }]),
      ),
      cancel: mock(),
      delete: mock().mockResolvedValue(undefined),
    };
    const firstConversationReady = createDeferred<typeof firstConversation>();
    const firstEngine = {
      createConversation: mock().mockReturnValue(firstConversationReady.promise),
      delete: mock().mockResolvedValue(undefined),
    };
    const secondConversation = {
      sendMessageStreaming: mock(
        createAsyncChunks([{ role: "assistant", content: "current" }]),
      ),
      cancel: mock(),
      delete: mock().mockResolvedValue(undefined),
    };
    const secondEngine = {
      createConversation: mock().mockResolvedValue(secondConversation),
      delete: mock().mockResolvedValue(undefined),
    };
    const loadLiteRtLm = mock().mockResolvedValue(undefined);
    const Engine = {
      create: mock()
        .mockResolvedValueOnce(firstEngine)
        .mockResolvedValueOnce(secondEngine),
    };
    const provider = createLiteRtBrowserProvider({ loadLiteRtLm, Engine });

    const firstLoad = provider.load(config);
    await waitForPendingPromises();
    expect(firstEngine.createConversation).toHaveBeenCalledTimes(1);

    await provider.load(config);
    firstConversationReady.resolve(firstConversation);
    await firstLoad;

    expect(firstConversation.delete).toHaveBeenCalledTimes(1);
    expect(firstEngine.delete).toHaveBeenCalledTimes(1);

    const result = await provider.generate({ text: "Current?", onToken: mock() });

    expect(result).toEqual({ text: "current" });
    expect(secondConversation.sendMessageStreaming).toHaveBeenCalledWith("Current?");
  });
});
