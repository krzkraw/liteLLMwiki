import { describe, expect, it, mock } from "bun:test";
import {
  createSidecarControlClient,
  createSidecarWebSocketUrl,
  type SidecarControlEvent,
  type SidecarRuntimeConfig,
} from "./sidecarControlClient";

class FakeWebSocket {
  static instances: FakeWebSocket[] = [];

  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: (() => void) | null = null;
  sent: string[] = [];

  constructor(readonly url: string) {
    FakeWebSocket.instances.push(this);
  }

  send(message: string) {
    this.sent.push(message);
  }

  close() {
    this.onclose?.();
  }

  open() {
    this.onopen?.();
  }

  receive(event: unknown) {
    this.onmessage?.({ data: JSON.stringify(event) });
  }
}

function base64ToText(value: string): string {
  return new TextDecoder().decode(
    Uint8Array.from(atob(value), (character) => character.charCodeAt(0)),
  );
}

function textToBase64(value: string): string {
  return btoa(value);
}

describe("createSidecarWebSocketUrl", () => {
  it("normalizes sidecar HTTP endpoints to the WebSocket control endpoint", () => {
    expect(createSidecarWebSocketUrl("http://127.0.0.1:9379/v1")).toBe(
      "ws://127.0.0.1:9379/sidecar/v1/ws",
    );
    expect(createSidecarWebSocketUrl("https://example.test/litert/v1/models")).toBe(
      "wss://example.test/sidecar/v1/ws",
    );
  });
});

describe("createSidecarControlClient", () => {
  it("requests status and log subscription when the WebSocket opens", () => {
    FakeWebSocket.instances = [];
    const client = createSidecarControlClient({
      endpoint: "http://127.0.0.1:9379/v1",
      WebSocketImpl: FakeWebSocket,
      onEvent: () => undefined,
    });

    client.connect();
    const socket = FakeWebSocket.instances[0];
    socket.open();

    expect(socket.sent.map((message) => JSON.parse(message))).toEqual([
      { type: "status.get" },
      { type: "logs.subscribe" },
    ]);
  });

  it("sends runtime control messages and emits parsed status/log events", () => {
    FakeWebSocket.instances = [];
    const events: SidecarControlEvent[] = [];
    const onOpen = mock();
    const client = createSidecarControlClient({
      endpoint: "http://127.0.0.1:9379/v1",
      WebSocketImpl: FakeWebSocket,
      onEvent: (event) => events.push(event),
      onOpen,
    });

    client.connect();
    const socket = FakeWebSocket.instances[0];
    socket.open();
    const runtimeConfig: SidecarRuntimeConfig = {
      runtimeExe: "/opt/litert-lm",
      runtimeHost: "127.0.0.1",
      runtimePort: 9481,
      modelFile: "models/litert/gemma-4-E2B-it.litertlm",
      modelId: "gemma4-e2b",
      huggingfaceToken: "hf_secret",
      importModel: false,
      launchRuntime: true,
      runtimeVerbose: true,
    };

    client.startRuntime("debug", runtimeConfig);
    client.restartRuntime("release", runtimeConfig);
    client.stopRuntime();
    socket.receive({
      type: "status",
      status: {
        state: "available",
        backends: [],
        capabilities: {
          multimodal: { state: "available" },
        },
      },
    });
    socket.receive({
      type: "log",
      entry: {
        seq: 7,
        source: "runtime",
        stream: "stderr",
        line: "runtime ready",
      },
    });

    expect(socket.url).toBe("ws://127.0.0.1:9379/sidecar/v1/ws");
    expect(onOpen).toHaveBeenCalledTimes(1);
    expect(socket.sent.map((message) => JSON.parse(message))).toEqual([
      { type: "status.get" },
      { type: "logs.subscribe" },
      { type: "runtime.start", mode: "debug", config: runtimeConfig },
      { type: "runtime.restart", mode: "release", config: runtimeConfig },
      { type: "runtime.stop" },
    ]);
    expect(events).toHaveLength(2);
    expect(events[0].type).toBe("status");
    expect(events[1]).toMatchObject({
      type: "log",
      entry: { seq: 7, line: "runtime ready" },
    });
  });

  it("tunnels API responses over request IDs while keeping control events separate", async () => {
    FakeWebSocket.instances = [];
    const events: SidecarControlEvent[] = [];
    const client = createSidecarControlClient({
      endpoint: "http://127.0.0.1:9379/v1",
      WebSocketImpl: FakeWebSocket,
      onEvent: (event) => events.push(event),
    });

    client.connect();
    const socket = FakeWebSocket.instances[0];
    socket.open();

    const responsePromise = client.request({
      method: "POST",
      path: "/v1/chat/completions",
      headers: { "Content-Type": "application/json" },
      body: { model: "gemma4-e2b", stream: true },
    });

    const request = JSON.parse(socket.sent[2]);
    expect(request).toMatchObject({
      type: "api.request",
      id: "api-1",
      method: "POST",
      path: "/v1/chat/completions",
      headers: { "Content-Type": "application/json" },
    });
    expect(JSON.parse(base64ToText(request.bodyBase64))).toEqual({
      model: "gemma4-e2b",
      stream: true,
    });

    socket.receive({
      type: "log",
      entry: {
        seq: 8,
        source: "runtime",
        stream: "stdout",
        line: "tokenizing",
      },
    });
    socket.receive({
      type: "api.response.start",
      id: "api-1",
      status: 200,
      headers: { "content-type": "text/event-stream" },
    });

    const response = await responsePromise;
    const textPromise = response.text();

    socket.receive({
      type: "api.response.chunk",
      id: "api-1",
      dataBase64: textToBase64("data: one\n\n"),
    });
    socket.receive({
      type: "api.response.chunk",
      id: "api-1",
      dataBase64: textToBase64("data: two\n\n"),
    });
    socket.receive({ type: "api.response.end", id: "api-1" });

    expect(response.status).toBe(200);
    expect(response.headers["content-type"]).toBe("text/event-stream");
    await expect(textPromise).resolves.toBe("data: one\n\ndata: two\n\n");
    expect(events).toHaveLength(1);
    expect(events[0]).toMatchObject({ type: "log", entry: { line: "tokenizing" } });
  });

  it("sends API cancellation when an in-flight tunneled request is aborted", async () => {
    FakeWebSocket.instances = [];
    const client = createSidecarControlClient({
      endpoint: "http://127.0.0.1:9379/v1",
      WebSocketImpl: FakeWebSocket,
      onEvent: () => undefined,
    });
    const controller = new AbortController();

    client.connect();
    const socket = FakeWebSocket.instances[0];
    socket.open();
    const responsePromise = client.request({
      method: "POST",
      path: "/v1/chat/completions",
      body: { stream: true },
      signal: controller.signal,
    });

    controller.abort();

    expect(socket.sent.map((message) => JSON.parse(message)).at(-1)).toEqual({
      type: "api.cancel",
      id: "api-1",
    });
    await expect(responsePromise).rejects.toMatchObject({ name: "AbortError" });
  });
});
