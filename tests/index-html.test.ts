import { readFileSync } from "fs";
import { join } from "path";
import { describe, expect, it } from "bun:test";

describe("index.html", () => {
  it("declares an inert favicon so browser smoke runs stay console-clean", () => {
    const contents = readFileSync(join(process.cwd(), "index.html"), "utf8");

    expect(contents).toContain('<link rel="icon" href="data:," />');
  });
});
