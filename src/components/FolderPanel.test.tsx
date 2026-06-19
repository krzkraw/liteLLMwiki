import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, mock } from "bun:test";
import { FolderPanel } from "./FolderPanel";
import type { FolderManifest } from "../lib/folderIndex";
import type { SummaryResult } from "../lib/summarization";

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

async function renderFolderPanel(
  props: Partial<React.ComponentProps<typeof FolderPanel>> = {},
) {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);

  await act(async () => {
    root?.render(
      <FolderPanel
        selectedFileCount={0}
        manifest={null}
        summaryResult={null}
        hasDirectoryPicker={false}
        isSelectingDirectory={false}
        isIndexing={false}
        isSummarizing={false}
        summaryMode="deterministic"
        error={null}
        onFilesSelected={() => undefined}
        onChooseDirectory={() => undefined}
        onIndexFolder={() => undefined}
        onSummarizeFolder={() => undefined}
        {...props}
      />,
    );
  });
}

describe("FolderPanel", () => {
  afterEach(() => {
    act(() => {
      root?.unmount();
    });
    container?.remove();
    root = null;
    container = null;
  });

  it("renders folder selection controls with directory file input", async () => {
    const onFilesSelected = mock();

    await renderFolderPanel({ onFilesSelected });

    const input = getByTestId<HTMLInputElement>("folder-file-input");

    expect(input.type).toBe("file");
    expect(input.multiple).toBe(true);
    expect((input as HTMLInputElement & { webkitdirectory?: boolean }).webkitdirectory).toBe(
      true,
    );
    expect(getByTestId("index-folder-button")).toBeInstanceOf(HTMLButtonElement);
    expect(getByTestId("summarize-folder-button")).toBeInstanceOf(HTMLButtonElement);

    await act(async () => {
      input.dispatchEvent(new Event("change", { bubbles: true }));
    });

    expect(onFilesSelected).toHaveBeenCalledWith(input.files);
    expect(input.value).toBe("");
  });

  it("offers the direct directory picker when the browser supports it", async () => {
    const onChooseDirectory = mock();

    await renderFolderPanel({
      hasDirectoryPicker: true,
      onChooseDirectory,
    });

    const button = getByTestId<HTMLButtonElement>("directory-picker-button");
    expect(button.textContent).toContain("Choose folder");
    expect(container?.textContent).toContain("Direct folder access available");

    await act(async () => {
      button.click();
    });

    expect(onChooseDirectory).toHaveBeenCalledTimes(1);
  });

  it("keeps the direct directory picker hidden when only file-list fallback is available", async () => {
    await renderFolderPanel({ hasDirectoryPicker: false });

    expect(container?.querySelector('[data-testid="directory-picker-button"]')).toBeNull();
    expect(container?.textContent).toContain("Use the folder file input fallback");
  });

  it("shows selected and indexed counts plus the folder summary", async () => {
    const manifest = {
      stats: {
        totalFiles: 4,
        indexedFiles: 2,
        ignoredFiles: 1,
        errorFiles: 1,
        chunkCount: 6,
      },
    } as FolderManifest;
    const summaryResult = {
      folderSummary: {
        text: "Two source files describe the provider runtime.",
        topics: ["Provider", "Runtime"],
      },
    } as SummaryResult;

    await renderFolderPanel({
      selectedFileCount: 4,
      manifest,
      summaryResult,
    });

    expect(getByTestId("folder-file-count").textContent).toContain("4 selected");
    expect(getByTestId("folder-file-count").textContent).toContain(
      "4 selected files",
    );
    expect(getByTestId("folder-file-count").textContent).toContain("2 indexed");
    expect(getByTestId("folder-file-count").textContent).toContain(
      "2 indexed files",
    );
    expect(getByTestId("folder-file-count").textContent).toContain("6 chunks");
    expect(getByTestId("folder-file-count").textContent).not.toContain(
      "selecteds",
    );
    expect(getByTestId("folder-file-count").textContent).not.toContain(
      "indexeds",
    );
    expect(getByTestId("folder-summary").textContent).toContain(
      "Two source files describe the provider runtime.",
    );
    expect(getByTestId("folder-summary").textContent).toContain("Provider");
  });

  it("shows whether folder summaries use loaded Gemma or deterministic preview", async () => {
    await renderFolderPanel({ summaryMode: "provider" });

    expect(getByTestId("folder-summary").textContent).toContain(
      "Summaries use the loaded Gemma provider.",
    );

    await act(async () => {
      root?.render(
        <FolderPanel
          selectedFileCount={0}
          manifest={null}
          summaryResult={null}
          hasDirectoryPicker={false}
          isSelectingDirectory={false}
          isIndexing={false}
          isSummarizing={false}
          summaryMode="deterministic"
          error={null}
          onFilesSelected={() => undefined}
          onChooseDirectory={() => undefined}
          onIndexFolder={() => undefined}
          onSummarizeFolder={() => undefined}
        />,
      );
    });

    expect(getByTestId("folder-summary").textContent).toContain(
      "Summaries use deterministic preview until a provider is loaded.",
    );
  });

  it("caps long topic lists with an overflow count", async () => {
    const summaryResult = {
      folderSummary: {
        text: "Many topics.",
        topics: Array.from({ length: 14 }, (_, index) => `Topic ${index + 1}`),
      },
    } as SummaryResult;

    await renderFolderPanel({
      selectedFileCount: 14,
      summaryResult,
    });

    expect(getByTestId("folder-summary").textContent).toContain("Topic 10");
    expect(getByTestId("folder-summary").textContent).toContain("+4 more");
    expect(getByTestId("folder-summary").textContent).not.toContain("Topic 14");
  });

  it("disables actions while busy or when prerequisites are missing", async () => {
    await renderFolderPanel({
      selectedFileCount: 0,
      isIndexing: true,
      error: "Index failed",
    });

    expect(getByTestId<HTMLButtonElement>("index-folder-button").disabled).toBe(true);
    expect(getByTestId<HTMLButtonElement>("summarize-folder-button").disabled).toBe(true);
    expect(container?.textContent).toContain("Index failed");
  });
});
