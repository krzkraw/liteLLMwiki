import { stat } from "fs/promises";
import { extname, join, normalize, resolve, sep } from "path";
import { crossOriginIsolationHeaders } from "../src/lib/securityHeaders";

const repoRoot = resolve(import.meta.dir, "..");
const distRoot = resolve(repoRoot, "dist");
const modelRoot = resolve(repoRoot, "models");

function getArgValue(name: string): string | null {
  const index = Bun.argv.indexOf(name);
  if (index === -1) {
    return null;
  }

  return Bun.argv[index + 1] ?? null;
}

const host = getArgValue("--host") ?? Bun.env.WEBUI_HOST ?? "127.0.0.1";
const port = Number(getArgValue("--port") ?? Bun.env.WEBUI_PORT ?? "5174");

function contentType(filePath: string): string {
  switch (extname(filePath)) {
    case ".css":
      return "text/css; charset=utf-8";
    case ".html":
      return "text/html; charset=utf-8";
    case ".js":
      return "text/javascript; charset=utf-8";
    case ".json":
      return "application/json; charset=utf-8";
    case ".map":
      return "application/json; charset=utf-8";
    case ".wasm":
      return "application/wasm";
    default:
      return "application/octet-stream";
  }
}

function safeJoin(root: string, requestPath: string): string | null {
  const decoded = decodeURIComponent(requestPath);
  const normalized = normalize(decoded).replace(/^(\.\.(\/|\\|$))+/, "");
  const target = resolve(root, `.${sep}${normalized}`);

  if (target !== root && !target.startsWith(`${root}${sep}`)) {
    return null;
  }

  return target;
}

type RangeResult =
  | { state: "none" }
  | { state: "invalid" }
  | { state: "satisfiable"; start: number; end: number };

function parseRange(header: string | null, size: number): RangeResult {
  if (!header) {
    return { state: "none" };
  }

  const match = /^bytes=(\d*)-(\d*)$/.exec(header);
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

async function serveFile(
  request: Request,
  filePath: string,
  type: string,
): Promise<Response | null> {
  const info = await stat(filePath).catch(() => null);

  if (!info?.isFile()) {
    return null;
  }

  const headers = new Headers({
    ...crossOriginIsolationHeaders,
    "accept-ranges": "bytes",
    "cache-control": "no-cache",
    "content-type": type,
  });
  const range = parseRange(request.headers.get("range"), info.size);

  if (range.state === "invalid") {
    headers.set("content-range", `bytes */${info.size}`);
    return new Response(null, { status: 416, headers });
  }

  if (range.state === "satisfiable") {
    headers.set("content-length", String(range.end - range.start + 1));
    headers.set("content-range", `bytes ${range.start}-${range.end}/${info.size}`);
    if (request.method === "HEAD") {
      return new Response(null, { status: 206, headers });
    }

    return new Response(Bun.file(filePath).slice(range.start, range.end + 1), {
      status: 206,
      headers,
    });
  }

  headers.set("content-length", String(info.size));
  if (request.method === "HEAD") {
    return new Response(null, { status: 200, headers });
  }

  return new Response(Bun.file(filePath), { status: 200, headers });
}

async function handleRequest(request: Request): Promise<Response> {
  const url = new URL(request.url);

  if (request.method !== "GET" && request.method !== "HEAD") {
    return new Response("method not allowed", { status: 405 });
  }

  if (url.pathname.startsWith("/models/")) {
    const modelPath = safeJoin(modelRoot, url.pathname.slice("/models/".length));
    const modelResponse = modelPath
      ? await serveFile(request, modelPath, "application/octet-stream")
      : null;
    return modelResponse ?? new Response("not found", { status: 404 });
  }

  const assetPath = safeJoin(distRoot, url.pathname === "/" ? "index.html" : url.pathname);
  const assetResponse = assetPath
    ? await serveFile(request, assetPath, contentType(assetPath))
    : null;
  if (assetResponse) {
    return assetResponse;
  }

  const indexResponse = await serveFile(
    request,
    join(distRoot, "index.html"),
    "text/html; charset=utf-8",
  );
  return indexResponse ?? new Response("dist/index.html not found; run bun run build first", {
    status: 404,
  });
}

Bun.serve({
  hostname: host,
  port,
  fetch: handleRequest,
});

console.log(`LiteRT Rspack preview listening at http://${host}:${port}/`);
