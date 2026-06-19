import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

describe("index.html", () => {
  it("declares an inert favicon so browser smoke runs stay console-clean", () => {
    const contents = readFileSync(join(process.cwd(), "index.html"), "utf8");

    expect(contents).toContain('<link rel="icon" href="data:," />');
  });
});
