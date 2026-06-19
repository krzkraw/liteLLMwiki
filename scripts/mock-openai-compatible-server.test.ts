import { spawn, type ChildProcessWithoutNullStreams } from "child_process";
import { randomBytes } from "crypto";
import { once } from "events";
import { dirname, join } from "path";
import { fileURLToPath } from "url";
import { describe, expect, it } from "bun:test";

type BunTcpSocket = Awaited<ReturnType<typeof Bun.connect>>;

interface RawWebSocket {
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

function encodeClientWebSocketTextFrame(text: string): Buffer {
  const payload = Buffer.from(text);
  const mask = randomBytes(4);
  const header =
    payload.length < 126
      ? Buffer.from([0x81, 0x80 | payload.length])
      : Buffer.from([0x81, 0x80 | 126, payload.length >> 8, payload.length & 0xff]);
  const masked = Buffer.alloc(payload.length);

  for (let index = 0; index < payload.length; index += 1) {
    masked[index] = payload[index] ^ mask[index % mask.length];
  }

  return Buffer.concat([header, mask, masked]);
}

function decodeServerWebSocketTextFrame(buffer: Buffer): string | null {
  if (buffer.length < 2) {
    return null;
  }

  const opcode = buffer[0] & 0x0f;
  let payloadLength = buffer[1] & 0x7f;
  let headerLength = 2;

  if (payloadLength === 126) {
    if (buffer.length < 4) {
      return null;
    }
    payloadLength = buffer.readUInt16BE(2);
    headerLength = 4;
  } else if (payloadLength === 127) {
    if (buffer.length < 10) {
      return null;
    }
    payloadLength = Number(buffer.readBigUInt64BE(2));
    headerLength = 10;
  }

  if (opcode !== 0x1 || buffer.length < headerLength + payloadLength) {
    return null;
  }

  return buffer.subarray(headerLength, headerLength + payloadLength).toString("utf8");
}

function openRawWebSocket(url: string) {
  return new Promise<RawWebSocket>((resolve, reject) => {
    const parsed = new URL(url);
    let socketRef: BunTcpSocket | null = null;
    let upgraded = false;
    let upgradeBuffer = Buffer.alloc(0);
    let frameBuffer = Buffer.alloc(0);
    let readResolve: ((message: unknown) => void) | null = null;
    let readReject: ((error: Error) => void) | null = null;
    const timeout = setTimeout(() => {
      socketRef?.end();
      reject(
        new Error(
          `Timed out waiting for WebSocket upgrade to ${parsed.hostname}:${parsed.port}; socket opened: ${
            socketRef !== null
          }.`,
        ),
      );
    }, 1_000);

    const handleFrameBytes = (data: Buffer) => {
      frameBuffer = Buffer.concat([frameBuffer, data]);
      const text = decodeServerWebSocketTextFrame(frameBuffer);
      if (!text || !readResolve) {
        return;
      }

      frameBuffer = Buffer.alloc(0);
      const resolveRead = readResolve;
      readResolve = null;
      readReject = null;
      resolveRead(JSON.parse(text));
    };

    const rawWebSocket: RawWebSocket = {
      close() {
        socketRef?.end();
      },
      readJson() {
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

          if (frameBuffer.length > 0) {
            handleFrameBytes(Buffer.alloc(0));
          }
        });
      },
      writeText(text) {
        socketRef?.write(encodeClientWebSocketTextFrame(text));
      },
    };

    void Bun.connect({
      hostname: parsed.hostname,
      port: Number(parsed.port),
      socket: {
        open(socket) {
          socketRef = socket;
          const key = randomBytes(16).toString("base64");
          socket.write(
            [
              `GET ${parsed.pathname} HTTP/1.1`,
              `Host: ${parsed.host}`,
              "Upgrade: websocket",
              "Connection: Upgrade",
              `Sec-WebSocket-Key: ${key}`,
              "Sec-WebSocket-Version: 13",
              "",
              "",
            ].join("\r\n"),
          );
        },
        data(socket, chunk) {
          const data = Buffer.from(chunk);

          if (upgraded) {
            handleFrameBytes(data);
            return;
          }

          upgradeBuffer = Buffer.concat([upgradeBuffer, data]);
          const headerEnd = upgradeBuffer.indexOf("\r\n\r\n");
          if (headerEnd === -1) {
            return;
          }

          const header = upgradeBuffer.subarray(0, headerEnd).toString("utf8");
          if (!header.startsWith("HTTP/1.1 101")) {
            clearTimeout(timeout);
            socket.end();
            reject(new Error(`WebSocket upgrade failed: ${header}`));
            return;
          }

          upgraded = true;
          clearTimeout(timeout);
          resolve(rawWebSocket);

          const frameStart = headerEnd + 4;
          if (upgradeBuffer.length > frameStart) {
            handleFrameBytes(upgradeBuffer.subarray(frameStart));
          }
        },
        close() {
          readReject?.(new Error("WebSocket closed."));
        },
        error(_socket, error) {
          clearTimeout(timeout);
          readReject?.(error);
          reject(error);
        },
      },
    }).catch((error) => {
      clearTimeout(timeout);
      reject(error);
    });
  });
}

describe("mock OpenAI-compatible server", () => {
  it("serves sidecar status over the WebSocket control channel", async () => {
    const scriptDir = dirname(fileURLToPath(import.meta.url));
    const child = spawn(process.execPath, [
      join(scriptDir, "mock-openai-compatible-server.mjs"),
    ]);
    let socket: RawWebSocket | undefined;

    try {
      const { url } = await waitForServerReady(child);
      socket = await openRawWebSocket(
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
