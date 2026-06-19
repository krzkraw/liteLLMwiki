import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";
import { crossOriginIsolationHeaders } from "./src/lib/securityHeaders";
import { createModelFileMiddleware } from "./modelServer";

const modelDir = fileURLToPath(new URL("./models", import.meta.url));

export default defineConfig({
  plugins: [
    react(),
    {
      name: "local-litert-models",
      configureServer(server) {
        server.middlewares.use(createModelFileMiddleware(modelDir));
      },
      configurePreviewServer(server) {
        server.middlewares.use(createModelFileMiddleware(modelDir));
      },
    },
  ],
  server: {
    headers: crossOriginIsolationHeaders,
  },
  preview: {
    headers: crossOriginIsolationHeaders,
  },
  test: {
    environment: "jsdom",
    globals: true,
  },
});
