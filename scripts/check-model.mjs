import { stat } from "fs/promises";
import { resolveWebModelPath } from "./modelFiles.mjs";

const modelPath = resolveWebModelPath(process.cwd(), process.argv[2]);

try {
  const info = await stat(modelPath);
  if (!info.isFile()) {
    console.error(`Not a file: ${modelPath}`);
    process.exit(1);
  }

  const gib = info.size / 1024 / 1024 / 1024;
  console.log(
    JSON.stringify(
      {
        path: modelPath,
        bytes: info.size,
        gib: Number(gib.toFixed(2)),
        looksLikeGemma4E2bWebModel:
          modelPath.endsWith("gemma-4-E2B-it-web.litertlm") && gib > 1,
      },
      null,
      2,
    ),
  );

  if (gib <= 1) {
    process.exitCode = 1;
  }
} catch (error) {
  console.error(
    error instanceof Error ? error.message : `Unable to inspect ${modelPath}`,
  );
  process.exit(1);
}
