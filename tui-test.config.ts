import { defineConfig } from "@microsoft/tui-test";

export default defineConfig({
  expect: { timeout: 10_000 },
  retries: process.env.CI ? 2 : 0,
  testMatch: "tests/tui/**/*.tui.spec.ts",
  timeout: 120_000,
  trace: true,
  workers: 1,
});
