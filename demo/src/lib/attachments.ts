import type { ChatAttachment } from "./chatProvider";

export const maxNativeAttachmentBytes = 32 << 20;

export interface MultimodalAttachmentRequest {
  name: string;
  mimeType: string;
  dataBase64: string;
}

function isSupportedMimeType(mimeType: string): boolean {
  return mimeType.startsWith("image/") || mimeType.startsWith("audio/");
}

function createAttachmentId(file: File, index: number): string {
  const safeName = file.name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");

  return `${safeName || "attachment"}-${file.size}-${file.lastModified}-${index}`;
}

function assertSupportedFile(file: File) {
  const mimeType = file.type || "application/octet-stream";

  if (!isSupportedMimeType(mimeType)) {
    throw new Error("Native multimodal attachments must be image or audio files.");
  }

  if (file.size > maxNativeAttachmentBytes) {
    throw new Error("Native multimodal attachments must be 32 MiB or smaller.");
  }
}

export function createChatAttachments(fileList: FileList | null): ChatAttachment[] {
  if (!fileList) {
    return [];
  }

  return Array.from(fileList).map((file, index) => {
    assertSupportedFile(file);

    return {
      id: createAttachmentId(file, index),
      name: file.name,
      mimeType: file.type,
      file,
    };
  });
}

async function encodeFileToBase64(file: File): Promise<string> {
  const bytes = new Uint8Array(await readFileBytes(file));
  let binary = "";
  const chunkSize = 0x8000;

  for (let offset = 0; offset < bytes.length; offset += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + chunkSize));
  }

  return btoa(binary);
}

function readFileBytes(file: File): Promise<ArrayBuffer> {
  if (typeof file.arrayBuffer === "function") {
    return file.arrayBuffer();
  }

  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.addEventListener("load", () => {
      if (reader.result instanceof ArrayBuffer) {
        resolve(reader.result);
        return;
      }

      reject(new Error("Attachment reader did not return bytes."));
    });
    reader.addEventListener("error", () => {
      reject(reader.error ?? new Error("Read attachment file failed."));
    });
    reader.readAsArrayBuffer(file);
  });
}

export async function toMultimodalAttachmentRequests(
  attachments: ChatAttachment[],
): Promise<MultimodalAttachmentRequest[]> {
  return Promise.all(
    attachments.map(async (attachment) => ({
      name: attachment.name,
      mimeType: attachment.mimeType,
      dataBase64: await encodeFileToBase64(attachment.file),
    })),
  );
}
