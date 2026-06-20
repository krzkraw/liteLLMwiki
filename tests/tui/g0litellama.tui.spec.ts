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

  await expect(terminal.getByText("-ctk q4_0", { strict: false })).toBeVisible();
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
