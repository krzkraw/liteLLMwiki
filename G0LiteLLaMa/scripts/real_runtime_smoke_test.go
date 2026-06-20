package scripts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRealRuntimeSmokeCoversTextAndMultimodal(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is required to run real-runtime-smoke.sh")
	}
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun is required by real-runtime-smoke.sh and the fake runtime")
	}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve G0LiteLLaMa root: %v", err)
	}

	tmp := t.TempDir()
	fakeRuntime := filepath.Join(tmp, "litert-lm")
	if err := os.WriteFile(fakeRuntime, []byte(fakeLiteRTLM()), 0o755); err != nil {
		t.Fatalf("write fake runtime: %v", err)
	}
	modelFile := filepath.Join(tmp, "gemma-4-E2B-it.litertlm")
	if err := os.WriteFile(modelFile, []byte("fake model"), 0o644); err != nil {
		t.Fatalf("write fake model: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", filepath.Join(root, "scripts", "real-runtime-smoke.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"LITERT_LM_BIN="+fakeRuntime,
		"LITERT_HOME="+filepath.Join(tmp, "litert-home"),
		"MODEL_FILE="+modelFile,
		"READY_TIMEOUT_SECONDS=20",
		"CHAT_TIMEOUT_SECONDS=20",
	)

	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("real-runtime-smoke.sh timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("real-runtime-smoke.sh failed: %v\n%s", err, output)
	}

	text := string(output)
	if !strings.Contains(text, "Assistant: OK.") {
		t.Fatalf("text chat smoke output missing assistant response:\n%s", text)
	}
	if !strings.Contains(text, "Multimodal assistant: fake multimodal response") {
		t.Fatalf("multimodal smoke output missing assistant response:\n%s", text)
	}
}

func fakeLiteRTLM() string {
	return `#!/usr/bin/env bun
const http = require("http");

const args = process.argv.slice(2);

if (args[0] === "--version") {
  console.log("litert-lm, version fake");
  process.exit(0);
}

if (args[0] === "list") {
  console.log("ID              SIZE");
  console.log("gemma4-e2b      2.4 GB");
  process.exit(0);
}

if (args[0] === "import") {
  process.exit(0);
}

if (args[0] === "run") {
  const hasAttachment = args.some((arg) => arg.startsWith("--attachment="));
  console.log(hasAttachment ? "fake multimodal response" : "OK.");
  process.exit(0);
}

if (args[0] === "serve") {
  let host = "127.0.0.1";
  let port = 9381;
  for (let index = 1; index < args.length; index += 1) {
    if (args[index] === "--host") {
      host = args[index + 1];
      index += 1;
    } else if (args[index] === "--port") {
      port = Number(args[index + 1]);
      index += 1;
    }
  }

  const server = http.createServer((req, res) => {
    if (req.method === "GET" && req.url === "/v1/models") {
      res.writeHead(200, { "content-type": "application/json" });
      res.end(JSON.stringify({
        object: "list",
        data: [
          { id: "gemma4-e2b", object: "model" },
          { id: "gemma4-e2b,gpu", object: "model" }
        ]
      }));
      return;
    }

    if (req.method === "POST" && req.url === "/v1/chat/completions") {
      req.resume();
      res.writeHead(200, { "content-type": "application/json" });
      res.end(JSON.stringify({
        choices: [{ message: { role: "assistant", content: "OK." } }]
      }));
      return;
    }

    res.writeHead(404, { "content-type": "text/plain" });
    res.end("not found");
  });

  server.listen(port, host);
  const shutdown = () => server.close(() => process.exit(0));
  process.on("SIGTERM", shutdown);
  process.on("SIGINT", shutdown);
  return;
}

console.error("unsupported fake litert-lm args: " + args.join(" "));
process.exit(2);
`
}
