import type { FolderManifest, IndexedFile, TextChunk } from "./folderIndex";

type Awaitable<T> = T | Promise<T>;

export interface ChunkSummary {
  chunkId: string;
  fileId: string;
  path: string;
  summary: string;
  entities: string[];
}

export interface FileSummary {
  fileId: string;
  path: string;
  summary: string;
  entities: string[];
}

export interface FolderSummary {
  text: string;
  topics: string[];
}

export interface SummaryResult {
  chunkSummaries: ChunkSummary[];
  fileSummaries: FileSummary[];
  folderSummary: FolderSummary;
}

export interface SummaryFragment {
  summary: string;
  entities: string[];
}

export interface FolderSummarizer {
  summarizeChunk(chunk: TextChunk): Awaitable<SummaryFragment>;
  summarizeFile(
    file: IndexedFile,
    chunks: TextChunk[],
    chunkSummaries: ChunkSummary[],
  ): Awaitable<SummaryFragment>;
  summarizeFolder(
    manifest: FolderManifest,
    fileSummaries: FileSummary[],
  ): Awaitable<FolderSummary>;
}

export interface TextGenerationProvider {
  generateText(prompt: string, signal?: AbortSignal): Promise<string>;
}

export interface ProviderSummarizerOptions {
  maxPromptChars?: number;
  signal?: AbortSignal;
}

const commonEntityWords = new Set([
  "about",
  "after",
  "again",
  "also",
  "and",
  "are",
  "async",
  "await",
  "boolean",
  "chunk",
  "chunks",
  "class",
  "const",
  "default",
  "demo",
  "export",
  "false",
  "file",
  "files",
  "for",
  "from",
  "function",
  "have",
  "import",
  "indexed",
  "interface",
  "into",
  "let",
  "main",
  "null",
  "number",
  "plain",
  "return",
  "source",
  "src",
  "string",
  "that",
  "the",
  "this",
  "true",
  "type",
  "undefined",
  "uses",
  "using",
  "var",
  "with",
]);

function normalizeWhitespace(value: string): string {
  return value.replace(/\s+/g, " ").trim();
}

function truncateText(value: string, maxLength: number): string {
  const normalized = normalizeWhitespace(value);

  if (normalized.length <= maxLength) {
    return normalized;
  }

  return `${normalized.slice(0, Math.max(0, maxLength - 3)).trimEnd()}...`;
}

function singularize(count: number, singular: string, plural = `${singular}s`) {
  return `${count} ${count === 1 ? singular : plural}`;
}

function uniqueStable(values: string[]): string[] {
  const seen = new Set<string>();
  const output: string[] = [];

  for (const value of values) {
    const normalized = normalizeWhitespace(value);
    const key = normalized.toLowerCase();

    if (!normalized || seen.has(key)) {
      continue;
    }

    seen.add(key);
    output.push(normalized);
  }

  return output;
}

function sortedUnique(values: string[]): string[] {
  return uniqueStable(values).sort((left, right) =>
    left.toLowerCase().localeCompare(right.toLowerCase()),
  );
}

function chunkRef(fileId: string, chunkId: string): string {
  return `${fileId}\0${chunkId}`;
}

function extractEntities(text: string, limit = 12): string[] {
  const matches = text.match(/[A-Za-z_][A-Za-z0-9_]{2,}/g) ?? [];
  const entities = matches.filter((match) => {
    const normalized = match.toLowerCase();
    return !commonEntityWords.has(normalized);
  });

  return sortedUnique(entities).slice(0, limit);
}

function summarizeText(text: string, entities: string[]): string {
  const excerpt = truncateText(text, 160);
  const entityList = entities.slice(0, 6).join(", ");

  if (!excerpt && entityList) {
    return `Mentions ${entityList}.`;
  }

  if (!entityList) {
    return excerpt || "No text content.";
  }

  return `${excerpt} Key terms: ${entityList}.`;
}

function splitEntityLine(value: string): string[] {
  return value
    .split(/[,;|]/)
    .map((item) => item.replace(/^\s*[-*]\s+/, "").trim())
    .filter((item) => {
      const normalized = item.toLowerCase();
      return (
        normalized.length > 0 &&
        normalized !== "none" &&
        normalized !== "n/a" &&
        normalized !== "na" &&
        normalized !== "null" &&
        normalized !== "no entities" &&
        normalized !== "no topics"
      );
    });
}

function parseProviderResponse(response: string): SummaryFragment {
  const lines = response.split(/\r?\n/);
  const entityValues: string[] = [];
  const summaryLines: string[] = [];
  let collectingEntities = false;
  let sawEntitySection = false;

  for (const line of lines) {
    const entityMatch = line.match(/^\s*(?:entities|topics)\s*:\s*(.*)$/i);

    if (entityMatch) {
      sawEntitySection = true;
      collectingEntities = true;
      entityValues.push(...splitEntityLine(entityMatch[1]));
      continue;
    }

    const bulletMatch = line.match(/^\s*[-*]\s+(.+)$/);

    if (collectingEntities && bulletMatch) {
      entityValues.push(...splitEntityLine(bulletMatch[1]));
      continue;
    }

    if (collectingEntities && line.trim().length === 0) {
      continue;
    }

    collectingEntities = false;
    summaryLines.push(line);
  }

  const summary = normalizeWhitespace(summaryLines.join(" ")) || response.trim();
  const fallbackEntities = extractEntities(summary);

  return {
    summary: summary || "No summary available.",
    entities: uniqueStable(
      entityValues.length > 0 || sawEntitySection ? entityValues : fallbackEntities,
    ),
  };
}

function chunksForFile(file: IndexedFile, chunksById: Map<string, TextChunk>) {
  return file.chunkIds
    .map((chunkId) => chunksById.get(chunkId))
    .filter(
      (chunk): chunk is TextChunk =>
        chunk !== undefined && chunk.fileId === file.id,
    )
    .sort((left, right) => left.index - right.index || left.id.localeCompare(right.id));
}

function promptBody(value: string, maxLength: number): string {
  return value.length <= maxLength ? value : value.slice(0, maxLength);
}

export function createDeterministicSummarizer(): FolderSummarizer {
  return {
    summarizeChunk(chunk) {
      const entities = extractEntities(chunk.text);

      return {
        summary: summarizeText(chunk.text, entities),
        entities,
      };
    },
    summarizeFile(file, chunks, chunkSummaries) {
      const entities = sortedUnique(
        chunkSummaries.flatMap((chunkSummary) => chunkSummary.entities),
      );
      const fallbackEntities = extractEntities(
        [file.path, ...chunks.map((chunk) => chunk.text)].join(" "),
      );
      const finalEntities = entities.length > 0 ? entities : fallbackEntities;
      const chunkText = singularize(chunks.length, "chunk");
      const entityText = finalEntities.slice(0, 6).join(", ") || "no prominent terms";

      return {
        summary: `${file.path}: ${chunkText}; key terms ${entityText}.`,
        entities: finalEntities,
      };
    },
    summarizeFolder(manifest, fileSummaries) {
      const topics = sortedUnique(
        fileSummaries.flatMap((fileSummary) => fileSummary.entities),
      );
      const topicText = topics.length > 0 ? ` Topics: ${topics.slice(0, 8).join(", ")}.` : "";

      return {
        text: `${manifest.rootName}: ${singularize(
          manifest.stats.indexedFiles,
          "indexed file",
        )}, ${singularize(
          manifest.stats.ignoredFiles,
          "ignored file",
        )}, ${singularize(manifest.stats.errorFiles, "error")}.${topicText}`,
        topics,
      };
    },
  };
}

export function createProviderSummarizer(
  provider: TextGenerationProvider,
  options: ProviderSummarizerOptions = {},
): FolderSummarizer {
  const maxPromptChars = Math.max(256, options.maxPromptChars ?? 6_000);

  return {
    async summarizeChunk(chunk) {
      const response = await provider.generateText(
        [
          "Summarize this folder chunk for a local knowledge graph.",
          `Path: ${chunk.path}`,
          `Chunk: ${chunk.index + 1}`,
          "Return a concise summary and an 'Entities:' line.",
          promptBody(chunk.text, maxPromptChars),
        ].join("\n"),
        options.signal,
      );

      return parseProviderResponse(response);
    },
    async summarizeFile(file, chunks, chunkSummaries) {
      const response = await provider.generateText(
        [
          "Summarize this file from its chunk summaries for a local knowledge graph.",
          `Path: ${file.path}`,
          `Chunks: ${chunks.length}`,
          "Return a concise summary and an 'Entities:' line.",
          promptBody(
            chunkSummaries
              .map((chunkSummary) => `- ${chunkSummary.summary}`)
              .join("\n"),
            maxPromptChars,
          ),
        ].join("\n"),
        options.signal,
      );

      return parseProviderResponse(response);
    },
    async summarizeFolder(manifest, fileSummaries) {
      const response = await provider.generateText(
        [
          "Summarize this folder from file summaries for a local knowledge graph.",
          `Root: ${manifest.rootName}`,
          `Indexed files: ${manifest.stats.indexedFiles}`,
          "Return a concise summary and a 'Topics:' line.",
          promptBody(
            fileSummaries
              .map((fileSummary) => `- ${fileSummary.path}: ${fileSummary.summary}`)
              .join("\n"),
            maxPromptChars,
          ),
        ].join("\n"),
        options.signal,
      );
      const parsed = parseProviderResponse(response);

      return {
        text: parsed.summary,
        topics: parsed.entities,
      };
    },
  };
}

export async function summarizeFolderManifest(
  manifest: FolderManifest,
  summarizer: FolderSummarizer,
): Promise<SummaryResult> {
  const indexedFiles = manifest.files
    .filter((file) => file.status === "indexed")
    .sort((left, right) => left.path.localeCompare(right.path));
  const validChunkRefs = new Set(
    indexedFiles.flatMap((file) =>
      file.chunkIds.map((chunkId) => chunkRef(file.id, chunkId)),
    ),
  );
  const sortedChunks = manifest.chunks
    .filter((chunk) => validChunkRefs.has(chunkRef(chunk.fileId, chunk.id)))
    .sort(
      (left, right) =>
        left.path.localeCompare(right.path) ||
        left.index - right.index ||
        left.id.localeCompare(right.id),
  );
  const chunksById = new Map(sortedChunks.map((chunk) => [chunk.id, chunk]));
  const chunkSummaries: ChunkSummary[] = [];
  const chunkSummariesById = new Map<string, ChunkSummary>();

  for (const chunk of sortedChunks) {
    const fragment = await summarizer.summarizeChunk(chunk);
    const chunkSummary: ChunkSummary = {
      chunkId: chunk.id,
      fileId: chunk.fileId,
      path: chunk.path,
      summary: fragment.summary,
      entities: uniqueStable(fragment.entities),
    };

    chunkSummaries.push(chunkSummary);
    chunkSummariesById.set(chunk.id, chunkSummary);
  }

  const fileSummaries: FileSummary[] = [];

  for (const file of indexedFiles) {
    const fileChunks = chunksForFile(file, chunksById);
    const matchingChunkSummaries = fileChunks
      .map((chunk) => chunkSummariesById.get(chunk.id))
      .filter((summary): summary is ChunkSummary => Boolean(summary));
    const fragment = await summarizer.summarizeFile(
      file,
      fileChunks,
      matchingChunkSummaries,
    );

    fileSummaries.push({
      fileId: file.id,
      path: file.path,
      summary: fragment.summary,
      entities: uniqueStable(fragment.entities),
    });
  }

  return {
    chunkSummaries,
    fileSummaries,
    folderSummary: await summarizer.summarizeFolder(manifest, fileSummaries),
  };
}
