import { describe, expect, it } from "vitest";
import type { ChatMessage } from "./chatProvider";
import {
  addChatSession,
  closeChatSession,
  createChatSession,
  createInitialChatSession,
  updateChatSessionMessages,
  updateChatSessionPrompt,
} from "./chatSessions";

const userMessage: ChatMessage = {
  id: "message-1",
  role: "user",
  text: "Hello",
  status: "complete",
};

describe("chatSessions", () => {
  it("creates an initial empty chat session", () => {
    const session = createInitialChatSession();

    expect(session.id).toBeTruthy();
    expect(session.title).toBe("Chat 1");
    expect(session.messages).toEqual([]);
    expect(session.prompt).toBe("");
  });

  it("adds a new session and makes it active", () => {
    const initial = createChatSession({ id: "chat-1", title: "Chat 1" });

    const result = addChatSession([initial], "chat-2");

    expect(result.activeSessionId).toBe("chat-2");
    expect(result.sessions.map((session) => session.id)).toEqual(["chat-1", "chat-2"]);
    expect(result.sessions[1]).toMatchObject({ id: "chat-2", title: "Chat 2" });
  });

  it("closes the active session and activates a nearby remaining session", () => {
    const sessions = [
      createChatSession({ id: "chat-1", title: "Chat 1" }),
      createChatSession({ id: "chat-2", title: "Chat 2" }),
      createChatSession({ id: "chat-3", title: "Chat 3" }),
    ];

    const result = closeChatSession(sessions, "chat-2", "chat-2");

    expect(result.sessions.map((session) => session.id)).toEqual(["chat-1", "chat-3"]);
    expect(result.activeSessionId).toBe("chat-3");
  });

  it("closing the only session wipes it and leaves one empty session", () => {
    const result = closeChatSession(
      [
        createChatSession({
          id: "chat-1",
          title: "Draft",
          messages: [userMessage],
          prompt: "half-written",
        }),
      ],
      "chat-1",
      "chat-1",
    );

    expect(result.sessions).toHaveLength(1);
    expect(result.activeSessionId).toBe(result.sessions[0].id);
    expect(result.sessions[0]).toMatchObject({
      title: "Chat 1",
      messages: [],
      prompt: "",
    });
  });

  it("updates messages and prompt immutably for a matching session", () => {
    const sessions = [
      createChatSession({ id: "chat-1", title: "Chat 1" }),
      createChatSession({ id: "chat-2", title: "Chat 2" }),
    ];

    const withMessages = updateChatSessionMessages(sessions, "chat-2", (messages) => [
      ...messages,
      userMessage,
    ]);
    const withPrompt = updateChatSessionPrompt(withMessages, "chat-2", "Next prompt");

    expect(withPrompt[0]).toBe(sessions[0]);
    expect(withPrompt[1]).not.toBe(sessions[1]);
    expect(withPrompt[1].messages).toEqual([userMessage]);
    expect(withPrompt[1].prompt).toBe("Next prompt");
  });
});
