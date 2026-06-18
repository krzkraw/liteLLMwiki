import { Bot, Plus, Paperclip, Send, Square, User, X } from "lucide-react";
import type { ChangeEvent, FormEvent, KeyboardEvent } from "react";
import type { ChatAttachment, ChatMessage } from "../lib/chatProvider";
import type { ChatSession } from "../lib/chatSessions";

export interface ChatWorkspaceProps {
  messages?: ChatMessage[];
  prompt?: string;
  sessions?: ChatSession[];
  activeSessionId?: string;
  isGenerating: boolean;
  modelLoaded: boolean;
  attachmentsEnabled: boolean;
  attachments: Pick<ChatAttachment, "id" | "name" | "mimeType">[];
  attachmentStatus: string;
  systemPrompt?: string;
  contextLength?: number;
  thinkingEnabled?: boolean;
  tokensPerSecond?: number | null;
  debugLines?: string[];
  onActiveSessionChange?: (id: string) => void;
  onNewSession?: () => void;
  onCloseSession?: (id: string) => void;
  onPromptChange: (prompt: string) => void;
  onSend: () => void;
  onStop: () => void;
  onAttachmentsSelected: (files: FileList | null) => void;
  onRemoveAttachment: (id: string) => void;
  onSystemPromptChange?: (prompt: string) => void;
  onContextLengthChange?: (contextLength: number) => void;
  onThinkingEnabledChange?: (thinkingEnabled: boolean) => void;
}

export function ChatWorkspace({
  messages = [],
  prompt = "",
  sessions,
  activeSessionId,
  isGenerating,
  modelLoaded,
  attachmentsEnabled,
  attachments,
  attachmentStatus,
  systemPrompt = "",
  contextLength = 4096,
  thinkingEnabled = false,
  tokensPerSecond = null,
  debugLines = [],
  onActiveSessionChange,
  onNewSession,
  onCloseSession,
  onPromptChange,
  onSend,
  onStop,
  onAttachmentsSelected,
  onRemoveAttachment,
  onSystemPromptChange,
  onContextLengthChange,
  onThinkingEnabledChange,
}: ChatWorkspaceProps) {
  const activeSession =
    sessions?.find((session) => session.id === activeSessionId) ?? sessions?.[0];
  const visibleMessages = activeSession?.messages ?? messages;
  const visiblePrompt = activeSession?.prompt ?? prompt;
  const canSend = modelLoaded && !isGenerating && visiblePrompt.trim().length > 0;
  const lastMessage = visibleMessages.at(-1);
  const liveStatus = isGenerating
    ? "Generating response."
    : lastMessage?.role === "assistant" &&
        lastMessage.status === "complete" &&
        visibleMessages.length > 1
      ? "Response complete."
      : lastMessage?.role === "assistant" && lastMessage.status === "error"
        ? "Response failed."
        : "";

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    if (canSend) {
      onSend();
    }
  }

  function handleAttachmentsSelected(event: ChangeEvent<HTMLInputElement>) {
    onAttachmentsSelected(event.target.files);
    event.target.value = "";
  }

  function handlePromptKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key !== "Enter" || event.shiftKey) {
      return;
    }

    event.preventDefault();

    if (canSend) {
      onSend();
    }
  }

  return (
    <section className="chat-panel" aria-label="Chat workspace">
      <header className="chat-header">
        <div>
          <h2>Local chat</h2>
          <p>Prompt the loaded Gemma provider and watch streamed text responses.</p>
        </div>
        <div className="chat-header-meters">
          {tokensPerSecond !== null ? (
            <span className="session-pill ready" data-testid="tokens-per-second">
              {tokensPerSecond.toFixed(1)} tok/s
            </span>
          ) : null}
          <span className={`session-pill ${modelLoaded ? "ready" : "idle"}`}>
            {modelLoaded ? "Provider loaded" : "Provider idle"}
          </span>
        </div>
      </header>

      {sessions ? (
        <div className="chat-tabs" role="tablist" aria-label="Chats">
          {sessions.map((session) => {
            const isActive = session.id === activeSession?.id;

            return (
              <div key={session.id} className="chat-tab-group">
                <button
                  type="button"
                  role="tab"
                  className={`chat-tab ${isActive ? "active" : ""}`}
                  aria-selected={isActive}
                  data-testid={`chat-tab-${session.id}`}
                  onClick={() => onActiveSessionChange?.(session.id)}
                >
                  <span>{session.title}</span>
                </button>
                <button
                  type="button"
                  aria-label={`Close ${session.title}`}
                  data-testid={`close-chat-${session.id}`}
                  className="chat-tab-close"
                  onClick={() => onCloseSession?.(session.id)}
                >
                  <X size={13} aria-hidden="true" />
                </button>
              </div>
            );
          })}
          <button
            type="button"
            className="chat-tab new-chat"
            data-testid="new-chat-button"
            aria-label="New chat"
            onClick={onNewSession}
          >
            <Plus size={14} aria-hidden="true" />
          </button>
        </div>
      ) : null}

      <div className="sr-only" aria-live="polite" aria-atomic="true">
        {liveStatus}
      </div>

      <div className="messages">
        {visibleMessages.map((message) => (
          <article key={message.id} className={`message ${message.role}`}>
            <div className="avatar" aria-hidden="true">
              {message.role === "user" ? <User size={17} /> : <Bot size={17} />}
            </div>
            <div
              className="bubble"
              data-testid={
                message.role === "user" ? "chat-message-user" : "chat-message-assistant"
              }
            >
              <p>{message.text || (message.status === "streaming" ? "Thinking..." : "")}</p>
              {message.attachmentNames?.length ? (
                <ul className="message-attachments" aria-label="Attached files">
                  {message.attachmentNames.map((name) => (
                    <li key={name}>{name}</li>
                  ))}
                </ul>
              ) : null}
              {message.status === "error" ? <span className="message-status">Error</span> : null}
            </div>
          </article>
        ))}
      </div>

      <details className="debug-log chat-debug-log" data-testid="chat-debug-output">
        <summary>Chat debug</summary>
        <pre data-testid="chat-debug-lines">
          {debugLines.length > 0 ? debugLines.join("\n") : "No chat debug output yet."}
        </pre>
      </details>

      <form className="composer" onSubmit={handleSubmit}>
        <details className="chat-options" data-testid="chat-options">
          <summary>Chat options</summary>
          <label>
            <span>System</span>
            <textarea
              value={systemPrompt}
              data-testid="system-prompt-input"
              rows={2}
              placeholder="System prompt"
              onChange={(event) => onSystemPromptChange?.(event.target.value)}
            />
          </label>
          <label>
            <span>Context</span>
            <input
              type="number"
              min={512}
              step={512}
              value={contextLength}
              data-testid="context-length-input"
              onChange={(event) => onContextLengthChange?.(event.target.valueAsNumber)}
            />
          </label>
          <label className="thinking-toggle">
            <input
              type="checkbox"
              checked={thinkingEnabled}
              data-testid="thinking-toggle"
              onChange={(event) => onThinkingEnabledChange?.(event.target.checked)}
            />
            <span>Thinking</span>
          </label>
        </details>

        <div className="prompt-surface">
          <label className="prompt-field">
            <span className="sr-only">Prompt</span>
            <textarea
              value={visiblePrompt}
              data-testid="prompt-input"
              onChange={(event) => onPromptChange(event.target.value)}
              onKeyDown={handlePromptKeyDown}
              rows={2}
              placeholder="Ask Gemma about code, notes, or a local workflow..."
            />
          </label>

          <div className="prompt-footer">
            <div className="attachment-meta">
              <p className="attachment-status">{attachmentStatus}</p>
              {attachments.length > 0 ? (
                <ul className="attachment-list" aria-label="Selected attachments">
                  {attachments.map((attachment) => (
                    <li key={attachment.id}>
                      <span>{attachment.name}</span>
                      <button
                        type="button"
                        data-testid={`remove-attachment-${attachment.id}`}
                        aria-label={`Remove ${attachment.name}`}
                        disabled={isGenerating}
                        onClick={() => onRemoveAttachment(attachment.id)}
                      >
                        <X size={14} aria-hidden="true" />
                      </button>
                    </li>
                  ))}
                </ul>
              ) : null}
            </div>

            <div className="prompt-actions" aria-label="Prompt actions">
              <label className="file-picker attachment-picker pill-action">
                <Paperclip size={16} aria-hidden="true" />
                <span>Attach</span>
                <input
                  type="file"
                  accept="image/*,audio/*"
                  multiple
                  data-testid="attachment-input"
                  disabled={!attachmentsEnabled || isGenerating}
                  onChange={handleAttachmentsSelected}
                />
              </label>
              <button
                type="button"
                className="secondary-button pill-action"
                data-testid="stop-button"
                onClick={onStop}
                disabled={!isGenerating}
              >
                <Square size={16} aria-hidden="true" />
                <span>Stop</span>
              </button>
              <button
                type="submit"
                className="primary-button pill-action"
                data-testid="send-button"
                disabled={!canSend}
              >
                <Send size={16} aria-hidden="true" />
                <span>Run</span>
              </button>
            </div>
          </div>
        </div>
      </form>
    </section>
  );
}
