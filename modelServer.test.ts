import { createServer, type Server } from "node:http";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { tmpdir } from "node:os";
import { afterEach, describe, expect, it } from "vitest";
import { createModelFileMiddleware } from "./modelServer";

let server: Server | null = null;
let tmpRoot: string | null = null;

async function startModelServer(files: Record<string, string>) {
  tmpRoot = await mkdtemp(join(tmpdir(), "litert-model-server-test-"));
  for (const [name, content] of Object.entries(files)) {
    const filePath = join(tmpRoot, name);
    await mkdir(dirname(filePath), { recursive: true });
    await writeFile(filePath, content);
  }

  const middleware = createModelFileMiddleware(tmpRoot);
  server = createServer((request, response) => {
    middleware(request, response, () => {
      response.writeHead(404, { "content-type": "text/plain" });
      response.end("not found");
    });
  });

  await new Promise<void>((resolve) => {
    server?.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  if (!address || typeof address !== "object") {
    throw new Error("test server did not bind to a TCP address");
  }

  return `http://127.0.0.1:${address.port}`;
}

describe("createModelFileMiddleware", () => {
  afterEach(async () => {
    if (server) {
      await new Promise<void>((resolve) => server?.close(() => resolve()));
      server = null;
    }
    if (tmpRoot) {
      await rm(tmpRoot, { recursive: true, force: true });
      tmpRoot = null;
    }
  });

  it("serves direct HEAD requests for model files", async () => {
    const baseUrl = await startModelServer({
      "litert/gemma-4-E2B-it-web.litertlm": "model bytes",
    });

    const response = await fetch(
      `${baseUrl}/models/litert/gemma-4-E2B-it-web.litertlm`,
      {
        method: "HEAD",
      },
    );

    expect(response.status).toBe(200);
    expect(response.headers.get("content-type")).toBe("application/octet-stream");
    expect(response.headers.get("content-length")).toBe("11");
    expect(response.headers.get("accept-ranges")).toBe("bytes");
  });

  it("serves byte ranges for large model fetches", async () => {
    const baseUrl = await startModelServer({
      "litert/gemma-4-E2B-it-web.litertlm": "0123456789",
    });

    const response = await fetch(
      `${baseUrl}/models/litert/gemma-4-E2B-it-web.litertlm`,
      {
        headers: { range: "bytes=2-5" },
      },
    );

    expect(response.status).toBe(206);
    expect(response.headers.get("content-range")).toBe("bytes 2-5/10");
    expect(await response.text()).toBe("2345");
  });

  it("rejects unsatisfiable byte ranges without streaming the full file", async () => {
    const baseUrl = await startModelServer({
      "litert/gemma-4-E2B-it-web.litertlm": "0123456789",
    });

    const response = await fetch(
      `${baseUrl}/models/litert/gemma-4-E2B-it-web.litertlm`,
      {
        headers: { range: "bytes=20-25" },
      },
    );

    expect(response.status).toBe(416);
    expect(response.headers.get("content-range")).toBe("bytes */10");
    expect(await response.text()).toBe("");
  });

  it("does not serve traversal or malformed paths", async () => {
    const baseUrl = await startModelServer({
      "litert/gemma-4-E2B-it-web.litertlm": "model bytes",
    });

    const traversal = await fetch(`${baseUrl}/models/../package.json`);
    const malformed = await fetch(`${baseUrl}/models/%E0%A4%A`);

    expect(traversal.status).toBe(404);
    expect(malformed.status).toBe(404);
  });
});
