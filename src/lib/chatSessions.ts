import type { ChatMessage } from "./chatProvider";

export interface ChatSession {
  id: string;
  title: string;
  messages: ChatMessage[];
  prompt: string;
}

export interface CreateChatSessionOptions {
  id?: string;
  title?: string;
  messages?: ChatMessage[];
  prompt?: string;
}

export interface ChatSessionResult {
  sessions: ChatSession[];
  activeSessionId: string;
}

function createSessionId(): string {
  return globalThis.crypto?.randomUUID?.() ?? `chat-${Date.now()}`;
}

function createSessionTitle(index: number): string {
  return `Chat ${index}`;
}

export function createChatSession({
  id = createSessionId(),
  title = createSessionTitle(1),
  messages = [],
  prompt = "",
}: CreateChatSessionOptions = {}): ChatSession {
  return {
    id,
    title,
    messages,
    prompt,
  };
}

export function createInitialChatSession(): ChatSession {
  return createChatSession({ title: createSessionTitle(1) });
}

export function addChatSession(sessions: ChatSession[], id?: string): ChatSessionResult {
  const session = createChatSession({
    id,
    title: createSessionTitle(sessions.length + 1),
  });

  return {
    sessions: [...sessions, session],
    activeSessionId: session.id,
  };
}

export function closeChatSession(
  sessions: ChatSession[],
  activeSessionId: string,
  closingSessionId: string,
): ChatSessionResult {
  if (sessions.length <= 1) {
    const session = createInitialChatSession();

    return {
      sessions: [session],
      activeSessionId: session.id,
    };
  }

  const closingIndex = sessions.findIndex((session) => session.id === closingSessionId);

  if (closingIndex === -1) {
    return { sessions, activeSessionId };
  }

  const nextSessions = sessions.filter((session) => session.id !== closingSessionId);

  if (activeSessionId !== closingSessionId) {
    return {
      sessions: nextSessions,
      activeSessionId: nextSessions.some((session) => session.id === activeSessionId)
        ? activeSessionId
        : nextSessions[0].id,
    };
  }

  const nextActiveSession = nextSessions[Math.min(closingIndex, nextSessions.length - 1)];

  return {
    sessions: nextSessions,
    activeSessionId: nextActiveSession.id,
  };
}

export function updateChatSessionMessages(
  sessions: ChatSession[],
  sessionId: string,
  updater: (messages: ChatMessage[]) => ChatMessage[],
): ChatSession[] {
  return sessions.map((session) =>
    session.id === sessionId
      ? {
          ...session,
          messages: updater(session.messages),
        }
      : session,
  );
}

export function updateChatSessionPrompt(
  sessions: ChatSession[],
  sessionId: string,
  prompt: string,
): ChatSession[] {
  return sessions.map((session) =>
    session.id === sessionId
      ? {
          ...session,
          prompt,
        }
      : session,
  );
}
