import {
  Backend,
  Engine,
  loadLiteRtLm,
  SamplerType,
  type ConversationConfig,
  type EngineSettings,
  type Message,
} from "@litert-lm/core";
import type {
  BrowserProviderConfig,
  ChatGenerateRequest,
  ChatGenerateResult,
  ChatProvider,
} from "./chatProvider";

type LiteRtMessage = Partial<Message>;

type LiteRtConversation = {
  sendMessageStreaming(
    message: string,
  ): AsyncIterable<LiteRtMessage> | ReadableStream<LiteRtMessage>;
  cancel(): void;
  delete(): Promise<void> | void;
};

type LiteRtEngine = {
  createConversation(config?: ConversationConfig): Promise<LiteRtConversation>;
  delete(): Promise<void> | void;
};

type LiteRtDeps = {
  loadLiteRtLm: (path: string) => Promise<unknown>;
  Engine: {
    create(settings: EngineSettings): Promise<LiteRtEngine>;
  };
};

type WasmLoadState = {
  loadLiteRtLm: LiteRtDeps["loadLiteRtLm"];
  path: string;
  promise: Promise<unknown>;
  status: "pending" | "loaded";
};

let wasmLoadState: WasmLoadState | null = null;

function enumValue<
  Values extends Record<string, string | number>,
  Key extends string | undefined,
>(values: Values, key: Key): Values[keyof Values] | undefined {
  if (!key || !(key in values)) {
    return undefined;
  }

  return values[key as keyof Values];
}

function ensureWasmLoaded(config: BrowserProviderConfig, deps: LiteRtDeps) {
  if (wasmLoadState && wasmLoadState.loadLiteRtLm !== deps.loadLiteRtLm) {
    wasmLoadState = null;
  }

  if (wasmLoadState) {
    if (wasmLoadState.path !== config.wasmPath) {
      const status = wasmLoadState.status === "loaded" ? "loaded" : "loading";
      throw new Error(
        `LiteRT-LM WASM already ${status} from "${wasmLoadState.path}" and cannot be reloaded from "${config.wasmPath}".`,
      );
    }

    return wasmLoadState.promise;
  }

  const state: WasmLoadState = {
    loadLiteRtLm: deps.loadLiteRtLm,
    path: config.wasmPath,
    promise: Promise.resolve(),
    status: "pending",
  };

  state.promise = deps
    .loadLiteRtLm(config.wasmPath)
    .then((result) => {
      if (wasmLoadState === state) {
        state.status = "loaded";
      }

      return result;
    })
    .catch((error: unknown) => {
      if (wasmLoadState === state) {
        wasmLoadState = null;
      }

      throw error;
    });

  wasmLoadState = state;

  return state.promise;
}

async function* readLiteRtStream(
  stream: AsyncIterable<LiteRtMessage> | ReadableStream<LiteRtMessage>,
): AsyncIterable<LiteRtMessage> {
  if (Symbol.asyncIterator in stream) {
    yield* stream;
    return;
  }

  const reader = stream.getReader();

  try {
    while (true) {
      const result = await reader.read();
      if (result.done) {
        return;
      }
      yield result.value;
    }
  } finally {
    reader.releaseLock();
  }
}

export function extractTextFromLiteRtMessage(message: LiteRtMessage): string {
  if (typeof message.content === "string") {
    return message.content;
  }

  if (!Array.isArray(message.content)) {
    return "";
  }

  return message.content
    .map((item) => (item.type === "text" && typeof item.text === "string" ? item.text : ""))
    .join("");
}

function createAbortError(): Error {
  if (typeof DOMException !== "undefined") {
    return new DOMException("Generation aborted.", "AbortError");
  }

  return new Error("Generation aborted.");
}

function createConversationConfig(config: BrowserProviderConfig): ConversationConfig {
  const sessionConfig: NonNullable<ConversationConfig["sessionConfig"]> = {
    maxOutputTokens: config.maxOutputTokens,
  };
  const samplerBackend = enumValue(Backend, config.samplerBackend);
  const samplerType = enumValue(SamplerType, config.samplerType);

  if (config.stopTokenIds?.some((row) => row.length > 0)) {
    sessionConfig.stopTokenIds = config.stopTokenIds;
  }

  if (config.startTokenId !== undefined && config.startTokenId > 0) {
    sessionConfig.startTokenId = config.startTokenId;
  }

  if (
    config.numOutputCandidates !== undefined &&
    config.numOutputCandidates > 0
  ) {
    sessionConfig.numOutputCandidates = config.numOutputCandidates;
  }

  if (
    samplerType !== undefined ||
    config.temperature !== undefined ||
    config.topK !== undefined ||
    config.topP !== undefined ||
    config.seed !== undefined
  ) {
    sessionConfig.samplerParams = {};

    if (samplerType !== undefined) {
      sessionConfig.samplerParams.type = samplerType;
    }

    if (config.temperature !== undefined) {
      sessionConfig.samplerParams.temperature = config.temperature;
    }

    if (config.topK !== undefined) {
      sessionConfig.samplerParams.k = config.topK;
    }

    if (config.topP !== undefined) {
      sessionConfig.samplerParams.p = config.topP;
    }

    if (config.seed !== undefined && config.seed > 0) {
      sessionConfig.samplerParams.seed = config.seed;
    }
  }

  if (samplerBackend !== undefined) {
    sessionConfig.samplerBackend = samplerBackend;
  }

  if (config.applyPromptTemplateInSession !== undefined) {
    sessionConfig.applyPromptTemplateInSession =
      config.applyPromptTemplateInSession;
  }

  if (config.useExternalSampler !== undefined) {
    sessionConfig.useExternalSampler = config.useExternalSampler;
  }

  const conversationConfig: ConversationConfig = { sessionConfig };

  if (config.systemPrompt) {
    conversationConfig.preface = {
      messages: [{ role: "system", content: config.systemPrompt }],
    };
  }

  if (config.enableConstrainedDecoding !== undefined) {
    conversationConfig.enableConstrainedDecoding =
      config.enableConstrainedDecoding;
  }

  if (config.prefillPrefaceOnInit !== undefined) {
    conversationConfig.prefillPrefaceOnInit = config.prefillPrefaceOnInit;
  }

  if (config.filterChannelContentFromKvCache !== undefined) {
    conversationConfig.filterChannelContentFromKvCache =
      config.filterChannelContentFromKvCache;
  }

  return conversationConfig;
}

function createEngineSettings(config: BrowserProviderConfig): EngineSettings {
  const backend = enumValue(Backend, config.engineBackend);
  const samplerBackend = enumValue(Backend, config.samplerBackend);

  return {
    model: config.model.value,
    ...(backend !== undefined ? { backend } : {}),
    mainExecutorSettings: {
      maxNumTokens: config.maxNumTokens,
      ...(samplerBackend !== undefined ? { samplerBackend } : {}),
    },
  };
}

async function deleteResources(
  resourceConversation: LiteRtConversation | null,
  resourceEngine: LiteRtEngine | null,
) {
  if (resourceConversation) {
    await resourceConversation.delete();
  }

  if (resourceEngine) {
    await resourceEngine.delete();
  }
}

export function createLiteRtBrowserProvider(
  deps: LiteRtDeps = { loadLiteRtLm, Engine },
): ChatProvider {
  let engine: LiteRtEngine | null = null;
  let conversation: LiteRtConversation | null = null;
  const textConversations = new Set<LiteRtConversation>();
  let activeConversationConfig: ConversationConfig | undefined;
  let generation = 0;

  function cancelTextConversations() {
    for (const textConversation of textConversations) {
      textConversation.cancel();
    }
    textConversations.clear();
  }

  async function dispose() {
    generation += 1;
    const previousConversation = conversation;
    const previousEngine = engine;
    conversation = null;
    engine = null;
    activeConversationConfig = undefined;

    cancelTextConversations();
    await deleteResources(previousConversation, previousEngine);
  }

  return {
    id: "browser-gemma4-e2b",
    async load(config: BrowserProviderConfig) {
      const wasmLoad = ensureWasmLoaded(config, deps);
      generation += 1;
      const loadGeneration = generation;

      const previousConversation = conversation;
      const previousEngine = engine;
      conversation = null;
      engine = null;
      activeConversationConfig = undefined;

      cancelTextConversations();
      await deleteResources(previousConversation, previousEngine);

      let nextEngine: LiteRtEngine | null = null;
      let nextConversation: LiteRtConversation | null = null;
      const nextConversationConfig = createConversationConfig(config);

      try {
        await wasmLoad;

        nextEngine = await deps.Engine.create(createEngineSettings(config));
        nextConversation = await nextEngine.createConversation(nextConversationConfig);

        if (generation !== loadGeneration) {
          return;
        }

        engine = nextEngine;
        conversation = nextConversation;
        activeConversationConfig = nextConversationConfig;
        nextEngine = null;
        nextConversation = null;
      } finally {
        await deleteResources(nextConversation, nextEngine);
      }
    },
    async generate(request: ChatGenerateRequest): Promise<ChatGenerateResult> {
      const activeConversation = conversation;

      if (!activeConversation) {
        throw new Error("Load the browser Gemma provider before sending a message.");
      }

      if (request.attachments?.length) {
        throw new Error(
          "Browser Gemma is text-only; use the executable provider for image or audio attachments.",
        );
      }

      if (request.signal?.aborted) {
        activeConversation.cancel();
        throw createAbortError();
      }

      let fullText = "";
      const abort = () => {
        activeConversation.cancel();
      };

      request.signal?.addEventListener("abort", abort);

      try {
        for await (const chunk of readLiteRtStream(
          activeConversation.sendMessageStreaming(request.text),
        )) {
          if (request.signal?.aborted) {
            throw createAbortError();
          }

          const token = extractTextFromLiteRtMessage(chunk);
          fullText += token;
          request.onToken(token, fullText);
        }

        if (request.signal?.aborted) {
          throw createAbortError();
        }

        return { text: fullText };
      } finally {
        request.signal?.removeEventListener("abort", abort);
      }
    },
    async generateText(prompt: string, signal?: AbortSignal): Promise<string> {
      const activeEngine = engine;
      const textGeneration = generation;

      if (!activeEngine) {
        throw new Error("Load the browser Gemma provider before summarizing a folder.");
      }

      let summaryConversation: LiteRtConversation | null = null;

      try {
        summaryConversation = await activeEngine.createConversation(
          activeConversationConfig,
        );

        if (generation !== textGeneration) {
          summaryConversation.cancel();
          throw createAbortError();
        }

        textConversations.add(summaryConversation);

        if (signal?.aborted) {
          summaryConversation.cancel();
          throw createAbortError();
        }

        let fullText = "";
        const abort = () => {
          summaryConversation?.cancel();
        };

        signal?.addEventListener("abort", abort);

        try {
          for await (const chunk of readLiteRtStream(
            summaryConversation.sendMessageStreaming(prompt),
          )) {
            if (signal?.aborted) {
              throw createAbortError();
            }

            fullText += extractTextFromLiteRtMessage(chunk);
          }

          if (signal?.aborted) {
            throw createAbortError();
          }

          return fullText;
        } finally {
          signal?.removeEventListener("abort", abort);
        }
      } finally {
        if (summaryConversation) {
          textConversations.delete(summaryConversation);
          await summaryConversation.delete();
        }
      }
    },
    cancel() {
      conversation?.cancel();
      for (const textConversation of textConversations) {
        textConversation.cancel();
      }
    },
    dispose,
  };
}
