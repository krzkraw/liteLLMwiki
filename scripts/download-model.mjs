import { createWriteStream } from "node:fs";
import { mkdir, rename, rm } from "node:fs/promises";
import { dirname } from "node:path";
import { Readable } from "node:stream";
import { finished } from "node:stream/promises";
import { gemma4E2bWebFilename, resolveWebModelPath } from "./modelFiles.mjs";

const repo = "litert-community/gemma-4-E2B-it-litert-lm";
const filename = gemma4E2bWebFilename;
const target = resolveWebModelPath();
const partial = `${target}.partial`;
const token = process.env.HF_TOKEN || process.env.HUGGING_FACE_HUB_TOKEN;
const url = `https://huggingface.co/${repo}/resolve/main/${filename}`;
const requestInit = token
  ? {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    }
  : undefined;

await mkdir(dirname(target), { recursive: true });
await rm(partial, { force: true });

const response = await fetch(url, requestInit);

if (!response.ok || !response.body) {
  console.error(`Download failed: HTTP ${response.status} ${response.statusText}`);
  console.error(`URL: ${url}`);
  console.error(
    "If Hugging Face requires auth for this request, set HF_TOKEN or HUGGING_FACE_HUB_TOKEN.",
  );
  process.exit(1);
}

const total = Number(response.headers.get("content-length") ?? 0);
let downloaded = 0;
let nextLog = 0;

const progress = new TransformStream({
  transform(chunk, controller) {
    downloaded += chunk.byteLength;
    if (total > 0 && downloaded >= nextLog) {
      const percent = ((downloaded / total) * 100).toFixed(1);
      process.stderr.write(`\rDownloading ${percent}%`);
      nextLog = downloaded + total / 100;
    }
    controller.enqueue(chunk);
  },
});

await finished(
  Readable.fromWeb(response.body.pipeThrough(progress)).pipe(createWriteStream(partial)),
);
process.stderr.write("\n");
await rename(partial, target);

console.log(
  JSON.stringify(
    {
      path: target,
      bytes: downloaded,
      source: url,
    },
    null,
    2,
  ),
);
