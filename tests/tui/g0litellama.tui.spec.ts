import { expect, Key, test } from "@microsoft/tui-test";

test.use({
  columns: 140,
  rows: 40,
  program: {
    file: "bash",
    args: ["-lc", "cd ../../G0LiteLLaMa && go run ./cmd/g0litellama-tui-fixture"],
  },
});

test.afterEach(({ terminal }) => {
  terminal.keyPress(Key.Escape);
});

test("renders dashboard on startup", async ({ terminal }) => {
  await expect(terminal.getByText("G0LiteLLaMa", { strict: false })).toBeVisible();
  await expect(terminal.getByText("1 Dashboard", { strict: false })).toBeVisible();
  await expect(terminal.getByText("2 Launch Wizard", { strict: false })).toBeVisible();
  await expect(terminal.getByText("3 Setup", { strict: false })).toBeVisible();
  await expect(terminal.getByText("LiteRT", { strict: false })).toBeVisible();
  await expect(terminal.getByText("llama.cpp", { strict: false })).toBeVisible();
});

test("keyboard navigation opens launch wizard", async ({ terminal }) => {
  await openWizardByKeyboard(terminal);

  await expect(terminal.getByText("Launch Wizard", { strict: false })).toBeVisible();
  await expect(terminal.getByText("runtime", { strict: false })).toBeVisible();
  await expect(terminal.getByText("model role", { strict: false })).toBeVisible();
  await expect(terminal.getByText("[ START ]", { strict: false })).toBeVisible();
});

test("launch wizard creates a fake runner", async ({ terminal }) => {
  await openWizardByKeyboard(terminal);

  terminal.keyPress(Key.Enter);

  await expect(terminal.getByText("4 ● LR-M-1", { strict: false })).toBeVisible();
  await expect(terminal.getByText("Runner LR-M-1", { strict: false })).toBeVisible();
  await expect(terminal.getByText("Routes / Controls", { strict: false })).toBeVisible();
});

test("chat tab renders clickable streaming layout", async ({ terminal }) => {
  await openWizardByKeyboard(terminal);
  terminal.keyPress(Key.Enter);
  await expect(terminal.getByText("4 ● LR-M-1", { strict: false })).toBeVisible();

  terminal.keyPress("5");
  await expect(terminal.getByText("/v1/chat/completions", { strict: false })).toBeVisible();
  await expect(terminal.getByText("Prompt settings", { strict: false })).toBeVisible();
  await expect(terminal.getByText("Thinking: off", { strict: false })).toBeVisible();
  await expect(terminal.getByText("Target: main", { strict: false })).toBeVisible();
  await expect(terminal.getByText("Temperature: default", { strict: false })).toBeVisible();
  await expect(terminal.getByText("Ready. Input your prompt", { strict: false })).toBeVisible();
  await expect(terminal.getByText("Send", { strict: false })).toBeVisible();
  // Real scrollbar uses ▲/▼ instead of old [up]/[down] controls
  if (findText(terminal, "Chat console") || findText(terminal, "Transcript")) {
    throw new Error("chat should not render old console/transcript panels");
  }

  clickText(terminal, "Thinking:");
  await expect(terminal.getByText("on", { strict: false })).toBeVisible();
  await expect(terminal.getByText("off", { strict: false })).toBeVisible();

  clickText(terminal, "Max Tokens:");
  await expect(terminal.getByText("custom...", { strict: false })).toBeVisible();
  clickText(terminal, "custom...");
  terminal.write("777");
  await expect(terminal.getByText("777", { strict: false })).toBeVisible();
  terminal.keyPress(Key.Enter);
  await expect(terminal.getByText("Max Tokens: 777", { strict: false })).toBeVisible();

  terminal.write("Hi?! @ []");
  terminal.keyPress(Key.Enter);
  await expect(terminal.getByText("Hi?! @ []", { strict: false })).toBeVisible();
});

test("dashboard keyboard can open and select runner route slot", async ({ terminal }) => {
  await openWizardByKeyboard(terminal);
  terminal.keyPress(Key.Enter);
  await expect(terminal.getByText("4 ● LR-M-1", { strict: false })).toBeVisible();

  terminal.keyPress("2");
  await expect(terminal.getByText("[ START ]", { strict: false })).toBeVisible();
  terminal.keyPress(Key.Enter);
  await expect(terminal.getByText("5 ● LR-M-2", { strict: false })).toBeVisible();

  terminal.keyPress("1");
  await expect(terminal.getByText("main        LR-M-2 running  [choose]", { strict: false })).toBeVisible();
  terminal.keyPress("m");
  await expect(terminal.getByText("Main runners", { strict: false })).toBeVisible();
  terminal.keyPress("1");
  await expect(terminal.getByText("main        LR-M-1 running  [choose]", { strict: false })).toBeVisible();
});

test("wizard option modal updates command preview", async ({ terminal }) => {
  await openWizardByKeyboard(terminal);
  terminal.write("t");
  await expect(terminal.getByText("[ctk]", { strict: false })).toBeVisible();

  terminal.write("k");

  await expect(terminal.getByText("--cache-type-k", { strict: false })).toBeVisible();
  await expect(terminal.getByText("[ Save ]", { strict: false })).toBeVisible();
  await expect(terminal.getByText("[ Reset ]", { strict: false })).toBeVisible();

  terminal.write("q4_0");
  terminal.keyPress(Key.Enter);

  await expect(terminal.getByText("q4_0           KV cache K", { strict: false })).toBeVisible();
});

test("wizard command preview edit adds option rows", async ({ terminal }) => {
  await openWizardByKeyboard(terminal);
  terminal.write("t");

  terminal.write("c");
  await expect(terminal.getByText("Edit Command Preview", { strict: false })).toBeVisible();

  terminal.write(" --threads 8");
  terminal.keyPress(Key.Enter);

  await expect(terminal.getByText("[threads]", { strict: false })).toBeVisible();
  await expect(terminal.getByText("--threads 8", { strict: false })).toBeVisible();
});

test("mouse click can switch tabs or activate a visible control", async ({ terminal }) => {
  await expect(terminal.getByText("2 Launch Wizard", { strict: false })).toBeVisible();

  clickText(terminal, "Launch Wizard");

  await expect(terminal.getByText("runtime", { strict: false })).toBeVisible();
  await expect(terminal.getByText("[ START ]", { strict: false })).toBeVisible();
});

test("global menu palette click changes palette", async ({ terminal }) => {
  await expect(terminal.getByText("Menu", { strict: false })).toBeVisible();

  clickText(terminal, "Menu");

  await expect(terminal.getByText("Global menu", { strict: false })).toBeVisible();
  if (findText(terminal, "change view") || findText(terminal, "choose colors")) {
    throw new Error("global menu should not show descriptions");
  }

  clickText(terminal, "Palette themes");
  await expect(terminal.getByText("Palette choices", { strict: false })).toBeVisible();

  clickText(terminal, "Amber");
  clickText(terminal, "Menu");
  clickText(terminal, "Palette themes");
  await expect(terminal.getByText("● Amber", { strict: false })).toBeVisible();
});

test("chat keyboard scroll with sticky composer", async ({ terminal }) => {
  await openWizardByKeyboard(terminal);
  terminal.keyPress(Key.Enter);
  await expect(terminal.getByText("4 ● LR-M-1", { strict: false })).toBeVisible();

  // Switch to chat tab
  terminal.keyPress("5");
  await expect(terminal.getByText("Prompt settings", { strict: false })).toBeVisible();

  // Send a message to populate chat (user message visible immediately)
  terminal.write("hello chat");
  terminal.keyPress(Key.Enter);
  await expect(terminal.getByText("hello chat", { strict: false })).toBeVisible();

  // Keyboard scroll up (PageUp) - should scroll the scrollbox without crashing
  terminal.keyPress(Key.PageUp);

  // After scrolling up, composer and bottom bar must still be accessible
  // Bottom bar shows chat actions Clear and New regardless of scroll position
  await expect(terminal.getByText("Clear", { strict: false })).toBeVisible();
  await expect(terminal.getByText("New", { strict: false })).toBeVisible();

  // The draft text is still in the input box after API error restores it
  await expect(terminal.getByText("hello chat", { strict: false })).toBeVisible();

  // Scroll back down
  terminal.keyPress(Key.PageDown);
  await expect(terminal.getByText("Clear", { strict: false })).toBeVisible();
  await expect(terminal.getByText("New", { strict: false })).toBeVisible();
});

test("chat bounded settings editor clips long values", async ({ terminal }) => {
  await openWizardByKeyboard(terminal);
  terminal.keyPress(Key.Enter);
  await expect(terminal.getByText("4 ● LR-M-1", { strict: false })).toBeVisible();

  // Switch to chat tab
  terminal.keyPress("5");
  await expect(terminal.getByText("Prompt settings", { strict: false })).toBeVisible();

  // Click "System: empty" in settings to open the system prompt editor
  clickText(terminal, "System: empty");
  // Popup should show "System" title and "Enter saves." hint
  await expect(terminal.getByText("Enter saves.", { strict: false })).toBeVisible();

  // Type a long system prompt that exceeds 80 char clipping limit
  const longPrompt = "This is a very long system prompt that should definitely exceed the 80-character limit and get clipped by the settings display";
  terminal.write(longPrompt);

  // Press Enter to save and close popup
  terminal.keyPress(Key.Enter);

  // After saving, popup closes. Settings line no longer shows "empty"
  // and shows the clipped version with "..."
  await expect(terminal.getByText("System:", { strict: false })).toBeVisible();
  await expect(terminal.getByText("...", { strict: false })).toBeVisible();
});

async function openWizardByKeyboard(terminal) {
  await expect(terminal.getByText("2 Launch Wizard", { strict: false })).toBeVisible();
  terminal.keyPress("2");
  await expect(terminal.getByText("[ START ]", { strict: false })).toBeVisible();
}

function clickText(terminal, text: string) {
  const hit = findText(terminal, text);
  if (!hit) {
    throw new Error(`text not visible: ${text}`);
  }
  terminal.mousePress(hit.x + Math.floor(text.length / 2), hit.y);
}

function findText(terminal, text: string, lineText = ""): { x: number; y: number } | undefined {
  const hit = terminal
    .getViewableBuffer()
    .map((row) => row.join(""))
    .map((line, y) => ({ line, x: line.indexOf(text), y }))
    .find((row) => row.x >= 0 && (lineText === "" || row.line.includes(lineText)));
  return hit && { x: hit.x, y: hit.y };
}
