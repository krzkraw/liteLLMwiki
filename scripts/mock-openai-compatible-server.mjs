import http from "node:http";
import crypto from "node:crypto";

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
      targetPath: "models/llamacpp/gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf",
      runtime: "llamacpp",
      role: "main",
      required: true,
      state: "present",
      bytesDownloaded: 1024,
      sizeBytes: 1024,
    },
    {
      id: "qwen3-embedding-gguf",
      repo: "Qwen/Qwen3-Embedding-0.6B-GGUF",
      filename: "Qwen3-Embedding-0.6B-Q8_0.gguf",
      targetPath: "models/llamacpp/Qwen3-Embedding-0.6B-Q8_0.gguf",
      runtime: "llamacpp",
      role: "embedding",
      required: true,
      state: "present",
      bytesDownloaded: 2048,
      sizeBytes: 2048,
    },
  ],
};

function sendCors(res) {
  res.setHeader("Access-Control-Allow-Origin", "*");
  res.setHeader("Access-Control-Allow-Headers", "content-type");
  res.setHeader("Access-Control-Allow-Methods", "GET,POST,OPTIONS");
}

function sendJson(res, body) {
  res.writeHead(200, { "content-type": "application/json" });
  res.end(JSON.stringify(body));
}

function encodeWebSocketTextFrame(text) {
  const payload = Buffer.from(text);

  if (payload.length < 126) {
    return Buffer.concat([Buffer.from([0x81, payload.length]), payload]);
  }

  if (payload.length <= 0xffff) {
    const header = Buffer.alloc(4);
    header[0] = 0x81;
    header[1] = 126;
    header.writeUInt16BE(payload.length, 2);
    return Buffer.concat([header, payload]);
  }

  const header = Buffer.alloc(10);
  header[0] = 0x81;
  header[1] = 127;
  header.writeBigUInt64BE(BigInt(payload.length), 2);
  return Buffer.concat([header, payload]);
}

function sendWebSocketJson(socket, body) {
  socket.write(encodeWebSocketTextFrame(JSON.stringify(body)));
}

function decodeWebSocketMessages(buffer) {
  const messages = [];
  let offset = 0;
  let close = false;

  while (offset + 2 <= buffer.length) {
    const firstByte = buffer[offset];
    const secondByte = buffer[offset + 1];
    const opcode = firstByte & 0x0f;
    const masked = (secondByte & 0x80) !== 0;
    let payloadLength = secondByte & 0x7f;
    let headerLength = 2;

    if (payloadLength === 126) {
      if (offset + 4 > buffer.length) {
        break;
      }
      payloadLength = buffer.readUInt16BE(offset + 2);
      headerLength = 4;
    } else if (payloadLength === 127) {
      if (offset + 10 > buffer.length) {
        break;
      }
      payloadLength = Number(buffer.readBigUInt64BE(offset + 2));
      headerLength = 10;
    }

    const maskLength = masked ? 4 : 0;
    const frameLength = headerLength + maskLength + payloadLength;
    if (offset + frameLength > buffer.length) {
      break;
    }

    if (opcode === 0x8) {
      close = true;
      offset += frameLength;
      break;
    }

    const maskOffset = offset + headerLength;
    const payloadOffset = maskOffset + maskLength;
    const payload = Buffer.from(
      buffer.subarray(payloadOffset, payloadOffset + payloadLength),
    );

    if (masked) {
      const mask = buffer.subarray(maskOffset, maskOffset + 4);
      for (let index = 0; index < payload.length; index += 1) {
        payload[index] ^= mask[index % 4];
      }
    }

    if (opcode === 0x1) {
      messages.push(payload.toString("utf8"));
    }

    offset += frameLength;
  }

  return {
    messages,
    close,
    remaining: buffer.subarray(offset),
  };
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
  const parsed = JSON.parse(message);

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

  if (parsed.type === "api.request") {
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
}

const server = http.createServer((req, res) => {
  sendCors(res);

  if (req.method === "OPTIONS") {
    res.writeHead(204);
    res.end();
    return;
  }

  if (req.method === "GET" && req.url === "/sidecar/v1/status") {
    sendJson(res, sidecarStatus);
    return;
  }

  if (req.method === "GET" && req.url === "/sidecar/v1/models") {
    sendJson(res, sidecarModels);
    return;
  }

  if (req.method === "POST" && req.url === "/sidecar/v1/multimodal") {
    sendJson(res, {
      text: "Mock multimodal response",
    });
    return;
  }

  if (req.method === "GET" && req.url === "/v1/models") {
    sendJson(res, {
      object: "list",
      data: [{ id: "gemma4-e2b", object: "model" }],
    });
    return;
  }

  if (req.method === "POST" && req.url === "/v1/chat/completions") {
    res.writeHead(200, {
      "content-type": "text/event-stream",
      "cache-control": "no-cache",
    });
    res.write('data: {"choices":[{"delta":{"content":"Mock "}}]}\n\n');
    res.write('data: {"choices":[{"delta":{"content":"response"}}]}\n\n');
    res.write("data: [DONE]\n\n");
    res.end();
    return;
  }

  res.writeHead(404, { "content-type": "text/plain" });
  res.end("not found");
});

server.on("upgrade", (req, socket) => {
  if (req.url !== "/sidecar/v1/ws") {
    socket.destroy();
    return;
  }

  const key = req.headers["sec-websocket-key"];
  if (typeof key !== "string") {
    socket.destroy();
    return;
  }

  const accept = crypto
    .createHash("sha1")
    .update(`${key}258EAFA5-E914-47DA-95CA-C5AB0DC85B11`)
    .digest("base64");

  socket.write(
    [
      "HTTP/1.1 101 Switching Protocols",
      "Upgrade: websocket",
      "Connection: Upgrade",
      `Sec-WebSocket-Accept: ${accept}`,
      "",
      "",
    ].join("\r\n"),
  );

  let pending = Buffer.alloc(0);

  socket.on("data", (chunk) => {
    pending = Buffer.concat([pending, chunk]);
    const decoded = decodeWebSocketMessages(pending);
    pending = decoded.remaining;

    for (const message of decoded.messages) {
      handleWebSocketMessage(socket, message);
    }

    if (decoded.close) {
      socket.end();
    }
  });
});

await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));

const address = server.address();
const port = typeof address === "object" && address ? address.port : 0;

console.log(JSON.stringify({ port, url: `http://127.0.0.1:${port}` }));

process.on("SIGTERM", () => {
  server.close(() => process.exit(0));
});
process.on("SIGINT", () => {
  server.close(() => process.exit(0));
});
