import { cp, mkdir, rm, stat } from "node:fs/promises";
import { resolve } from "node:path";

const wasmAssetCopies = [
  {
    label: "LiteRT-LM",
    source: resolve("node_modules/@litert-lm/core/wasm"),
    parent: resolve("public/vendor/litert-lm/core"),
    target: resolve("public/vendor/litert-lm/core/wasm"),
  },
];

for (const { label, source, parent, target } of wasmAssetCopies) {
  try {
    const sourceInfo = await stat(source);
    if (!sourceInfo.isDirectory()) {
      throw new Error(`${source} is not a directory`);
    }
  } catch (error) {
    console.error(
      `${label} WASM assets are missing. Run npm install before npm run prepare:wasm.`,
    );
    if (error instanceof Error) {
      console.error(error.message);
    }
    process.exit(1);
  }

  await mkdir(parent, { recursive: true });
  await rm(target, { recursive: true, force: true });
  await cp(source, target, { recursive: true });

  console.log(`Copied ${label} WASM assets to ${target}`);
}
