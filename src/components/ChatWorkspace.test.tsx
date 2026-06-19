import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, mock } from "bun:test";
import { ChatWorkspace, type ChatWorkspaceProps } from "./ChatWorkspace";
import { createChatSession } from "../lib/chatSessions";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT =
  true;

let root: Root | null = null;
let container: HTMLDivElement | null = null;

function getByTestId<T extends Element = Element>(testId: string): T {
  const element = container?.querySelector(`[data-testid="${testId}"]`);

  if (!element) {
    throw new Error(`Unable to find element with data-testid="${testId}".`);
  }

  return element as T;
}

const defaultProps: ChatWorkspaceProps = {
  messages: [],
  prompt: "",
  isGenerating: false,
  modelLoaded: true,
  attachmentsEnabled: true,
  attachments: [],
  attachmentStatus: "Native image and audio attachments available.",
  onPromptChange: () => undefined,
  onSend: () => undefined,
  onStop: () => undefined,
  onAttachmentsSelected: () => undefined,
  onRemoveAttachment: () => undefined,
};

async function renderChatWorkspace(props: Partial<ChatWorkspaceProps> = {}) {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);

  await act(async () => {
    root?.render(<ChatWorkspace {...defaultProps} {...props} />);
  });
}

function fileListOf(file: File): FileList {
  return {
    0: file,
    length: 1,
    item: (index: number) => (index === 0 ? file : null),
  } as unknown as FileList;
}

function setNativeValue(element: HTMLInputElement | HTMLTextAreaElement, value: string) {
  const valueSetter = Object.getOwnPropertyDescriptor(element, "value")?.set;
  const prototype = Object.getPrototypeOf(element) as HTMLInputElement | HTMLTextAreaElement;
  const prototypeValueSetter = Object.getOwnPropertyDescriptor(prototype, "value")?.set;

  if (prototypeValueSetter && valueSetter !== prototypeValueSetter) {
    prototypeValueSetter.call(element, value);
    return;
  }

  valueSetter?.call(element, value);
}

describe("ChatWorkspace", () => {
  afterEach(() => {
    act(() => {
      root?.unmount();
    });
    container?.remove();
    root = null;
    container = null;
  });

  it("shows and clears selected native attachments", async () => {
    const onAttachmentsSelected = mock();
    const onRemoveAttachment = mock();

    await renderChatWorkspace({
      attachments: [{ id: "diagram", name: "diagram.png", mimeType: "image/png" }],
      onAttachmentsSelected,
      onRemoveAttachment,
    });

    expect(container?.textContent).toContain("diagram.png");

    const input = getByTestId<HTMLInputElement>("attachment-input");
    Object.defineProperty(input, "files", {
      configurable: true,
      value: fileListOf(new File(["image"], "sample.png", { type: "image/png" })),
    });

    await act(async () => {
      input.dispatchEvent(new Event("change", { bubbles: true }));
    });

    expect(onAttachmentsSelected).toHaveBeenCalledTimes(1);

    await act(async () => {
      getByTestId<HTMLButtonElement>("remove-attachment-diagram").dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(onRemoveAttachment).toHaveBeenCalledWith("diagram");
  });

  it("disables native attachments when the current provider cannot use them", async () => {
    await renderChatWorkspace({
      attachmentsEnabled: false,
      attachmentStatus: "Connect the executable sidecar for native attachments.",
    });

    expect(getByTestId<HTMLInputElement>("attachment-input").disabled).toBe(true);
    expect(container?.textContent).toContain(
      "Connect the executable sidecar for native attachments.",
    );
  });

  it("renders chat tabs and routes tab actions to callbacks", async () => {
    const onActiveSessionChange = mock();
    const onNewSession = mock();
    const onCloseSession = mock();

    await renderChatWorkspace({
      sessions: [
        createChatSession({ id: "chat-1", title: "Planning" }),
        createChatSession({ id: "chat-2", title: "Runtime" }),
      ],
      activeSessionId: "chat-1",
      onActiveSessionChange,
      onNewSession,
      onCloseSession,
    });

    expect(getByTestId<HTMLButtonElement>("chat-tab-chat-1").getAttribute("aria-selected")).toBe(
      "true",
    );
    expect(container?.textContent).toContain("Planning");
    expect(container?.textContent).toContain("Runtime");

    await act(async () => {
      getByTestId<HTMLButtonElement>("chat-tab-chat-2").dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    await act(async () => {
      getByTestId<HTMLButtonElement>("new-chat-button").dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    await act(async () => {
      getByTestId<HTMLButtonElement>("close-chat-chat-1").dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(onActiveSessionChange).toHaveBeenCalledWith("chat-2");
    expect(onNewSession).toHaveBeenCalledTimes(1);
    expect(onCloseSession).toHaveBeenCalledWith("chat-1");
  });

  it("uses the active session transcript and prompt", async () => {
    await renderChatWorkspace({
      sessions: [
        createChatSession({
          id: "chat-1",
          title: "Planning",
          messages: [{ id: "m1", role: "user", text: "Hidden", status: "complete" }],
          prompt: "hidden prompt",
        }),
        createChatSession({
          id: "chat-2",
          title: "Runtime",
          messages: [{ id: "m2", role: "user", text: "Visible", status: "complete" }],
          prompt: "visible prompt",
        }),
      ],
      activeSessionId: "chat-2",
    });

    expect(container?.textContent).toContain("Visible");
    expect(container?.textContent).not.toContain("Hidden");
    expect(getByTestId<HTMLTextAreaElement>("prompt-input").value).toBe("visible prompt");
  });

  it("sends on Enter and keeps Shift+Enter available for a newline", async () => {
    const onSend = mock();

    await renderChatWorkspace({ prompt: "Explain runtime", onSend });

    const promptInput = getByTestId<HTMLTextAreaElement>("prompt-input");
    const enter = new KeyboardEvent("keydown", {
      key: "Enter",
      bubbles: true,
      cancelable: true,
    });
    const shiftEnter = new KeyboardEvent("keydown", {
      key: "Enter",
      shiftKey: true,
      bubbles: true,
      cancelable: true,
    });

    await act(async () => {
      promptInput.dispatchEvent(shiftEnter);
    });

    await act(async () => {
      promptInput.dispatchEvent(enter);
    });

    expect(shiftEnter.defaultPrevented).toBe(false);
    expect(enter.defaultPrevented).toBe(true);
    expect(onSend).toHaveBeenCalledTimes(1);
  });

  it("groups the prompt and action controls in one compact composer surface", async () => {
    const onSend = mock();

    await renderChatWorkspace({ prompt: "Explain runtime", onSend });

    const promptInput = getByTestId<HTMLTextAreaElement>("prompt-input");
    const promptSurface = promptInput.closest(".prompt-surface");
    const promptActions = promptSurface?.querySelector(".prompt-actions");

    expect(promptSurface).not.toBeNull();
    expect(promptActions).not.toBeNull();
    expect(promptActions?.getAttribute("aria-label")).toBe("Prompt actions");
    expect(getByTestId<HTMLInputElement>("attachment-input").closest(".prompt-surface")).toBe(
      promptSurface,
    );
    expect(getByTestId<HTMLButtonElement>("stop-button").closest(".prompt-surface")).toBe(
      promptSurface,
    );
    expect(getByTestId<HTMLButtonElement>("send-button").closest(".prompt-surface")).toBe(
      promptSurface,
    );
    expect(promptActions?.contains(getByTestId<HTMLInputElement>("attachment-input"))).toBe(
      true,
    );
    expect(promptActions?.contains(getByTestId<HTMLButtonElement>("stop-button"))).toBe(
      true,
    );
    expect(promptActions?.contains(getByTestId<HTMLButtonElement>("send-button"))).toBe(
      true,
    );

    await act(async () => {
      getByTestId<HTMLButtonElement>("send-button").dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(onSend).toHaveBeenCalledTimes(1);
  });

  it("renders model option controls and forwards changes", async () => {
    const onSystemPromptChange = mock();
    const onContextLengthChange = mock();
    const onThinkingEnabledChange = mock();

    await renderChatWorkspace({
      systemPrompt: "Be concise.",
      contextLength: 4096,
      thinkingEnabled: true,
      onSystemPromptChange,
      onContextLengthChange,
      onThinkingEnabledChange,
    });

    expect(getByTestId<HTMLTextAreaElement>("system-prompt-input").value).toBe("Be concise.");
    expect(getByTestId<HTMLInputElement>("context-length-input").value).toBe("4096");
    expect(getByTestId<HTMLInputElement>("thinking-toggle").checked).toBe(true);

    await act(async () => {
      const input = getByTestId<HTMLTextAreaElement>("system-prompt-input");
      setNativeValue(input, "Short.");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      const input = getByTestId<HTMLInputElement>("context-length-input");
      setNativeValue(input, "8192");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => {
      const input = getByTestId<HTMLInputElement>("thinking-toggle");
      input.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(onSystemPromptChange).toHaveBeenCalled();
    expect(onContextLengthChange).toHaveBeenCalledWith(8192);
    expect(onThinkingEnabledChange).toHaveBeenCalledWith(false);
  });

  it("keeps chat options collapsed and the prompt input compact", async () => {
    await renderChatWorkspace();

    const details = getByTestId<HTMLDetailsElement>("chat-options");
    const promptInput = getByTestId<HTMLTextAreaElement>("prompt-input");

    expect(details.open).toBe(false);
    expect(promptInput.rows).toBe(2);
  });

  it("shows realtime token throughput when available", async () => {
    await renderChatWorkspace({ tokensPerSecond: 12.4 });

    expect(getByTestId("tokens-per-second").textContent).toBe("12.4 tok/s");
  });

  it("renders collapsible chat debug output", async () => {
    await renderChatWorkspace({
      debugLines: ["provider=web", "tokens=12"],
    });

    const details = getByTestId<HTMLDetailsElement>("chat-debug-output");

    expect(details.tagName).toBe("DETAILS");
    expect(details.open).toBe(false);
    expect(details.textContent).toContain("provider=web");
    expect(details.textContent).toContain("tokens=12");
  });
});
