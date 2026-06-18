import http from "node:http";

function sendCors(res) {
  res.setHeader("Access-Control-Allow-Origin", "*");
  res.setHeader("Access-Control-Allow-Headers", "content-type");
  res.setHeader("Access-Control-Allow-Methods", "GET,POST,OPTIONS");
}

function sendJson(res, body) {
  res.writeHead(200, { "content-type": "application/json" });
  res.end(JSON.stringify(body));
}

const server = http.createServer((req, res) => {
  sendCors(res);

  if (req.method === "OPTIONS") {
    res.writeHead(204);
    res.end();
    return;
  }

  if (req.method === "GET" && req.url === "/sidecar/v1/status") {
    sendJson(res, {
      state: "available",
      detail: "Mock sidecar ready.",
      backends: {
        cpu: "available",
        gpu: "available",
        npu: "available",
        cuda: "not-a-litert-backend",
      },
      capabilities: {
        multimodal: {
          state: "available",
          endpoint: "/sidecar/v1/multimodal",
          detail: "Mock native multimodal endpoint ready.",
          imageBackends: ["cpu", "gpu"],
          audioBackends: ["cpu", "gpu"],
        },
      },
    });
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
