import type { ChatGenerateRequest, ChatGenerateResult } from "../chatProvider";
import { toMultimodalAttachmentRequests } from "../attachments";
import type { NativeBackend } from "./backendStatus";
import { createSidecarEndpoint, normalizeExecutableEndpoint } from "./endpoint";
import { collectOpenAiSseText } from "./openaiSse";
import type { SidecarApiRequest, SidecarApiResponse } from "./sidecarControlClient";

type FetchResponse = Pick<Response, "body" | "json" | "ok"> &
  Partial<Pick<Response, "status">>;
type FetchImpl = (
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<FetchResponse>;

export interface ExecutableProviderTransport {
  request(request: SidecarApiRequest): Promise<SidecarApiResponse>;
}

export interface ExecutableProviderConfig {
  endpoint: string;
  modelId: string;
  backend: NativeBackend;
  maxNumTokens: number;
  maxTokens?: number;
  systemPrompt?: string;
  temperature?: number;
  topK?: number;
  topP?: number;
  seed?: number;
  visionBackend?: string;
  audioBackend?: string;
  preset?: string;
  noTemplate?: boolean;
  filterChannelContentFromKvCache?: boolean;
  enableSpeculativeDecoding?: string;
  cache?: string;
  verbose?: boolean;
  fromHuggingFaceRepo?: string;
  huggingfaceToken?: string;
}

export interface ExecutableProvider {
  readonly id: string;
  load(config: ExecutableProviderConfig): Promise<void>;
  generate(request: ChatGenerateRequest): Promise<ChatGenerateResult>;
  generateText(prompt: string, signal?: AbortSignal): Promise<string>;
  cancel(): void;
  dispose(): Promise<void>;
}

export function formatExecutableModel(
  modelId: string,
  backend: NativeBackend,
  maxTokens: number,
): string {
  if (backend === "auto" || backend === "cuda") {
    return modelId;
  }

  return `${modelId},${backend},${maxTokens}`;
}

function createAbortError(): Error {
  if (typeof DOMException !== "undefined") {
    return new DOMException("Generation aborted.", "AbortError");
  }

  return new Error("Generation aborted.");
}

function multimodalCoreBackend(backend: NativeBackend): string | undefined {
  return backend === "cpu" || backend === "gpu" || backend === "npu"
    ? backend
    : undefined;
}

function multimodalMediaBackend(backend: string | undefined): string | undefined {
  return backend === "cpu" || backend === "gpu" ? backend : undefined;
}

function resolveMultimodalMediaBackend(
  mediaBackend: string | undefined,
  coreBackend: NativeBackend,
): string | undefined {
  return multimodalMediaBackend(
    mediaBackend && mediaBackend !== "auto" ? mediaBackend : coreBackend,
  );
}

function optionalNumberField(name: string, value: number | undefined) {
  return value !== undefined && Number.isFinite(value) && value > 0
    ? { [name]: value }
    : {};
}

function optionalFiniteNumberField(name: string, value: number | undefined) {
  return value !== undefined && Number.isFinite(value) ? { [name]: value } : {};
}

function optionalStringField(name: string, value: string | undefined) {
  const trimmedValue = value?.trim();
  return trimmedValue ? { [name]: trimmedValue } : {};
}

function createOpenAiMessages(systemPrompt: string | undefined, text: string) {
  const trimmedSystemPrompt = systemPrompt?.trim();
  const messages: Array<{ role: "system" | "user"; content: string }> = [];

  if (trimmedSystemPrompt) {
    messages.push({ role: "system", content: trimmedSystemPrompt });
  }

  messages.push({ role: "user", content: text });

  return messages;
}

function createOpenAiRequestBody(
  activeConfig: ExecutableProviderConfig,
  text: string,
) {
  return {
    model: formatExecutableModel(
      activeConfig.modelId,
      activeConfig.backend,
      activeConfig.maxNumTokens,
    ),
    messages: createOpenAiMessages(activeConfig.systemPrompt, text),
    stream: true,
    ...optionalNumberField("max_tokens", activeConfig.maxTokens),
    ...(activeConfig.temperature !== undefined
      ? { temperature: activeConfig.temperature }
      : {}),
    ...(activeConfig.topP !== undefined ? { top_p: activeConfig.topP } : {}),
    ...(activeConfig.seed !== undefined && activeConfig.seed > 0
      ? { seed: activeConfig.seed }
      : {}),
  };
}

export function createExecutableProvider({
  fetchImpl = fetch,
  transport,
}: {
  fetchImpl?: FetchImpl;
  transport?: ExecutableProviderTransport;
} = {}): ExecutableProvider {
  let config: ExecutableProviderConfig | null = null;
  const activeControllers = new Set<AbortController>();

  function createRequestController() {
    const controller = new AbortController();
    activeControllers.add(controller);

    return controller;
  }

  function releaseRequestController(controller: AbortController) {
    activeControllers.delete(controller);
  }

  function isSuccessfulStatus(status: number): boolean {
    return status >= 200 && status < 300;
  }

  async function cancelResponseBody(
    body: ReadableStream<Uint8Array> | null | undefined,
  ) {
    try {
      await body?.cancel();
    } catch {
      // Best-effort cleanup before surfacing the original request failure.
    }
  }

  async function requestOpenAiChat(
    activeConfig: ExecutableProviderConfig,
    text: string,
    signal: AbortSignal,
  ): Promise<ReadableStream<Uint8Array>> {
    const body = createOpenAiRequestBody(activeConfig, text);

    if (transport) {
      const response = await transport.request({
        method: "POST",
        path: "/v1/chat/completions",
        headers: { "Content-Type": "application/json" },
        body,
        signal,
      });

      if (!isSuccessfulStatus(response.status)) {
        await cancelResponseBody(response.body);
        throw new Error(`Executable provider request failed: ${response.status}`);
      }

      return response.body;
    }

    const response = await fetchImpl(`${activeConfig.endpoint}/chat/completions`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
      signal,
    });

    if (!response.ok) {
      await cancelResponseBody(response.body);
      throw new Error(`Executable provider request failed: ${response.status}`);
    }

    if (!response.body) {
      throw new Error("Executable provider response did not include a stream.");
    }

    return response.body;
  }

  async function createMultimodalRequestBody(
    activeConfig: ExecutableProviderConfig,
    request: ChatGenerateRequest,
  ) {
    const attachmentRequests = await toMultimodalAttachmentRequests(
      request.attachments ?? [],
    );

    return {
      prompt: request.text,
      modelId: activeConfig.modelId,
      backend: multimodalCoreBackend(activeConfig.backend),
      visionBackend: resolveMultimodalMediaBackend(
        activeConfig.visionBackend,
        activeConfig.backend,
      ),
      audioBackend: resolveMultimodalMediaBackend(
        activeConfig.audioBackend,
        activeConfig.backend,
      ),
      ...optionalNumberField("maxNumTokens", activeConfig.maxNumTokens),
      ...optionalNumberField("topK", activeConfig.topK),
      ...optionalFiniteNumberField("topP", activeConfig.topP),
      ...(activeConfig.temperature !== undefined
        ? { temperature: activeConfig.temperature }
        : {}),
      ...(activeConfig.seed !== undefined && activeConfig.seed > 0
        ? { seed: activeConfig.seed }
        : {}),
      ...optionalStringField("preset", activeConfig.preset),
      ...(activeConfig.noTemplate ? { noTemplate: true } : {}),
      ...(activeConfig.filterChannelContentFromKvCache
        ? { filterChannelContentFromKvCache: true }
        : {}),
      ...optionalStringField(
        "enableSpeculativeDecoding",
        activeConfig.enableSpeculativeDecoding === "auto"
          ? undefined
          : activeConfig.enableSpeculativeDecoding,
      ),
      ...optionalStringField("cache", activeConfig.cache),
      ...(activeConfig.verbose ? { verbose: true } : {}),
      ...optionalStringField(
        "fromHuggingFaceRepo",
        activeConfig.fromHuggingFaceRepo,
      ),
      ...optionalStringField("huggingfaceToken", activeConfig.huggingfaceToken),
      attachments: attachmentRequests,
    };
  }

  async function requestMultimodal(
    activeConfig: ExecutableProviderConfig,
    request: ChatGenerateRequest,
    signal: AbortSignal,
  ): Promise<Partial<ChatGenerateResult>> {
    const body = await createMultimodalRequestBody(activeConfig, request);

    if (transport) {
      const response = await transport.request({
        method: "POST",
        path: "/sidecar/v1/multimodal",
        headers: { "Content-Type": "application/json" },
        body,
        signal,
      });

      if (!isSuccessfulStatus(response.status)) {
        await cancelResponseBody(response.body);
        throw new Error(
          `Executable multimodal request failed: ${response.status}`,
        );
      }

      return (await response.json()) as Partial<ChatGenerateResult>;
    }

    const response = await fetchImpl(
      createSidecarEndpoint(activeConfig.endpoint, "/sidecar/v1/multimodal"),
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
        signal,
      },
    );

    if (!response.ok) {
      await cancelResponseBody(response.body);
      throw new Error(`Executable multimodal request failed: ${response.status}`);
    }

    return (await response.json()) as Partial<ChatGenerateResult>;
  }

  async function generateOpenAiText(
    text: string,
    signal: AbortSignal | undefined,
    onToken: (token: string, fullText: string) => void,
  ): Promise<string> {
    const activeConfig = config;

    if (!activeConfig) {
      throw new Error("Load the executable Gemma provider before sending a message.");
    }

    if (signal?.aborted) {
      throw createAbortError();
    }

    const controller = createRequestController();
    const abort = () => controller.abort();
    signal?.addEventListener("abort", abort);

    try {
      let fullText = "";
      const stream = await requestOpenAiChat(activeConfig, text, controller.signal);
      return await collectOpenAiSseText(stream, (token) => {
        fullText += token;
        onToken(token, fullText);
      });
    } finally {
      signal?.removeEventListener("abort", abort);
      releaseRequestController(controller);
    }
  }

  return {
    id: "executable-gemma4-e2b",
    async load(nextConfig: ExecutableProviderConfig) {
      config = {
        ...nextConfig,
        endpoint: normalizeExecutableEndpoint(nextConfig.endpoint),
      };
    },
    async generate(request: ChatGenerateRequest): Promise<ChatGenerateResult> {
      if (!config) {
        throw new Error("Load the executable Gemma provider before sending a message.");
      }

      if (request.signal?.aborted) {
        throw createAbortError();
      }

      if (!request.attachments?.length) {
        const text = await generateOpenAiText(
          request.text,
          request.signal,
          request.onToken,
        );

        return { text };
      }

      const controller = createRequestController();
      const abort = () => controller.abort();
      request.signal?.addEventListener("abort", abort);

      try {
        const body = await requestMultimodal(config, request, controller.signal);

        if (typeof body.text !== "string") {
          throw new Error("Executable multimodal response did not include text.");
        }

        request.onToken(body.text, body.text);

        return { text: body.text };
      } finally {
        request.signal?.removeEventListener("abort", abort);
        releaseRequestController(controller);
      }
    },
    generateText(prompt: string, signal?: AbortSignal): Promise<string> {
      return generateOpenAiText(prompt, signal, () => undefined);
    },
    cancel() {
      for (const controller of activeControllers) {
        controller.abort();
      }
    },
    async dispose() {
      for (const controller of activeControllers) {
        controller.abort();
      }
      config = null;
      activeControllers.clear();
    },
  };
}
