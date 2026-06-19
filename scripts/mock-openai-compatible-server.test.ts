import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { once } from "node:events";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

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

function waitForWebSocketMessage(socket: WebSocket) {
  return new Promise<unknown>((resolve, reject) => {
    const timeout = setTimeout(() => {
      reject(new Error("Timed out waiting for WebSocket message."));
    }, 1_000);

    socket.addEventListener(
      "message",
      (event) => {
        clearTimeout(timeout);
        resolve(JSON.parse(String(event.data)));
      },
      { once: true },
    );
    socket.addEventListener(
      "error",
      () => {
        clearTimeout(timeout);
        reject(new Error("WebSocket failed."));
      },
      { once: true },
    );
  });
}

function waitForWebSocketOpen(socket: WebSocket) {
  return new Promise<void>((resolve, reject) => {
    const timeout = setTimeout(() => {
      reject(new Error("Timed out waiting for WebSocket open."));
    }, 1_000);

    socket.addEventListener(
      "open",
      () => {
        clearTimeout(timeout);
        resolve();
      },
      { once: true },
    );
    socket.addEventListener(
      "error",
      () => {
        clearTimeout(timeout);
        reject(new Error("WebSocket failed before opening."));
      },
      { once: true },
    );
  });
}

describe("mock OpenAI-compatible server", () => {
  it("serves sidecar status over the WebSocket control channel", async () => {
    const scriptDir = dirname(fileURLToPath(import.meta.url));
    const child = spawn(process.execPath, [
      join(scriptDir, "mock-openai-compatible-server.mjs"),
    ]);
    let socket: WebSocket | undefined;

    try {
      const { url } = await waitForServerReady(child);
      socket = new WebSocket(
        `${url.replace("http://", "ws://")}/sidecar/v1/ws`,
      );
      await waitForWebSocketOpen(socket);

      socket.send(JSON.stringify({ type: "status.get" }));

      await expect(waitForWebSocketMessage(socket)).resolves.toMatchObject({
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
