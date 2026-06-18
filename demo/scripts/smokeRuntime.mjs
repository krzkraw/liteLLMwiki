import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

export async function createSmokeWorkspace(prefix) {
  const root = await mkdtemp(join(tmpdir(), prefix));

  return {
    root,
    path(name) {
      return join(root, name);
    },
    async cleanup() {
      await rm(root, { recursive: true, force: true });
    },
  };
}

export function createChromiumGpuArgs(platform = process.platform) {
  const args = ["--enable-unsafe-webgpu", "--enable-features=WebGPU"];

  if (platform === "darwin") {
    args.push("--use-angle=metal");
  } else if (platform === "win32") {
    args.push("--use-angle=d3d11");
  } else if (platform === "linux") {
    args.push("--use-angle=vulkan");
  }

  return args;
}
