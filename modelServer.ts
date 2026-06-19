import { createReadStream } from "node:fs";
import { stat } from "node:fs/promises";
import { join } from "node:path";
import type { IncomingMessage, ServerResponse } from "node:http";

type Next = () => void;

export type ModelFileMiddleware = (
  request: IncomingMessage,
  response: ServerResponse,
  next: Next,
) => void;

function sendNotFound(response: ServerResponse) {
  response.writeHead(404, { "content-type": "text/plain" });
  response.end("not found");
}

function parseModelRequest(url: string | undefined): string | null {
  if (!url) {
    return null;
  }

  const parsed = new URL(url, "http://127.0.0.1");

  if (!parsed.pathname.startsWith("/models/")) {
    return null;
  }

  let modelPath: string;
  try {
    modelPath = decodeURIComponent(parsed.pathname.slice("/models/".length));
  } catch {
    return "";
  }

  if (!modelPath || modelPath.startsWith("/") || modelPath.includes("\\")) {
    return "";
  }

  const segments = modelPath.split("/");
  if (segments.some((segment) => !segment || segment === "." || segment === "..")) {
    return "";
  }

  return segments.join("/");
}

type ParsedRange =
  | { state: "none" }
  | { state: "invalid" }
  | { state: "satisfiable"; start: number; end: number };

function parseRange(range: string | undefined, size: number): ParsedRange {
  if (!range) {
    return { state: "none" };
  }

  const match = /^bytes=(\d*)-(\d*)$/.exec(range);
  if (!match) {
    return { state: "invalid" };
  }

  let start = match[1] === "" ? 0 : Number(match[1]);
  let end = match[2] === "" ? size - 1 : Number(match[2]);

  if (match[1] === "" && match[2] !== "") {
    const suffixLength = Number(match[2]);
    start = Math.max(size - suffixLength, 0);
    end = size - 1;
  }

  if (
    !Number.isSafeInteger(start) ||
    !Number.isSafeInteger(end) ||
    start < 0 ||
    end < start ||
    start >= size
  ) {
    return { state: "invalid" };
  }

  return { state: "satisfiable", start, end: Math.min(end, size - 1) };
}

export function createModelFileMiddleware(modelDir: string): ModelFileMiddleware {
  return (request, response, next) => {
    const modelName = parseModelRequest(request.url);

    if (modelName === null) {
      next();
      return;
    }

    if (!modelName || (request.method !== "GET" && request.method !== "HEAD")) {
      sendNotFound(response);
      return;
    }

    void (async () => {
      const modelPath = join(modelDir, modelName);
      const info = await stat(modelPath).catch(() => null);

      if (!info || !info.isFile()) {
        sendNotFound(response);
        return;
      }

      const commonHeaders = {
        "accept-ranges": "bytes",
        "cache-control": "no-cache",
        "content-type": "application/octet-stream",
      };
      const range = parseRange(request.headers.range, info.size);

      if (range.state === "invalid") {
        response.writeHead(416, {
          ...commonHeaders,
          "content-range": `bytes */${info.size}`,
        });
        response.end();
        return;
      }

      if (range.state === "satisfiable") {
        response.writeHead(206, {
          ...commonHeaders,
          "content-length": String(range.end - range.start + 1),
          "content-range": `bytes ${range.start}-${range.end}/${info.size}`,
        });
        if (request.method === "HEAD") {
          response.end();
          return;
        }

        createReadStream(modelPath, {
          start: range.start,
          end: range.end,
        }).pipe(response);
        return;
      }

      response.writeHead(200, {
        ...commonHeaders,
        "content-length": String(info.size),
      });
      if (request.method === "HEAD") {
        response.end();
        return;
      }

      createReadStream(modelPath).pipe(response);
    })().catch((error: unknown) => {
      response.writeHead(500, { "content-type": "text/plain" });
      response.end(error instanceof Error ? error.message : String(error));
    });
  };
}
