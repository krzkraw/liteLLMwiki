export type FolderSourceKind = "directory-handle" | "file-list";

export interface SourceFile {
  path: string;
  file: File;
  sourceKind: FolderSourceKind;
}

export interface FolderCapabilities {
  hasShowDirectoryPicker: boolean;
  hasWebkitDirectory: boolean;
}

export interface FolderCapabilityScope {
  showDirectoryPicker?: () => Promise<FileSystemDirectoryHandle | undefined>;
  HTMLInputElement?: {
    prototype?: object;
  };
  document?: {
    createElement?: (tagName: string) => unknown;
  };
}

type FileWithRelativePath = File & {
  webkitRelativePath?: string;
};

type DirectoryEntry = FileSystemDirectoryHandle | FileSystemFileHandle;

type IterableDirectoryHandle = FileSystemDirectoryHandle & {
  entries?: () => AsyncIterable<[string, DirectoryEntry]>;
  values?: () => AsyncIterable<DirectoryEntry>;
};

function defaultCapabilityScope(): FolderCapabilityScope {
  return globalThis as unknown as FolderCapabilityScope;
}

export function normalizeRelativePath(path: string): string {
  const normalized = path
    .replace(/\\/g, "/")
    .replace(/^file:\/\/\/?/i, "")
    .replace(/^[A-Za-z]:\//, "");

  const parts: string[] = [];

  for (const part of normalized.split("/")) {
    if (part === "" || part === ".") {
      continue;
    }

    if (part === "..") {
      parts.pop();
      continue;
    }

    parts.push(part);
  }

  return parts.join("/");
}

export function detectFolderCapabilities(
  scope: FolderCapabilityScope = defaultCapabilityScope(),
): FolderCapabilities {
  const hasShowDirectoryPicker =
    typeof scope.showDirectoryPicker === "function";

  let hasWebkitDirectory = false;

  try {
    if (scope.HTMLInputElement?.prototype) {
      hasWebkitDirectory =
        "webkitdirectory" in scope.HTMLInputElement.prototype;
    }

    if (!hasWebkitDirectory) {
      const input = scope.document?.createElement?.("input");
      hasWebkitDirectory =
        typeof input === "object" &&
        input !== null &&
        "webkitdirectory" in input;
    }
  } catch {
    hasWebkitDirectory = false;
  }

  return { hasShowDirectoryPicker, hasWebkitDirectory };
}

export function collectFilesFromFileList(files: FileList | File[]): SourceFile[] {
  return Array.from(files).map((file) => {
    const relativePath = (file as FileWithRelativePath).webkitRelativePath;
    const path = normalizeRelativePath(
      relativePath && relativePath.length > 0 ? relativePath : file.name,
    );

    return { path, file, sourceKind: "file-list" };
  });
}

export async function selectDirectoryHandle(): Promise<FileSystemDirectoryHandle | null> {
  const picker = defaultCapabilityScope().showDirectoryPicker;

  if (typeof picker !== "function") {
    return null;
  }

  try {
    return (await picker()) ?? null;
  } catch {
    return null;
  }
}

async function* entriesFromDirectory(
  handle: FileSystemDirectoryHandle,
): AsyncIterable<[string, DirectoryEntry]> {
  const iterableHandle = handle as IterableDirectoryHandle;

  if (typeof iterableHandle.entries === "function") {
    yield* iterableHandle.entries();
    return;
  }

  if (typeof iterableHandle.values === "function") {
    for await (const entry of iterableHandle.values()) {
      yield [entry.name, entry];
    }
  }
}

async function collectDirectoryEntries(
  handle: FileSystemDirectoryHandle,
  prefix: string,
  output: SourceFile[],
): Promise<void> {
  try {
    for await (const [name, entry] of entriesFromDirectory(handle)) {
      const path = normalizeRelativePath(`${prefix}/${name}`);

      if (entry.kind === "file") {
        try {
          const file = await entry.getFile();
          output.push({ path, file, sourceKind: "directory-handle" });
        } catch {
          continue;
        }
      }

      if (entry.kind === "directory") {
        await collectDirectoryEntries(entry, path, output);
      }
    }
  } catch {
    return;
  }
}

export async function collectFilesFromDirectory(
  handle: FileSystemDirectoryHandle,
): Promise<SourceFile[]> {
  const files: SourceFile[] = [];
  await collectDirectoryEntries(handle, normalizeRelativePath(handle.name), files);
  return files;
}
