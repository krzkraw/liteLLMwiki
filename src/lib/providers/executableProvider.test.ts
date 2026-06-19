import { describe, expect, it, vi } from "vitest";
import { createExecutableProvider, formatExecutableModel } from "./executableProvider";

describe("formatExecutableModel", () => {
  it("formats model backend and max token suffixes", () => {
    expect(formatExecutableModel("gemma4-e2b", "gpu", 8192)).toBe(
      "gemma4-e2b,gpu,8192",
    );
    expect(formatExecutableModel("gemma4-e2b", "cpu", 8192)).toBe(
      "gemma4-e2b,cpu,8192",
    );
    expect(formatExecutableModel("gemma4-e2b", "npu", 8192)).toBe(
      "gemma4-e2b,npu,8192",
    );
    expect(formatExecutableModel("gemma4-e2b", "auto", 8192)).toBe("gemma4-e2b");
  });

  it("never formats cuda as a LiteRT-LM backend", () => {
    expect(formatExecutableModel("gemma4-e2b", "cuda", 8192)).toBe("gemma4-e2b");
  });
});

describe("createExecutableProvider", () => {
  it("streams OpenAI-compatible executable responses", async () => {
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(
          new TextEncoder().encode(
            'data: {"choices":[{"delta":{"content":"Native"}}]}\n\n',
          ),
        );
        controller.enqueue(new TextEncoder().encode("data: [DONE]\n\n"));
        controller.close();
      },
    });
    const fetchImpl = vi.fn().mockResolvedValue({ ok: true, body });
    const provider = createExecutableProvider({ fetchImpl });
    const tokens: Array<[string, string]> = [];

    await provider.load({
      endpoint: "http://127.0.0.1:9379/v1",
      modelId: "gemma4-e2b",
      backend: "gpu",
      maxNumTokens: 8192,
    });
    const result = await provider.generate({
      text: "Hello",
      onToken: (token, fullText) => tokens.push([token, fullText]),
    });

    expect(result.text).toBe("Native");
    expect(tokens).toEqual([["Native", "Native"]]);
    expect(fetchImpl).toHaveBeenCalledWith(
      "http://127.0.0.1:9379/v1/chat/completions",
      expect.objectContaining({
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "gemma4-e2b,gpu,8192",
          messages: [{ role: "user", content: "Hello" }],
          stream: true,
        }),
      }),
    );
  });

  it("uses a sidecar WebSocket API transport for executable text when available", async () => {
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(
          new TextEncoder().encode(
            'data: {"choices":[{"delta":{"content":"Native"}}]}\n\n',
          ),
        );
        controller.enqueue(new TextEncoder().encode("data: [DONE]\n\n"));
        controller.close();
      },
    });
    const fetchImpl = vi.fn();
    const transport = {
      request: vi.fn().mockResolvedValue({
        status: 200,
        headers: { "content-type": "text/event-stream" },
        body,
      }),
    };
    const provider = createExecutableProvider({ fetchImpl, transport });
    const tokens: Array<[string, string]> = [];

    await provider.load({
      endpoint: "http://127.0.0.1:9379/v1",
      modelId: "gemma4-e2b",
      backend: "gpu",
      maxNumTokens: 8192,
    });
    const result = await provider.generate({
      text: "Hello",
      onToken: (token, fullText) => tokens.push([token, fullText]),
    });

    expect(result.text).toBe("Native");
    expect(tokens).toEqual([["Native", "Native"]]);
    expect(fetchImpl).not.toHaveBeenCalled();
    expect(transport.request).toHaveBeenCalledWith(
      expect.objectContaining({
        method: "POST",
        path: "/v1/chat/completions",
        headers: { "Content-Type": "application/json" },
        body: {
          model: "gemma4-e2b,gpu,8192",
          messages: [{ role: "user", content: "Hello" }],
          stream: true,
        },
        signal: expect.any(AbortSignal),
      }),
    );
  });

  it("cancels non-2xx tunneled executable text response bodies", async () => {
    let cancelled = false;
    const body = new ReadableStream<Uint8Array>({
      cancel() {
        cancelled = true;
      },
    });
    const transport = {
      request: vi.fn().mockResolvedValue({
        status: 503,
        headers: { "content-type": "text/plain" },
        body,
      }),
    };
    const provider = createExecutableProvider({ fetchImpl: vi.fn(), transport });

    await provider.load({
      endpoint: "http://127.0.0.1:9379/v1",
      modelId: "gemma4-e2b",
      backend: "auto",
      maxNumTokens: 8192,
    });

    await expect(
      provider.generate({
        text: "Hello",
        onToken: vi.fn(),
      }),
    ).rejects.toThrow("Executable provider request failed: 503");
    expect(cancelled).toBe(true);
  });

  it("sends a configured system prompt before executable user messages", async () => {
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new TextEncoder().encode("data: [DONE]\n\n"));
        controller.close();
      },
    });
    const fetchImpl = vi.fn().mockResolvedValue({ ok: true, body });
    const provider = createExecutableProvider({ fetchImpl });

    await provider.load({
      endpoint: "http://127.0.0.1:9379/v1",
      modelId: "gemma4-e2b",
      backend: "auto",
      maxNumTokens: 8192,
      systemPrompt: "You are concise.",
    });
    await provider.generate({
      text: "Hello",
      onToken: vi.fn(),
    });

    expect(fetchImpl).toHaveBeenCalledWith(
      "http://127.0.0.1:9379/v1/chat/completions",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          model: "gemma4-e2b",
          messages: [
            { role: "system", content: "You are concise." },
            { role: "user", content: "Hello" },
          ],
          stream: true,
        }),
      }),
    );
  });

  it("sends configured executable sampling options", async () => {
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new TextEncoder().encode("data: [DONE]\n\n"));
        controller.close();
      },
    });
    const fetchImpl = vi.fn().mockResolvedValue({ ok: true, body });
    const provider = createExecutableProvider({ fetchImpl });

    await provider.load({
      endpoint: "http://127.0.0.1:9379/v1",
      modelId: "gemma4-e2b",
      backend: "auto",
	      maxNumTokens: 8192,
	      maxTokens: 256,
	      temperature: 0.25,
	      topP: 0.8,
	      seed: 42,
    });
    await provider.generate({
      text: "Hello",
      onToken: vi.fn(),
    });

    expect(fetchImpl).toHaveBeenCalledWith(
      "http://127.0.0.1:9379/v1/chat/completions",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          model: "gemma4-e2b",
          messages: [{ role: "user", content: "Hello" }],
	          stream: true,
	          max_tokens: 256,
	          temperature: 0.25,
	          top_p: 0.8,
          seed: 42,
        }),
      }),
    );
  });

  it("generates folder summary text through the executable text path", async () => {
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(
          new TextEncoder().encode(
            'data: {"choices":[{"delta":{"content":"Folder"}}]}\n\n',
          ),
        );
        controller.enqueue(
          new TextEncoder().encode(
            'data: {"choices":[{"delta":{"content":" summary"}}]}\n\n',
          ),
        );
        controller.enqueue(new TextEncoder().encode("data: [DONE]\n\n"));
        controller.close();
      },
    });
    const fetchImpl = vi.fn().mockResolvedValue({ ok: true, body });
    const provider = createExecutableProvider({ fetchImpl });

    await provider.load({
      endpoint: "http://127.0.0.1:9379/v1",
      modelId: "gemma4-e2b",
      backend: "cpu",
      maxNumTokens: 4096,
    });
    const text = await provider.generateText("Summarize folder");

    expect(text).toBe("Folder summary");
    expect(fetchImpl).toHaveBeenCalledWith(
      "http://127.0.0.1:9379/v1/chat/completions",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          model: "gemma4-e2b,cpu,4096",
          messages: [{ role: "user", content: "Summarize folder" }],
          stream: true,
        }),
      }),
    );
  });

  it("passes abort signals to executable folder summary generation", async () => {
    const fetchImpl = vi.fn(
      (_url: string | URL | Request, init?: RequestInit) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener("abort", () => {
            reject(new DOMException("Aborted", "AbortError"));
          });
        }),
    );
    const provider = createExecutableProvider({ fetchImpl });
    const controller = new AbortController();

    await provider.load({
      endpoint: "http://127.0.0.1:9379/v1",
      modelId: "gemma4-e2b",
      backend: "auto",
      maxNumTokens: 8192,
    });
    const generation = provider.generateText("Summarize folder", controller.signal);

    expect(fetchImpl).toHaveBeenCalledWith(
      "http://127.0.0.1:9379/v1/chat/completions",
      expect.objectContaining({
        signal: expect.any(AbortSignal),
      }),
    );
    const fetchInit = fetchImpl.mock.calls[0][1] as RequestInit;
    const internalSignal = fetchInit.signal as AbortSignal;

    expect(internalSignal.aborted).toBe(false);
    controller.abort();
    expect(internalSignal.aborted).toBe(true);
    await expect(generation).rejects.toMatchObject({ name: "AbortError" });
  });

  it("cancels concurrent executable text requests together", async () => {
    const pendingRequests: Array<{
      signal: AbortSignal;
      reject: (reason?: unknown) => void;
    }> = [];
    const fetchImpl = vi.fn(
      (_url: string | URL | Request, init?: RequestInit) =>
        new Promise<Response>((_resolve, reject) => {
          const signal = init?.signal as AbortSignal;
          pendingRequests.push({ signal, reject });
          signal.addEventListener("abort", () => {
            reject(new DOMException("Aborted", "AbortError"));
          });
        }),
    );
    const provider = createExecutableProvider({ fetchImpl });

    await provider.load({
      endpoint: "http://127.0.0.1:9379/v1",
      modelId: "gemma4-e2b",
      backend: "auto",
      maxNumTokens: 8192,
    });
    const folderGeneration = provider.generateText("Summarize folder");
    const chatGeneration = provider.generate({
      text: "Chat",
      onToken: vi.fn(),
    });
    const observedGenerations = Promise.allSettled([
      folderGeneration,
      chatGeneration,
    ]);

    expect(pendingRequests).toHaveLength(2);
    provider.cancel();

    expect(pendingRequests[0].signal.aborted).toBe(true);
    expect(pendingRequests[1].signal.aborted).toBe(true);
    for (const request of pendingRequests) {
      request.reject(new DOMException("Aborted", "AbortError"));
    }
    await observedGenerations;
  });

  it("passes abort signals to fetch", async () => {
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new TextEncoder().encode("data: [DONE]\n\n"));
        controller.close();
      },
    });
    const fetchImpl = vi.fn().mockResolvedValue({ ok: true, body });
    const provider = createExecutableProvider({ fetchImpl });
    const controller = new AbortController();

    await provider.load({
      endpoint: "http://127.0.0.1:9379/v1/",
      modelId: "gemma4-e2b",
      backend: "cuda",
      maxNumTokens: 8192,
    });
    const generation = provider.generate({
      text: "Hello",
      onToken: vi.fn(),
      signal: controller.signal,
    });

    expect(fetchImpl).toHaveBeenCalledWith(
      "http://127.0.0.1:9379/v1/chat/completions",
      expect.objectContaining({
        signal: expect.any(AbortSignal),
        body: JSON.stringify({
          model: "gemma4-e2b",
          messages: [{ role: "user", content: "Hello" }],
          stream: true,
        }),
      }),
    );
    const fetchInit = fetchImpl.mock.calls[0][1] as RequestInit;
    const internalSignal = fetchInit.signal as AbortSignal;

    expect(internalSignal.aborted).toBe(false);
    controller.abort();
    expect(internalSignal.aborted).toBe(true);
    await generation;
  });

  it("allows provider cancel to abort the active fetch with caller signal present", async () => {
    const fetchImpl = vi.fn(
      (_url: string | URL | Request, init?: RequestInit) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener("abort", () => {
            reject(new DOMException("Aborted", "AbortError"));
          });
        }),
    );
    const provider = createExecutableProvider({ fetchImpl });
    const controller = new AbortController();

    await provider.load({
      endpoint: "http://127.0.0.1:9379/v1",
      modelId: "gemma4-e2b",
      backend: "auto",
      maxNumTokens: 8192,
    });
    const generation = provider.generate({
      text: "Hello",
      onToken: vi.fn(),
      signal: controller.signal,
    });

    provider.cancel();

    await expect(generation).rejects.toMatchObject({ name: "AbortError" });
  });

  it("routes attached prompts to the sidecar multimodal endpoint", async () => {
    const fetchImpl = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ text: "image summary" }),
    });
    const provider = createExecutableProvider({ fetchImpl });
    const onToken = vi.fn();

    await provider.load({
      endpoint: "http://127.0.0.1:9379/v1",
      modelId: "gemma4-e2b",
      backend: "gpu",
      maxNumTokens: 8192,
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
    const result = await provider.generate({
      text: "Describe this image",
      attachments: [
        {
          id: "sample",
          name: "sample.png",
          mimeType: "image/png",
          file: new File(["image bytes"], "sample.png", { type: "image/png" }),
        },
      ],
      onToken,
    });

    expect(result.text).toBe("image summary");
    expect(onToken).toHaveBeenCalledWith("image summary", "image summary");
    expect(fetchImpl).toHaveBeenCalledWith(
      "http://127.0.0.1:9379/sidecar/v1/multimodal",
      expect.objectContaining({
        method: "POST",
        headers: { "Content-Type": "application/json" },
      }),
    );

    const requestBody = JSON.parse(
      (fetchImpl.mock.calls[0][1] as RequestInit).body as string,
    );
    expect(requestBody).toEqual({
      prompt: "Describe this image",
      modelId: "gemma4-e2b",
      backend: "gpu",
      visionBackend: "cpu",
      audioBackend: "gpu",
      maxNumTokens: 8192,
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
      attachments: [
        {
          name: "sample.png",
          mimeType: "image/png",
          dataBase64: "aW1hZ2UgYnl0ZXM=",
        },
      ],
    });
  });

  it("uses a sidecar WebSocket API transport for multimodal executable prompts", async () => {
    const fetchImpl = vi.fn();
    const transport = {
      request: vi.fn().mockResolvedValue({
        status: 200,
        headers: { "content-type": "application/json" },
        body: new ReadableStream<Uint8Array>(),
        json: async () => ({ text: "image summary" }),
      }),
    };
    const provider = createExecutableProvider({ fetchImpl, transport });
    const onToken = vi.fn();

    await provider.load({
      endpoint: "http://127.0.0.1:9379/v1",
      modelId: "gemma4-e2b",
      backend: "gpu",
      maxNumTokens: 8192,
      visionBackend: "cpu",
      audioBackend: "gpu",
      huggingfaceToken: "hf_secret",
    });
    const result = await provider.generate({
      text: "Describe this image",
      attachments: [
        {
          id: "sample",
          name: "sample.png",
          mimeType: "image/png",
          file: new File(["image bytes"], "sample.png", { type: "image/png" }),
        },
      ],
      onToken,
    });

    expect(result.text).toBe("image summary");
    expect(onToken).toHaveBeenCalledWith("image summary", "image summary");
    expect(fetchImpl).not.toHaveBeenCalled();
    expect(transport.request).toHaveBeenCalledWith(
      expect.objectContaining({
        method: "POST",
        path: "/sidecar/v1/multimodal",
        headers: { "Content-Type": "application/json" },
        body: expect.objectContaining({
          prompt: "Describe this image",
          modelId: "gemma4-e2b",
          backend: "gpu",
          visionBackend: "cpu",
          audioBackend: "gpu",
          maxNumTokens: 8192,
          huggingfaceToken: "hf_secret",
          attachments: [
            {
              name: "sample.png",
              mimeType: "image/png",
              dataBase64: "aW1hZ2UgYnl0ZXM=",
            },
          ],
        }),
        signal: expect.any(AbortSignal),
      }),
    );
  });

  it("cancels non-2xx tunneled multimodal response bodies", async () => {
    let cancelled = false;
    const body = new ReadableStream<Uint8Array>({
      cancel() {
        cancelled = true;
      },
    });
    const transport = {
      request: vi.fn().mockResolvedValue({
        status: 502,
        headers: { "content-type": "text/plain" },
        body,
      }),
    };
    const provider = createExecutableProvider({ fetchImpl: vi.fn(), transport });

    await provider.load({
      endpoint: "http://127.0.0.1:9379/v1",
      modelId: "gemma4-e2b",
      backend: "gpu",
      maxNumTokens: 8192,
    });

    await expect(
      provider.generate({
        text: "Describe this image",
        attachments: [
          {
            id: "sample",
            name: "sample.png",
            mimeType: "image/png",
            file: new File(["image bytes"], "sample.png", { type: "image/png" }),
          },
        ],
        onToken: vi.fn(),
      }),
    ).rejects.toThrow("Executable multimodal request failed: 502");
    expect(cancelled).toBe(true);
  });

  it("preserves zero-valued multimodal sampling options", async () => {
    const fetchImpl = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ text: "image summary" }),
    });
    const provider = createExecutableProvider({ fetchImpl });

    await provider.load({
      endpoint: "http://127.0.0.1:9379/v1",
      modelId: "gemma4-e2b",
      backend: "auto",
      maxNumTokens: 8192,
      topP: 0,
      temperature: 0,
    });
    await provider.generate({
      text: "Describe this image",
      attachments: [
        {
          id: "sample",
          name: "sample.png",
          mimeType: "image/png",
          file: new File(["image bytes"], "sample.png", { type: "image/png" }),
        },
      ],
      onToken: vi.fn(),
    });

    const requestBody = JSON.parse(
      (fetchImpl.mock.calls[0][1] as RequestInit).body as string,
    );
    expect(requestBody).toMatchObject({
      topP: 0,
      temperature: 0,
    });
  });
});
