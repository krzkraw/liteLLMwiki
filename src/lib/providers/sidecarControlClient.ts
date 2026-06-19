import type { SidecarStatus } from "./sidecarClient";
import { normalizeExecutableEndpoint } from "./endpoint";

export type SidecarRuntimeMode = "release" | "debug";

export interface SidecarRuntimeConfig {
  upstream?: string;
  runtimeExe?: string;
  runtimeHost?: string;
  runtimePort?: number;
  modelFile?: string;
  modelId?: string;
  huggingfaceToken?: string;
  importModel?: boolean;
  launchRuntime?: boolean;
  runtimeVerbose?: boolean;
}

export interface SidecarLogEntry {
  seq: number;
  source: "sidecar" | "runtime";
  stream: "stdout" | "stderr";
  line: string;
}

export type SidecarControlEvent =
  | { type: "status"; status: SidecarStatus }
  | { type: "log"; entry: SidecarLogEntry }
  | { type: "error"; message: string };

export interface SidecarApiRequest {
  method: string;
  path: string;
  headers?: Record<string, string>;
  body?: unknown;
  signal?: AbortSignal;
}

export interface SidecarApiResponse {
  status: number;
  headers: Record<string, string>;
  body: ReadableStream<Uint8Array>;
  text(): Promise<string>;
  json(): Promise<unknown>;
}

type MinimalWebSocket = {
  onopen: ((event: Event) => void) | null;
  onmessage: ((event: MessageEvent) => void) | null;
  onerror: ((event: Event) => void) | null;
  onclose: ((event: CloseEvent) => void) | null;
  send(message: string): void;
  close(): void;
};

type WebSocketConstructor = new (url: string) => MinimalWebSocket;

export interface SidecarControlClientOptions {
  endpoint: string;
  WebSocketImpl?: WebSocketConstructor;
  onEvent: (event: SidecarControlEvent) => void;
  onOpen?: () => void;
  onClose?: () => void;
  onError?: () => void;
}

export interface SidecarControlClient {
  connect(): void;
  close(): void;
  request(request: SidecarApiRequest): Promise<SidecarApiResponse>;
  requestStatus(): void;
  subscribeLogs(): void;
  startRuntime(mode: SidecarRuntimeMode, config?: SidecarRuntimeConfig): void;
  restartRuntime(mode: SidecarRuntimeMode, config?: SidecarRuntimeConfig): void;
  stopRuntime(): void;
}

export function createSidecarWebSocketUrl(endpoint: string): string {
  const normalized = normalizeExecutableEndpoint(endpoint);
  const parsed = new URL(normalized, window.location.href);
  parsed.protocol = parsed.protocol === "https:" ? "wss:" : "ws:";
  parsed.pathname = "/sidecar/v1/ws";
  parsed.search = "";
  parsed.hash = "";

  return parsed.toString();
}

function createAbortError(): Error {
  if (typeof DOMException !== "undefined") {
    return new DOMException("Sidecar API request aborted.", "AbortError");
  }

  const error = new Error("Sidecar API request aborted.");
  error.name = "AbortError";
  return error;
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = "";
  const chunkSize = 0x8000;

  for (let index = 0; index < bytes.length; index += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(index, index + chunkSize));
  }

  return btoa(binary);
}

function base64ToBytes(value: string): Uint8Array {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);

  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }

  return bytes;
}

function bodyToBase64(body: unknown): string | undefined {
  if (body === undefined) {
    return undefined;
  }

  const text = typeof body === "string" ? body : JSON.stringify(body);
  return bytesToBase64(new TextEncoder().encode(text));
}

async function readStreamText(stream: ReadableStream<Uint8Array>): Promise<string> {
  const reader = stream.getReader();
  const decoder = new TextDecoder();
  let text = "";

  for (;;) {
    const { done, value } = await reader.read();
    if (done) {
      return text + decoder.decode();
    }
    text += decoder.decode(value, { stream: true });
  }
}

export function createSidecarControlClient({
  endpoint,
  WebSocketImpl = WebSocket,
  onEvent,
  onOpen,
  onClose,
  onError,
}: SidecarControlClientOptions): SidecarControlClient {
  let socket: MinimalWebSocket | null = null;
  let connected = false;
  let nextRequestId = 0;
  const pendingRequests = new Map<
    string,
    {
      resolve: (response: SidecarApiResponse) => void;
      reject: (error: Error) => void;
      controller: ReadableStreamDefaultController<Uint8Array> | null;
      abort: () => void;
    }
  >();

  function send(message: Record<string, unknown>): boolean {
    if (!socket || !connected) {
      onEvent({
        type: "error",
        message: "Sidecar control WebSocket is not connected.",
      });
      return false;
    }

    socket.send(JSON.stringify(message));
    return true;
  }

  function cleanupRequest(id: string) {
    const pending = pendingRequests.get(id);
    if (!pending) {
      return;
    }

    pending.abort();
    pendingRequests.delete(id);
  }

  function failRequest(id: string, error: Error) {
    const pending = pendingRequests.get(id);
    if (!pending) {
      return;
    }

    pending.reject(error);
    pending.controller?.error(error);
    cleanupRequest(id);
  }

  function failAllRequests(error: Error) {
    for (const id of Array.from(pendingRequests.keys())) {
      failRequest(id, error);
    }
  }

  function handleApiMessage(message: Record<string, unknown>): boolean {
    const type = String(message.type ?? "");
    if (!type.startsWith("api.")) {
      return false;
    }

    const id = typeof message.id === "string" ? message.id : "";
    const pending = pendingRequests.get(id);
    if (!pending) {
      return true;
    }

    if (type === "api.response.start") {
      const status =
        typeof message.status === "number" ? message.status : 0;
      const headers =
        message.headers && typeof message.headers === "object"
          ? (message.headers as Record<string, string>)
          : {};
      const body = new ReadableStream<Uint8Array>({
        start(controller) {
          pending.controller = controller;
        },
        cancel() {
          send({ type: "api.cancel", id });
          cleanupRequest(id);
        },
      });
      const response: SidecarApiResponse = {
        status,
        headers,
        body,
        text: () => readStreamText(body),
        json: async () => JSON.parse(await readStreamText(body)),
      };

      pending.resolve(response);
      return true;
    }

    if (type === "api.response.chunk") {
      if (typeof message.dataBase64 === "string") {
        pending.controller?.enqueue(base64ToBytes(message.dataBase64));
      }
      return true;
    }

    if (type === "api.response.end") {
      pending.controller?.close();
      cleanupRequest(id);
      return true;
    }

    if (type === "api.error") {
      const error = new Error(
        typeof message.message === "string"
          ? message.message
          : "Sidecar API request failed.",
      );
      failRequest(id, error);
      return true;
    }

    return true;
  }

  return {
    connect() {
      socket = new WebSocketImpl(createSidecarWebSocketUrl(endpoint));
      socket.onopen = () => {
        connected = true;
        send({ type: "status.get" });
        send({ type: "logs.subscribe" });
        onOpen?.();
      };
      socket.onclose = () => {
        connected = false;
        failAllRequests(new Error("Sidecar WebSocket closed."));
        onClose?.();
      };
      socket.onerror = () => {
        connected = false;
        failAllRequests(new Error("Sidecar WebSocket failed."));
        onError?.();
      };
      socket.onmessage = (event) => {
        try {
          const parsed = JSON.parse(String(event.data)) as Record<string, unknown>;
          if (handleApiMessage(parsed)) {
            return;
          }
          onEvent(parsed as SidecarControlEvent);
        } catch {
          onEvent({
            type: "error",
            message: "Decode sidecar control WebSocket message failed.",
          });
        }
      };
    },
    close() {
      connected = false;
      failAllRequests(new Error("Sidecar WebSocket closed."));
      socket?.close();
      socket = null;
    },
    request(request: SidecarApiRequest): Promise<SidecarApiResponse> {
      if (request.signal?.aborted) {
        return Promise.reject(createAbortError());
      }

      const id = `api-${(nextRequestId += 1)}`;
      return new Promise((resolve, reject) => {
        const abort = () => {
          const error = createAbortError();
          send({ type: "api.cancel", id });
          failRequest(id, error);
        };

        pendingRequests.set(id, {
          resolve,
          reject,
          controller: null,
          abort: () => request.signal?.removeEventListener("abort", abort),
        });
        request.signal?.addEventListener("abort", abort, { once: true });

        const encodedBody = bodyToBase64(request.body);
        const message = {
          type: "api.request",
          id,
          method: request.method,
          path: request.path,
          ...(request.headers ? { headers: request.headers } : {}),
          ...(encodedBody ? { bodyBase64: encodedBody } : {}),
        };

        if (!send(message)) {
          failRequest(
            id,
            new Error("Sidecar control WebSocket is not connected."),
          );
        }
      });
    },
    requestStatus() {
      send({ type: "status.get" });
    },
    subscribeLogs() {
      send({ type: "logs.subscribe" });
    },
    startRuntime(mode: SidecarRuntimeMode, config?: SidecarRuntimeConfig) {
      send({ type: "runtime.start", mode, ...(config ? { config } : {}) });
    },
    restartRuntime(mode: SidecarRuntimeMode, config?: SidecarRuntimeConfig) {
      send({ type: "runtime.restart", mode, ...(config ? { config } : {}) });
    },
    stopRuntime() {
      send({ type: "runtime.stop" });
    },
  };
}
