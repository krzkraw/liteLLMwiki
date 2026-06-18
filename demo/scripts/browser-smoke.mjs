import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { chromium } from "playwright";
import { createSmokeWorkspace } from "./smokeRuntime.mjs";

function getArgValue(name) {
  const index = process.argv.indexOf(name);
  if (index === -1) {
    return null;
  }

  return process.argv[index + 1] ?? null;
}

const targetUrl =
  getArgValue("--url") ?? process.env.SMOKE_URL ?? "http://127.0.0.1:5173/";
const requireModel =
  process.argv.includes("--require-model") ||
  process.env.SMOKE_REQUIRE_MODEL === "1";
const workspace = await createSmokeWorkspace("litert-browser-smoke-");
const sampleDir = workspace.path("folder");
const sampleModelPath = workspace.path("litert-smoke-model.litertlm");
let browser;

try {
  await mkdir(sampleDir, { recursive: true });
  await writeFile(
    path.join(sampleDir, "provider.ts"),
    'export const ProviderRuntime = "Gemma browser provider";\n',
  );
  await writeFile(
    path.join(sampleDir, "runtime.md"),
    "# Runtime\nGemma summarizes folders and builds Provider graph nodes.\n",
  );
  await writeFile(sampleModelPath, Buffer.alloc(16));

  browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1440, height: 950 } });
  const events = [];

  page.on("console", (message) => {
    if (message.type() === "error") {
      events.push(`${message.type()}: ${message.text()}`);
    }
  });
  page.on("pageerror", (error) => events.push(`pageerror: ${error.message}`));

  await page.goto(targetUrl, { waitUntil: "networkidle" });

  const wasmLoaderResponse = await page.request.get(
    new URL(
      "/vendor/litert-lm/core/wasm/litert_lm_core_wasm.js",
      targetUrl,
    ).toString(),
  );
  const modelHeadResponse = requireModel
    ? await page.request.fetch(
        new URL("/models/gemma-4-E2B-it-web.litertlm", targetUrl).toString(),
        { method: "HEAD" },
      )
    : null;
  const modelContentLength = Number(
    modelHeadResponse?.headers()["content-length"] ?? "0",
  );
  const modelContentType = modelHeadResponse?.headers()["content-type"] ?? "";
  const modelServedOk =
    !requireModel ||
    (modelHeadResponse?.ok() === true &&
      modelContentLength > 1_000_000_000 &&
      modelContentType === "application/octet-stream");
  const title = await page.title();
  const h1 = await page.locator("h1").textContent();
  const webProviderVisible = await page.getByTestId("provider-web-button").isVisible();
  const executableProviderVisible = await page
    .getByTestId("provider-executable-button")
    .isVisible();
  const mediaInputCount = await page.locator('[data-testid="media-input"]').count();
  const promptVisible = await page.getByTestId("prompt-input").isVisible();
  const sendVisible = await page.getByTestId("send-button").isVisible();
  const attachmentInputDisabled = await page
    .getByTestId("attachment-input")
    .isDisabled();

  await page.getByTestId("local-model-input").setInputFiles(sampleModelPath);
  await page.getByText("Selected local model file").waitFor({ timeout: 5000 });
  const localModelSelection = await page
    .getByText("Selected local model file")
    .isVisible();

  await page.getByTestId("folder-file-input").setInputFiles(sampleDir);
  const selectedText = await page.getByTestId("folder-file-count").textContent();
  await page.getByTestId("index-folder-button").click();
  await page.waitForFunction(() =>
    document
      .querySelector('[data-testid="folder-file-count"]')
      ?.textContent?.includes("2 indexed files"),
  );
  const indexedText = await page.getByTestId("folder-file-count").textContent();
  await page.getByTestId("summarize-folder-button").click();
  await page.waitForFunction(
    () =>
      !(
        document.querySelector('[data-testid="folder-summary"]')?.textContent ?? ""
      ).includes("No folder summary yet"),
  );
  const summaryText = await page.getByTestId("folder-summary").textContent();
  const graphNodeCount = await page.locator("[data-node-id]").count();
  const graphSvgCount = await page
    .getByTestId("knowledge-graph")
    .locator("svg")
    .count();
  const visibleGraphLabelCount = await page
    .getByTestId("knowledge-graph")
    .locator("svg text")
    .count();

  const result = {
    title,
    h1,
    webProviderVisible,
    executableProviderVisible,
    promptVisible,
    sendVisible,
    mediaInputCount,
    attachmentInputDisabled,
    wasmLoaderOk: wasmLoaderResponse.ok(),
    requireModel,
    modelServedOk,
    localModelSelection,
    selectedText,
    indexedText,
    summaryReady: !summaryText?.includes("No folder summary yet"),
    graphNodeCount,
    graphSvgCount,
    visibleGraphLabelCount,
    events,
  };

  console.log(JSON.stringify(result, null, 2));

  if (
    title !== "LiteRT Gemma Local Chat" ||
    h1 !== "Gemma Local Chat" ||
    !webProviderVisible ||
    !executableProviderVisible ||
    !promptVisible ||
    !sendVisible ||
    mediaInputCount !== 0 ||
    !attachmentInputDisabled ||
    !wasmLoaderResponse.ok() ||
    !modelServedOk ||
    !localModelSelection ||
    !selectedText?.includes("2 selected files") ||
    !indexedText?.includes("2 indexed files") ||
    summaryText?.includes("No folder summary yet") ||
    graphNodeCount < 4 ||
    graphSvgCount !== 1 ||
    visibleGraphLabelCount !== 0 ||
    events.some((event) => event.startsWith("error:") || event.startsWith("pageerror:"))
  ) {
    process.exitCode = 1;
  }
} finally {
  await browser?.close().catch(() => undefined);
  await workspace.cleanup();
}
