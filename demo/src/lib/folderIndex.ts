import type { SourceFile } from "./folderAccess";
import { normalizeRelativePath } from "./folderAccess";

export type PathIgnoreReason =
  | "ignored-directory"
  | "binary-or-media"
  | "model-file";

export type IndexedFileStatus = "indexed" | "ignored" | "error";

export type IndexedFileIgnoreReason = PathIgnoreReason | "too-large";

export interface IndexedFile {
  id: string;
  path: string;
  sourceKind: SourceFile["sourceKind"];
  name: string;
  extension: string;
  mimeType: string;
  sizeBytes: number;
  lastModified: number;
  status: IndexedFileStatus;
  chunkIds: string[];
  hash?: string;
  ignoreReason?: IndexedFileIgnoreReason;
  error?: string;
}

export interface TextChunk {
  id: string;
  fileId: string;
  path: string;
  index: number;
  text: string;
  startOffset: number;
  endOffset: number;
  hash: string;
  bytes: number;
}

export interface FolderManifest {
  id: string;
  rootName: string;
  sourceKind: SourceFile["sourceKind"];
  files: IndexedFile[];
  chunks: TextChunk[];
  stats: {
    totalFiles: number;
    indexedFiles: number;
    ignoredFiles: number;
    errorFiles: number;
    totalBytes: number;
    indexedBytes: number;
    chunkCount: number;
  };
}

export interface FolderIndexConfig {
  chunkSize?: number;
  maxFileBytes?: number;
  rootName?: string;
}

interface ChunkSlice {
  text: string;
  start: number;
  end: number;
}

const ignoredDirectoryNames = new Set([
  ".git",
  ".hg",
  ".next",
  ".nuxt",
  ".svelte-kit",
  ".venv",
  "__pycache__",
  "build",
  "coverage",
  "dist",
  "node_modules",
  "target",
]);

const modelExtensions = new Set([
  ".bin",
  ".gguf",
  ".litertlm",
  ".onnx",
  ".safetensors",
  ".tflite",
]);

const binaryOrMediaExtensions = new Set([
  ".7z",
  ".a",
  ".avi",
  ".bmp",
  ".bz2",
  ".class",
  ".dmg",
  ".dll",
  ".dylib",
  ".eot",
  ".exe",
  ".gif",
  ".gz",
  ".ico",
  ".jar",
  ".jpeg",
  ".jpg",
  ".mov",
  ".mp3",
  ".mp4",
  ".ogg",
  ".otf",
  ".pdf",
  ".png",
  ".rar",
  ".so",
  ".svg",
  ".tar",
  ".tgz",
  ".ttf",
  ".wav",
  ".wasm",
  ".webm",
  ".webp",
  ".woff",
  ".woff2",
  ".zip",
]);

const defaultChunkSize = 2_000;
const defaultMaxFileBytes = 500_000;

const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

function extensionFor(path: string): string {
  const fileName = path.split("/").at(-1) ?? path;
  const dotIndex = fileName.lastIndexOf(".");

  if (dotIndex <= 0) {
    return "";
  }

  return fileName.slice(dotIndex).toLowerCase();
}

function fallbackHash(bytes: Uint8Array): string {
  let hash = 0x811c9dc5;

  for (const byte of bytes) {
    hash ^= byte;
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }

  return hash.toString(16).padStart(8, "0");
}

async function hashText(text: string): Promise<string> {
  const bytes = textEncoder.encode(text);

  try {
    if (globalThis.crypto?.subtle) {
      const digest = await globalThis.crypto.subtle.digest("SHA-256", bytes);
      return Array.from(new Uint8Array(digest), (byte) =>
        byte.toString(16).padStart(2, "0"),
      ).join("");
    }
  } catch {
    return fallbackHash(bytes);
  }

  return fallbackHash(bytes);
}

async function readFileText(file: File): Promise<string> {
  const fileWithReaders = file as File & {
    text?: () => Promise<string>;
    arrayBuffer?: () => Promise<ArrayBuffer>;
  };

  if (typeof fileWithReaders.text === "function") {
    return fileWithReaders.text();
  }

  if (typeof fileWithReaders.arrayBuffer === "function") {
    return textDecoder.decode(await fileWithReaders.arrayBuffer());
  }

  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();

    reader.addEventListener("load", () => {
      resolve(typeof reader.result === "string" ? reader.result : "");
    });
    reader.addEventListener("error", () => {
      reject(reader.error ?? new Error("Unable to read file text."));
    });
    reader.readAsText(file);
  });
}

function baseFileRecord(source: SourceFile, id: string): IndexedFile {
  return {
    id,
    path: source.path,
    sourceKind: source.sourceKind,
    name: source.file.name,
    extension: extensionFor(source.path || source.file.name),
    mimeType: source.file.type || "text/plain",
    sizeBytes: source.file.size,
    lastModified: source.file.lastModified,
    status: "ignored",
    chunkIds: [],
  };
}

function createWordAwareChunks(text: string, chunkSize: number): ChunkSlice[] {
  const chunks: ChunkSlice[] = [];
  const words = Array.from(text.matchAll(/\S+/g));
  let currentText = "";
  let currentStart = 0;
  let currentEnd = 0;

  const flush = () => {
    if (currentText.length === 0) {
      return;
    }

    chunks.push({
      text: currentText,
      start: currentStart,
      end: currentEnd,
    });
    currentText = "";
    currentStart = 0;
    currentEnd = 0;
  };

  for (const word of words) {
    const wordText = word[0];
    const wordStart = word.index;
    const wordEnd = wordStart + wordText.length;

    if (wordText.length > chunkSize) {
      flush();

      for (let start = 0; start < wordText.length; start += chunkSize) {
        const segment = wordText.slice(start, start + chunkSize);
        chunks.push({
          text: segment,
          start: wordStart + start,
          end: wordStart + start + segment.length,
        });
      }

      continue;
    }

    const nextText =
      currentText.length === 0 ? wordText : `${currentText} ${wordText}`;

    if (currentText.length > 0 && nextText.length > chunkSize) {
      flush();
      currentText = wordText;
      currentStart = wordStart;
      currentEnd = wordEnd;
      continue;
    }

    if (currentText.length === 0) {
      currentStart = wordStart;
    }

    currentText = nextText;
    currentEnd = wordEnd;
  }

  flush();
  return chunks;
}

function normalizeSources(files: SourceFile[]): SourceFile[] {
  return files
    .map((source) => ({
      ...source,
      path: normalizeRelativePath(source.path || source.file.name),
    }))
    .sort((left, right) => left.path.localeCompare(right.path));
}

function deriveRootName(files: SourceFile[], configuredRootName?: string): string {
  if (configuredRootName?.trim()) {
    return configuredRootName.trim();
  }

  const firstPath = files[0]?.path;
  if (!firstPath) {
    return "Selected folder";
  }

  return firstPath.split("/")[0] || "Selected folder";
}

function deriveSourceKind(files: SourceFile[]): SourceFile["sourceKind"] {
  const firstKind = files[0]?.sourceKind ?? "file-list";

  return files.every((file) => file.sourceKind === firstKind)
    ? firstKind
    : "file-list";
}

export function shouldIgnorePath(path: string): PathIgnoreReason | null {
  const normalizedPath = normalizeRelativePath(path);
  const segments = normalizedPath.toLowerCase().split("/");

  if (segments.some((segment) => ignoredDirectoryNames.has(segment))) {
    return "ignored-directory";
  }

  const extension = extensionFor(normalizedPath);

  if (modelExtensions.has(extension)) {
    return "model-file";
  }

  if (binaryOrMediaExtensions.has(extension)) {
    return "binary-or-media";
  }

  return null;
}

export async function indexFolder(
  files: SourceFile[],
  config: FolderIndexConfig = {},
): Promise<FolderManifest> {
  const chunkSize = Math.max(1, Math.floor(config.chunkSize ?? defaultChunkSize));
  const maxFileBytes = Math.max(
    0,
    Math.floor(config.maxFileBytes ?? defaultMaxFileBytes),
  );
  const sources = normalizeSources(files);
  const manifest: FolderManifest = {
    id: "manifest:pending",
    rootName: deriveRootName(sources, config.rootName),
    sourceKind: deriveSourceKind(sources),
    files: [],
    chunks: [],
    stats: {
      totalFiles: 0,
      indexedFiles: 0,
      ignoredFiles: 0,
      errorFiles: 0,
      totalBytes: 0,
      indexedBytes: 0,
      chunkCount: 0,
    },
  };

  for (const source of sources) {
    manifest.stats.totalFiles += 1;
    manifest.stats.totalBytes += source.file.size;

    const baseId = `file:${await hashText(source.path)}`;
    const baseRecord = baseFileRecord(source, baseId);
    const ignoreReason = shouldIgnorePath(source.path);

    if (ignoreReason) {
      manifest.files.push({ ...baseRecord, ignoreReason });
      manifest.stats.ignoredFiles += 1;
      continue;
    }

    if (source.file.size > maxFileBytes) {
      manifest.files.push({ ...baseRecord, ignoreReason: "too-large" });
      manifest.stats.ignoredFiles += 1;
      continue;
    }

    try {
      const text = await readFileText(source.file);
      const fileHash = await hashText(text);
      const fileId = `file:${await hashText(`${source.path}\0${fileHash}`)}`;
      const slices = createWordAwareChunks(text, chunkSize);
      const chunkIds: string[] = [];

      for (const [index, slice] of slices.entries()) {
        const chunkHash = await hashText(slice.text);
        const chunkId = `chunk:${await hashText(
          `${fileId}\0${index}\0${chunkHash}`,
        )}`;

        chunkIds.push(chunkId);
        manifest.chunks.push({
          id: chunkId,
          fileId,
          path: source.path,
          index,
          text: slice.text,
          startOffset: slice.start,
          endOffset: slice.end,
          hash: chunkHash,
          bytes: textEncoder.encode(slice.text).byteLength,
        });
      }

      manifest.files.push({
        ...baseRecord,
        id: fileId,
        status: "indexed",
        hash: fileHash,
        chunkIds,
      });
      manifest.stats.indexedFiles += 1;
      manifest.stats.indexedBytes += source.file.size;
    } catch (error) {
      manifest.files.push({
        ...baseRecord,
        status: "error",
        error: error instanceof Error ? error.message : String(error),
      });
      manifest.stats.errorFiles += 1;
    }
  }

  manifest.stats.chunkCount = manifest.chunks.length;
  const manifestInput = manifest.files
    .map((file) => `${file.path}:${file.hash ?? file.ignoreReason ?? file.status}`)
    .join("\0");
  manifest.id = `manifest:${await hashText(
    `${manifest.rootName}\0${manifestInput}`,
  )}`;
  return manifest;
}
