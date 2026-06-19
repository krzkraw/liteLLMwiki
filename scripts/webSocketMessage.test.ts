import { describe, expect, it } from "bun:test";
import { webSocketMessageToText } from "./webSocketMessage.mjs";

describe("webSocketMessageToText", () => {
  it("decodes text websocket payload shapes across Bun platforms", async () => {
    const text = JSON.stringify({ type: "status.get" });
    const arrayBuffer = new TextEncoder().encode(text).buffer;

    expect(await webSocketMessageToText(text)).toBe(text);
    expect(await webSocketMessageToText(Buffer.from(text))).toBe(text);
    expect(await webSocketMessageToText(arrayBuffer)).toBe(text);
    expect(await webSocketMessageToText(new Uint8Array(arrayBuffer))).toBe(text);
  });

  it("decodes Blob websocket payloads when the runtime provides them", async () => {
    const text = JSON.stringify({ type: "status" });

    expect(await webSocketMessageToText(new Blob([text]))).toBe(text);
  });
});
