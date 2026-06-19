import { describe, expect, it } from "vitest";
import { collectOpenAiSseText } from "./openaiSse";

describe("collectOpenAiSseText", () => {
  it("collects split streamed chat completion chunks", async () => {
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(
          new TextEncoder().encode(
            'data: {"choices":[{"delta":{"content":"Hel"}}]}\n',
          ),
        );
        controller.enqueue(
          new TextEncoder().encode(
            'data: {"choices":[{"delta":{"content":"lo"}}]}\n\n',
          ),
        );
        controller.enqueue(new TextEncoder().encode("data: [DONE]\n\n"));
        controller.close();
      },
    });

    const tokens: string[] = [];
    const text = await collectOpenAiSseText(stream, (token) => tokens.push(token));

    expect(tokens).toEqual(["Hel", "lo"]);
    expect(text).toBe("Hello");
  });

  it("collects JSON events split across byte chunks and stops at done", async () => {
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(
          new TextEncoder().encode('data: {"choices":[{"delta":{"content":"Na'),
        );
        controller.enqueue(
          new TextEncoder().encode('tive"}}]}\n\ndata: [DONE]\n\n'),
        );
        controller.enqueue(
          new TextEncoder().encode(
            'data: {"choices":[{"delta":{"content":"ignored"}}]}\n\n',
          ),
        );
        controller.close();
      },
    });

    const tokens: string[] = [];
    const text = await collectOpenAiSseText(stream, (token) => tokens.push(token));

    expect(tokens).toEqual(["Native"]);
    expect(text).toBe("Native");
  });

  it("skips malformed data events and keeps collecting valid chunks", async () => {
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new TextEncoder().encode("data: {broken\n\n"));
        controller.enqueue(
          new TextEncoder().encode(
            'data: {"choices":[{"delta":{"content":"OK"}}]}\n\n',
          ),
        );
        controller.enqueue(new TextEncoder().encode("data: [DONE]\n\n"));
        controller.close();
      },
    });

    const tokens: string[] = [];
    const text = await collectOpenAiSseText(stream, (token) => tokens.push(token));

    expect(tokens).toEqual(["OK"]);
    expect(text).toBe("OK");
  });
});
