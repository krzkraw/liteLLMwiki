import type { ChatMessage } from "./chatProvider";

export interface AssistantTurn {
  messages: ChatMessage[];
  assistantId: string;
}

function createMessageId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }

  return `msg-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

export function startAssistantTurn(
  messages: ChatMessage[],
  userText: string,
  attachmentNames: string[] = [],
): AssistantTurn {
  const assistantId = createMessageId();

  return {
    messages: [
      ...messages,
      {
        id: createMessageId(),
        role: "user",
        text: userText,
        status: "complete",
        attachmentNames,
      },
      {
        id: assistantId,
        role: "assistant",
        text: "",
        status: "streaming",
      },
    ],
    assistantId,
  };
}

export function appendAssistantText(
  messages: ChatMessage[],
  assistantId: string,
  text: string,
): ChatMessage[] {
  return messages.map((message) =>
    message.id === assistantId
      ? { ...message, text: `${message.text}${text}`, status: "streaming" }
      : message,
  );
}

export function failAssistantMessage(
  messages: ChatMessage[],
  assistantId: string,
  errorText: string,
): ChatMessage[] {
  return messages.map((message) =>
    message.id === assistantId
      ? { ...message, text: errorText, status: "error" }
      : message,
  );
}
