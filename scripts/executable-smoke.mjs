import { spawn } from "child_process";
import { once } from "events";
import { writeFile } from "fs/promises";
import { dirname, join } from "path";
import { fileURLToPath } from "url";
import { chromium } from "playwright";
import { createSmokeWorkspace, launchSmokeChromium } from "./smokeRuntime.mjs";

const targetUrl = process.env.SMOKE_URL ?? "http://127.0.0.1:5173/";
const scriptDir = dirname(fileURLToPath(import.meta.url));
const mockServerPath = join(scriptDir, "mock-openai-compatible-server.mjs");
const workspace = await createSmokeWorkspace("litert-executable-smoke-");
const sampleImagePath = workspace.path("litert-executable-smoke-image.png");
let server;
let browser;

function waitForServerReady(child) {
  return new Promise((resolve, reject) => {
    let output = "";
    const timeout = setTimeout(() => {
      reject(new Error(`Mock server did not start. Output: ${output}`));
    }, 10_000);

    child.stdout.on("data", (chunk) => {
      output += chunk.toString();
      const lines = output.split(/\r?\n/);

      for (const line of lines) {
        if (!line.trim()) {
          continue;
        }

        try {
          const parsed = JSON.parse(line);

          if (typeof parsed.url === "string") {
            clearTimeout(timeout);
            resolve(parsed);
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

async function stopServer(child) {
  if (child.exitCode !== null) {
    return;
  }

  child.kill("SIGTERM");
  await once(child, "exit").catch(() => undefined);
}

try {
  server = spawn(process.execPath, [mockServerPath], {
    stdio: ["ignore", "pipe", "pipe"],
  });
  await writeFile(sampleImagePath, Buffer.from("mock image bytes"));
  const serverInfo = await waitForServerReady(server);
  const endpoint = `${serverInfo.url}/v1`;
  const statusResponse = await fetch(`${serverInfo.url}/sidecar/v1/status`);
  const sidecarStatus = await statusResponse.json();
  browser = await launchSmokeChromium(chromium);
  const page = await browser.newPage({ viewport: { width: 1440, height: 950 } });
  const events = [];

  page.on("console", (message) => {
    if (message.type() === "error") {
      events.push(`${message.type()}: ${message.text()}`);
    }
  });
  page.on("pageerror", (error) => events.push(`pageerror: ${error.message}`));

  await page.goto(targetUrl, { waitUntil: "networkidle" });
  await page.getByTestId("provider-executable-button").click();
  await page.getByTestId("executable-endpoint-input").fill(endpoint);
  await page.getByTestId("connect-sidecar-button").click();
  await page.getByText("Sidecar connected").waitFor({ timeout: 5000 });
  await page.getByText("gemma4-gguf").waitFor({ timeout: 5000 });
  await page.getByText("qwen3-embedding-gguf").waitFor({ timeout: 5000 });

  const connectDisabled = await page.getByTestId("connect-sidecar-button").isDisabled();
  const setupText = await page.locator(".setup-panel").textContent();

  await page.getByTestId("prompt-input").fill("Hello native provider");
  await page.getByTestId("send-button").click();
  await page.getByText("Mock response").waitFor({ timeout: 5000 });

  const assistantText = await page
    .locator('[data-testid="chat-message-assistant"]')
    .last()
    .textContent();

  await page.getByTestId("attachment-input").setInputFiles(sampleImagePath);
  await page.getByText("litert-executable-smoke-image.png").waitFor({
    timeout: 5000,
  });
  await page.getByTestId("prompt-input").fill("Describe attached image");
  await page.getByTestId("send-button").click();
  await page.getByText("Mock multimodal response").waitFor({ timeout: 5000 });

  const multimodalAssistantText = await page
    .locator('[data-testid="chat-message-assistant"]')
    .last()
    .textContent();

  const result = {
    endpoint,
    multimodalCapability: sidecarStatus.capabilities?.multimodal,
    setupText,
    connectDisabled,
    assistantText,
    multimodalAssistantText,
    events,
  };

  console.log(JSON.stringify(result, null, 2));

  const expectedBackendLabels =
    setupText?.includes("CPU") &&
    setupText.includes("GPU") &&
    setupText.includes("NPU") &&
    setupText.includes("CUDA probe-only");
  const unexpectedDisabledBackendLabels =
    setupText?.includes("GPU unavailable") || setupText?.includes("NPU unknown");
  const expectedSidecarDashboard =
    setupText?.includes("/v1/chat/completions") &&
    setupText.includes("/v1/embeddings") &&
    setupText.includes("/v1/rerank") &&
    setupText.includes("/sidecar/v1/models") &&
    setupText.includes("/sidecar/v1/ws") &&
    setupText.includes("gemma4-gguf") &&
    setupText.includes("qwen3-embedding-gguf") &&
    setupText.includes("Runtime host") &&
    setupText.includes("Model file");

  if (
    !setupText?.includes("Sidecar connected") ||
    !expectedBackendLabels ||
    unexpectedDisabledBackendLabels ||
    !expectedSidecarDashboard ||
    sidecarStatus.capabilities?.multimodal?.endpoint !== "/sidecar/v1/multimodal" ||
    sidecarStatus.capabilities?.multimodal?.state !== "available" ||
    !connectDisabled ||
    !assistantText?.includes("Mock response") ||
    !multimodalAssistantText?.includes("Mock multimodal response") ||
    events.some((event) => event.startsWith("error:") || event.startsWith("pageerror:"))
  ) {
    process.exitCode = 1;
  }
} finally {
  await browser?.close().catch(() => undefined);
  if (server) {
    await stopServer(server);
  }
  await workspace.cleanup();
}
