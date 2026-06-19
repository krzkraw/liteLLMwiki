import { describe, expect, it } from "vitest";
import {
  appendAssistantText,
  failAssistantMessage,
  startAssistantTurn,
} from "./chatState";

describe("chatState", () => {
  it("starts a turn with user and empty assistant messages", () => {
    const result = startAssistantTurn([], "Explain local Gemma");

    expect(result.messages).toHaveLength(2);
    expect(result.messages[0]).toMatchObject({
      role: "user",
      text: "Explain local Gemma",
    });
    expect(result.messages[1]).toMatchObject({ role: "assistant", text: "" });
    expect(result.assistantId).toBe(result.messages[1].id);
  });

  it("records attachment names on the user message", () => {
    const result = startAssistantTurn([], "Describe this", [
      "diagram.png",
      "notes.wav",
    ]);

    expect(result.messages[0]).toMatchObject({
      role: "user",
      text: "Describe this",
      attachmentNames: ["diagram.png", "notes.wav"],
    });
  });

  it("appends streamed text only to the target assistant message", () => {
    const turn = startAssistantTurn([], "Hello");
    const otherTurn = startAssistantTurn(turn.messages, "Second prompt");
    const messages = appendAssistantText(
      otherTurn.messages,
      turn.assistantId,
      "Hi",
    );

    expect(messages[1].text).toBe("Hi");
    expect(messages[0].text).toBe("Hello");
    expect(messages[3].text).toBe("");
  });

  it("records provider errors on the pending assistant message", () => {
    const turn = startAssistantTurn([], "Hello");
    const messages = failAssistantMessage(
      turn.messages,
      turn.assistantId,
      "WebGPU unavailable",
    );

    expect(messages[1]).toMatchObject({
      role: "assistant",
      text: "WebGPU unavailable",
      status: "error",
    });
  });
});
