import { FileText, FolderOpen, ListChecks, Sparkles } from "lucide-react";
import type { FolderManifest } from "../lib/folderIndex";
import type { SummaryResult } from "../lib/summarization";

type DirectoryInput = HTMLInputElement & { webkitdirectory: boolean };

export interface FolderPanelProps {
  selectedFileCount: number;
  manifest: FolderManifest | null;
  summaryResult: SummaryResult | null;
  summaryMode: "provider" | "deterministic";
  hasDirectoryPicker: boolean;
  isSelectingDirectory: boolean;
  isIndexing: boolean;
  isSummarizing: boolean;
  error: string | null;
  onFilesSelected: (files: FileList | null) => void;
  onChooseDirectory: () => void;
  onIndexFolder: () => void;
  onSummarizeFolder: () => void;
}

function pluralize(count: number, singular: string, plural = `${singular}s`) {
  return `${count} ${count === 1 ? singular : plural}`;
}

function manifestCountText(manifest: FolderManifest | null): string {
  if (!manifest) {
    return "No manifest yet";
  }

  const { stats } = manifest;

  return [
    pluralize(stats.indexedFiles, "indexed file"),
    pluralize(stats.ignoredFiles, "ignored file"),
    pluralize(stats.errorFiles, "error", "errors"),
    pluralize(stats.chunkCount, "chunk"),
  ].join(" · ");
}

const visibleTopicLimit = 10;

export function FolderPanel({
  selectedFileCount,
  manifest,
  summaryResult,
  summaryMode,
  hasDirectoryPicker,
  isSelectingDirectory,
  isIndexing,
  isSummarizing,
  error,
  onFilesSelected,
  onChooseDirectory,
  onIndexFolder,
  onSummarizeFolder,
}: FolderPanelProps) {
  const isBusy = isSelectingDirectory || isIndexing || isSummarizing;
  const canIndex = selectedFileCount > 0 && !isBusy;
  const canSummarize = (manifest?.stats.indexedFiles ?? 0) > 0 && !isBusy;
  const folderSummary = summaryResult?.folderSummary;
  const visibleTopics = folderSummary?.topics.slice(0, visibleTopicLimit) ?? [];
  const hiddenTopicCount = Math.max(
    0,
    (folderSummary?.topics.length ?? 0) - visibleTopicLimit,
  );

  return (
    <section className="folder-panel" aria-label="Folder workspace">
      <header className="panel-header">
        <div>
          <h2>Folder</h2>
          <p>Choose source files, index text chunks, then summarize the folder.</p>
        </div>
      </header>

      <div className="folder-source-grid">
        {hasDirectoryPicker ? (
          <button
            type="button"
            className="secondary-button"
            data-testid="directory-picker-button"
            disabled={isBusy}
            onClick={onChooseDirectory}
          >
            <FolderOpen size={16} aria-hidden="true" />
            <span>{isSelectingDirectory ? "Choosing" : "Choose folder"}</span>
          </button>
        ) : null}
        <label className="file-picker">
          <span>
            <FolderOpen size={16} aria-hidden="true" />
            Folder files
          </span>
          <input
            ref={(node) => {
              if (node) {
                (node as DirectoryInput).webkitdirectory = true;
              }
            }}
            type="file"
            multiple
            data-testid="folder-file-input"
            disabled={isBusy}
            onChange={(event) => {
              onFilesSelected(event.currentTarget.files);
              event.currentTarget.value = "";
            }}
          />
        </label>
      </div>

      <p className="folder-source-hint">
        {hasDirectoryPicker
          ? "Direct folder access available. The file input remains as a fallback."
          : "Use the folder file input fallback for broad browser and Windows compatibility."}
      </p>

      <div className="folder-actions">
        <button
          type="button"
          className="secondary-button"
          data-testid="index-folder-button"
          disabled={!canIndex}
          onClick={onIndexFolder}
        >
          <ListChecks size={16} aria-hidden="true" />
          <span>{isIndexing ? "Indexing" : "Index folder"}</span>
        </button>
        <button
          type="button"
          className="primary-button"
          data-testid="summarize-folder-button"
          disabled={!canSummarize}
          onClick={onSummarizeFolder}
        >
          <Sparkles size={16} aria-hidden="true" />
          <span>{isSummarizing ? "Summarizing" : "Summarize"}</span>
        </button>
      </div>

      <div className="folder-status" data-testid="folder-file-count">
        <FileText size={16} aria-hidden="true" />
        <span>
          {pluralize(selectedFileCount, "selected file")} ·{" "}
          {manifestCountText(manifest)}
        </span>
      </div>

      {error ? (
        <p className="status-card blocked" role="alert">
          {error}
        </p>
      ) : null}

      <article className="folder-summary" data-testid="folder-summary">
        <h3>Summary</h3>
        <span className="summary-mode">
          {summaryMode === "provider"
            ? "Summaries use the loaded Gemma provider."
            : "Summaries use deterministic preview until a provider is loaded."}
        </span>
        <p>{folderSummary?.text ?? "No folder summary yet."}</p>
        {visibleTopics.length ? (
          <ul aria-label="Summary topics">
            {visibleTopics.map((topic) => (
              <li key={topic}>{topic}</li>
            ))}
            {hiddenTopicCount > 0 ? <li>+{hiddenTopicCount} more</li> : null}
          </ul>
        ) : null}
      </article>
    </section>
  );
}
