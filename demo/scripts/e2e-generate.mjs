import { existsSync } from "node:fs";
import { resolve } from "node:path";
import { chromium } from "playwright";
import { createChromiumGpuArgs } from "./smokeRuntime.mjs";

const appUrl = process.env.APP_URL ?? "http://127.0.0.1:5173/";
const modelPath = process.env.MODEL_PATH ? resolve(process.env.MODEL_PATH) : "";
const timeoutMs = Number(process.env.E2E_TIMEOUT_MS ?? 600_000);
const headless = process.env.HEADLESS !== "0" && process.env.HEADED !== "1";
const channel = process.env.PLAYWRIGHT_CHANNEL || undefined;

if (!modelPath) {
  console.error("MODEL_PATH is required.");
  console.error(
    "Example: HEADLESS=0 PLAYWRIGHT_CHANNEL=chrome MODEL_PATH=/path/to/gemma-4-E2B-it-web.litertlm npm run e2e:generate",
  );
  process.exit(1);
}

if (!existsSync(modelPath)) {
  console.error(`MODEL_PATH does not exist: ${modelPath}`);
  process.exit(1);
}

if (!modelPath.toLowerCase().endsWith(".litertlm")) {
  console.error(`MODEL_PATH must point to a .litertlm file: ${modelPath}`);
  process.exit(1);
}

const browser = await chromium.launch({
  channel,
  headless,
  args: createChromiumGpuArgs(),
});

const page = await browser.newPage({ viewport: { width: 1440, height: 950 } });
const events = [];
page.on("console", (message) => {
  if (["error", "warning"].includes(message.type())) {
    events.push(`${message.type()}: ${message.text()}`);
  }
});
page.on("pageerror", (error) => events.push(`pageerror: ${error.message}`));

try {
  await page.goto(appUrl, { waitUntil: "networkidle" });
  await page.getByTestId("local-model-input").setInputFiles(modelPath);
  await page.getByText("Selected local model file").waitFor({ timeout: 10_000 });
  await page.getByText("Runtime ready").waitFor({ timeout: 30_000 });

  await page.getByTestId("load-model-button").click();
  await page.getByText("Provider loaded").waitFor({ timeout: timeoutMs });

  await page
    .getByTestId("prompt-input")
    .fill("Write one short sentence about local Gemma.");
  await page.getByTestId("send-button").click();

  await page.waitForFunction(
    () => {
      const messages = Array.from(
        document.querySelectorAll('[data-testid="chat-message-assistant"] p'),
      );
      const last = messages.at(-1);
      const text = last?.textContent?.trim() ?? "";

      return text.length > 0 && text !== "Thinking...";
    },
    { timeout: timeoutMs },
  );

  const answer = (
    await page
      .locator('[data-testid="chat-message-assistant"] p')
      .last()
      .textContent()
  )?.trim();

  console.log(
    JSON.stringify(
      {
        appUrl,
        modelPath,
        answer,
        warnings: events.filter((event) => event.startsWith("warning:")),
      },
      null,
      2,
    ),
  );
} finally {
  await browser.close();
}
