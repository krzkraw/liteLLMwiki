const sidecarStatus = {
  state: "available",
  detail: "Mock sidecar ready.",
  backends: [
    { backend: "cpu", state: "available" },
    { backend: "gpu", state: "available" },
    { backend: "npu", state: "available" },
    { backend: "cuda", state: "not-a-litert-backend" },
  ],
  capabilities: {
    multimodal: {
      state: "available",
      endpoint: "/sidecar/v1/multimodal",
      detail: "Mock native multimodal endpoint ready.",
      imageBackends: ["cpu", "gpu"],
      audioBackends: ["cpu", "gpu"],
    },
  },
};

const sidecarModels = {
  models: [
    {
      id: "gemma4-gguf",
      repo: "unsloth/gemma-4-E2B-it-qat-GGUF",
      filename: "gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf",
      targetPath: "models/llamacpp/main/gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf",
      runtime: "llamacpp",
      role: "main",
      required: true,
      state: "present",
      bytesDownloaded: 1024,
      sizeBytes: 1024,
    },
    {
      id: "qwen3-embedding-q8-mungert",
      repo: "Mungert/Qwen3-Embedding-0.6B-GGUF",
      filename: "Qwen3-Embedding-0.6B-q8_0.gguf",
      targetPath: "models/llamacpp/embedding/Qwen3-Embedding-0.6B-q8_0.gguf",
      runtime: "llamacpp",
      role: "embedding",
      required: true,
      state: "present",
      bytesDownloaded: 2048,
      sizeBytes: 2048,
    },
  ],
};

function withCors(headers = {}) {
  return {
    "access-control-allow-origin": "*",
    "access-control-allow-headers": "content-type",
    "access-control-allow-methods": "GET,POST,OPTIONS",
    ...headers,
  };
}

function jsonResponse(body) {
  return Response.json(body, { headers: withCors() });
}

function textResponse(body, status = 200, headers = {}) {
  return new Response(body, {
    status,
    headers: withCors({
      "content-type": "text/plain",
      ...headers,
    }),
  });
}

function sendWebSocketJson(socket, body) {
  socket.send(JSON.stringify(body));
}

function sendApiResponse(socket, id, status, headers, body) {
  sendWebSocketJson(socket, {
    type: "api.response.start",
    id,
    status,
    headers,
  });

  if (body) {
    sendWebSocketJson(socket, {
      type: "api.response.chunk",
      id,
      dataBase64: Buffer.from(body).toString("base64"),
    });
  }

  sendWebSocketJson(socket, { type: "api.response.end", id });
}

function handleWebSocketMessage(socket, message) {
  const parsed = JSON.parse(String(message));

  if (parsed.type === "status.get") {
    sendWebSocketJson(socket, {
      type: "status",
      status: sidecarStatus,
    });
    return;
  }

  if (parsed.type === "logs.subscribe") {
    return;
  }

  if (parsed.type !== "api.request") {
    return;
  }

  const id = String(parsed.id ?? "");

  if (parsed.method === "POST" && parsed.path === "/v1/chat/completions") {
    sendApiResponse(
      socket,
      id,
      200,
      { "content-type": "text/event-stream" },
      'data: {"choices":[{"delta":{"content":"Mock "}}]}\n\n' +
        'data: {"choices":[{"delta":{"content":"response"}}]}\n\n' +
        "data: [DONE]\n\n",
    );
    return;
  }

  if (parsed.method === "POST" && parsed.path === "/sidecar/v1/multimodal") {
    sendApiResponse(
      socket,
      id,
      200,
      { "content-type": "application/json" },
      JSON.stringify({ text: "Mock multimodal response" }),
    );
    return;
  }

  if (parsed.method === "GET" && parsed.path === "/sidecar/v1/models") {
    sendApiResponse(
      socket,
      id,
      200,
      { "content-type": "application/json" },
      JSON.stringify(sidecarModels),
    );
    return;
  }

  sendApiResponse(
    socket,
    id,
    404,
    { "content-type": "text/plain" },
    "not found",
  );
}

const server = Bun.serve({
  hostname: "127.0.0.1",
  port: 0,
  fetch(request, server) {
    const url = new URL(request.url);

    if (url.pathname === "/sidecar/v1/ws" && server.upgrade(request)) {
      return undefined;
    }

    if (request.method === "OPTIONS") {
      return new Response(null, { status: 204, headers: withCors() });
    }

    if (request.method === "GET" && url.pathname === "/sidecar/v1/status") {
      return jsonResponse(sidecarStatus);
    }

    if (request.method === "GET" && url.pathname === "/sidecar/v1/models") {
      return jsonResponse(sidecarModels);
    }

    if (request.method === "POST" && url.pathname === "/sidecar/v1/multimodal") {
      return jsonResponse({
        text: "Mock multimodal response",
      });
    }

    if (request.method === "GET" && url.pathname === "/v1/models") {
      return jsonResponse({
        object: "list",
        data: [{ id: "gemma4-e2b", object: "model" }],
      });
    }

    if (request.method === "POST" && url.pathname === "/v1/chat/completions") {
      return new Response(
        'data: {"choices":[{"delta":{"content":"Mock "}}]}\n\n' +
          'data: {"choices":[{"delta":{"content":"response"}}]}\n\n' +
          "data: [DONE]\n\n",
        {
          headers: withCors({
            "cache-control": "no-cache",
            "content-type": "text/event-stream",
          }),
        },
      );
    }

    return textResponse("not found", 404);
  },
  websocket: {
    message(socket, message) {
      handleWebSocketMessage(socket, message);
    },
  },
});

console.log(
  JSON.stringify({ port: server.port, url: `http://127.0.0.1:${server.port}` }),
);

function stop() {
  server.stop(true);
  process.exit(0);
}

process.on("SIGTERM", stop);
process.on("SIGINT", stop);
