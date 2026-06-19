import { resolve } from "path";

export const gemma4E2bWebFilename = "gemma-4-E2B-it-web.litertlm";
export const defaultWebModelRelativePath = `models/litert/${gemma4E2bWebFilename}`;

export function resolveWebModelPath(cwd = process.cwd(), explicitPath) {
  return resolve(cwd, explicitPath ?? defaultWebModelRelativePath);
}
