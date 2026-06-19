import { spawn, type ChildProcessWithoutNullStreams } from "child_process";
import { once } from "events";
import { dirname, join } from "path";
import { fileURLToPath } from "url";
import { describe, expect, it } from "bun:test";

interface TestWebSocket {
  close(): void;
  readJson(): Promise<unknown>;
  writeText(text: string): void;
}

function waitForServerReady(child: ChildProcessWithoutNullStreams) {
  return new Promise<{ url: string }>((resolve, reject) => {
    let output = "";
    const timeout = setTimeout(() => {
      reject(new Error(`Mock server did not start. Output: ${output}`));
    }, 10_000);

    child.stdout.on("data", (chunk) => {
      output += chunk.toString();
      for (const line of output.split(/\r?\n/)) {
        if (!line.trim()) {
          continue;
        }

        try {
          const parsed = JSON.parse(line) as { url?: unknown };
          if (typeof parsed.url === "string") {
            clearTimeout(timeout);
            resolve({ url: parsed.url });
            return;
          }
        } catch {
          // Keep waiting for the JSON startup line.
        }
      }
    });
    child.once("error", (error) => {
      clearTimeout(timeout);
      reject(error);
    });
    child.once("exit", (code) => {
      clearTimeout(timeout);
      reject(new Error(`Mock server exited early with code ${code}. Output: ${output}`));
    });
  });
}

function websocketDataToText(data: unknown): string {
  if (typeof data === "string") {
    return data;
  }
  if (data instanceof ArrayBuffer) {
    return Buffer.from(data).toString("utf8");
  }
  if (ArrayBuffer.isView(data)) {
    return Buffer.from(data.buffer, data.byteOffset, data.byteLength).toString(
      "utf8",
    );
  }
  return String(data);
}

function openTestWebSocket(url: string) {
  return new Promise<TestWebSocket>((resolve, reject) => {
    const socket = new WebSocket(url);
    const queuedMessages: unknown[] = [];
    let opened = false;
    let readResolve: ((message: unknown) => void) | null = null;
    let readReject: ((error: Error) => void) | null = null;
    const openTimeout = setTimeout(() => {
      socket.close();
      reject(new Error(`Timed out opening WebSocket ${url}.`));
    }, 2_000);

    const deliverMessage = (message: unknown) => {
      if (!readResolve) {
        queuedMessages.push(message);
        return;
      }

      const resolveRead = readResolve;
      readResolve = null;
      readReject = null;
      resolveRead(message);
    };

    const rejectRead = (error: Error) => {
      const rejectPendingRead = readReject;
      readResolve = null;
      readReject = null;
      rejectPendingRead?.(error);
    };

    const testWebSocket: TestWebSocket = {
      close() {
        socket.close();
      },
      readJson() {
        const queued = queuedMessages.shift();
        if (queued !== undefined) {
          return Promise.resolve(queued);
        }

        return new Promise<unknown>((resolveRead, rejectRead) => {
          const readTimeout = setTimeout(() => {
            readReject = null;
            readResolve = null;
            rejectRead(new Error("Timed out waiting for WebSocket message."));
          }, 1_000);

          readResolve = (message) => {
            clearTimeout(readTimeout);
            resolveRead(message);
          };
          readReject = (error) => {
            clearTimeout(readTimeout);
            rejectRead(error);
          };
        });
      },
      writeText(text) {
        socket.send(text);
      },
    };

    socket.onopen = () => {
      opened = true;
      clearTimeout(openTimeout);
      resolve(testWebSocket);
    };
    socket.onmessage = (event) => {
      try {
        deliverMessage(JSON.parse(websocketDataToText(event.data)));
      } catch (error) {
        rejectRead(error instanceof Error ? error : new Error(String(error)));
      }
    };
    socket.onerror = () => {
      const error = new Error(`WebSocket error for ${url}.`);
      clearTimeout(openTimeout);
      if (!opened) {
        reject(error);
      }
      rejectRead(error);
    };
    socket.onclose = (event) => {
      clearTimeout(openTimeout);
      const error = new Error(
        `WebSocket closed${event.code ? ` with code ${event.code}` : ""}${
          event.reason ? `: ${event.reason}` : ""
        }.`,
      );
      if (!opened) {
        reject(error);
      }
      rejectRead(error);
    };
  });
}

describe("mock OpenAI-compatible server", () => {
  it("serves sidecar status over the WebSocket control channel", async () => {
    const scriptDir = dirname(fileURLToPath(import.meta.url));
    const child = spawn(process.execPath, [
      join(scriptDir, "mock-openai-compatible-server.mjs"),
    ]);
    let socket: TestWebSocket | undefined;

    try {
      const { url } = await waitForServerReady(child);
      socket = await openTestWebSocket(
        `${url.replace("http://", "ws://")}/sidecar/v1/ws`,
      );
      socket.writeText(JSON.stringify({ type: "status.get" }));

      await expect(socket.readJson()).resolves.toMatchObject({
        type: "status",
        status: {
          state: "available",
          backends: expect.arrayContaining([
            { backend: "cpu", state: "available" },
            { backend: "gpu", state: "available" },
            { backend: "npu", state: "available" },
            { backend: "cuda", state: "not-a-litert-backend" },
          ]),
          capabilities: {
            multimodal: {
              state: "available",
              endpoint: "/sidecar/v1/multimodal",
            },
          },
        },
      });
    } finally {
      socket?.close();
      child.kill("SIGTERM");
      await once(child, "exit").catch(() => undefined);
    }
  }, 10_000);
});
