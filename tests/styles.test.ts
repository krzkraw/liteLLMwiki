import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const stylesPath = join(process.cwd(), "src", "styles.css");
const styles = readFileSync(stylesPath, "utf8");

function normalizeNewlines(value: string): string {
  return value.replace(/\r\n/g, "\n");
}

function parseDeclarations(body: string): Map<string, string> {
  const declarations = new Map<string, string>();

  for (const item of body.split(";")) {
    const [property, ...valueParts] = item.split(":");
    const value = valueParts.join(":").trim();

    if (property && value) {
      declarations.set(property.trim(), value);
    }
  }

  return declarations;
}

function blocksFor(selector: string): Map<string, string>[] {
  const blocks: Map<string, string>[] = [];

  for (const match of styles.matchAll(/(?<selectors>[^{}@][^{}]*)\{(?<body>[^}]*)\}/gm)) {
    const selectorList = match.groups?.selectors
      ?.split(",")
      .map((item) => item.trim());

    if (selectorList?.includes(selector) && match.groups?.body) {
      blocks.push(parseDeclarations(match.groups.body));
    }
  }

  if (!blocks.length) {
    throw new Error(`Missing CSS block for ${selector}`);
  }

  return blocks;
}

function firstBlockFor(selector: string): Map<string, string> {
  return blocksFor(selector)[0];
}

function mergedDeclarationsFor(selector: string): Map<string, string> {
  return blocksFor(selector).reduce((merged, block) => {
    for (const [property, value] of block) {
      merged.set(property, value);
    }

    return merged;
  }, new Map<string, string>());
}

describe("application layout styles", () => {
  it("keeps document scrolling disabled and gives scroll ownership to panels", () => {
    expect(normalizeNewlines(styles)).toContain("html,\nbody,\n#root");
    expect(styles).toContain("overflow: hidden;");
    expect(firstBlockFor(".app-shell").get("overflow")).toBe("hidden");
    expect(firstBlockFor(".workspace").get("height")).toBe("100%");
    expect(firstBlockFor(".workspace").get("min-height")).toBe("0");
    expect(firstBlockFor(".setup-panel").get("min-height")).toBe("0");
    expect(mergedDeclarationsFor(".setup-panel").get("overflow")).toBe("auto");
    expect(firstBlockFor(".chat-panel").get("overflow")).toBe("hidden");
    expect(firstBlockFor(".insight-panel").get("min-height")).toBe("0");
    expect(firstBlockFor(".insight-panel").get("max-height")).toBe("100%");
    expect(firstBlockFor(".insight-panel").get("overflow")).toBe("auto");
  });

  it("keeps the chat transcript scrollable and the composer compact", () => {
    expect(firstBlockFor(".messages").get("flex")).toBe("1 1 auto");
    expect(firstBlockFor(".messages").get("min-height")).toBe("0");
    expect(firstBlockFor(".messages").get("overflow")).toBe("auto");
    expect(firstBlockFor(".composer").get("flex")).toBe("0 0 auto");
    expect(firstBlockFor(".composer").get("overflow")).toBe("visible");
    expect(firstBlockFor(".composer").get("padding")).toBe("8px 12px 10px");
    expect(firstBlockFor(".prompt-surface").get("position")).toBe("relative");
    expect(firstBlockFor(".prompt-surface").get("min-height")).toBe("96px");
    expect(firstBlockFor(".prompt-surface").get("padding")).toBe("9px 9px 48px");
    expect(firstBlockFor(".prompt-actions").get("position")).toBe("absolute");
    expect(firstBlockFor(".prompt-actions").get("right")).toBe("9px");
    expect(firstBlockFor(".prompt-actions").get("bottom")).toBe("9px");
  });
});
