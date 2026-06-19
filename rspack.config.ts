import { rspack, type Configuration } from "@rspack/core";
import { ReactRefreshRspackPlugin } from "@rspack/plugin-react-refresh";
import { dirname, resolve } from "path";
import { fileURLToPath } from "url";
import { crossOriginIsolationHeaders } from "./src/lib/securityHeaders";
import { createModelFileMiddleware } from "./modelServer";

const rootDir = dirname(fileURLToPath(import.meta.url));
const isDevelopment = Bun.env.NODE_ENV !== "production";
const modelDir = resolve(rootDir, "models");

const config: Configuration = {
  context: rootDir,
  entry: {
    main: "./src/main.tsx",
  },
  output: {
    path: resolve(rootDir, "dist"),
    filename: isDevelopment ? "assets/[name].js" : "assets/[name].[contenthash:8].js",
    chunkFilename: isDevelopment
      ? "assets/[name].chunk.js"
      : "assets/[name].[contenthash:8].chunk.js",
    publicPath: "/",
    clean: true,
  },
  resolve: {
    extensions: ["...", ".ts", ".tsx", ".js", ".jsx"],
  },
  module: {
    rules: [
      {
        test: /\.[jt]sx?$/,
        exclude: /node_modules/,
        loader: "builtin:swc-loader",
        options: {
          jsc: {
            parser: {
              syntax: "typescript",
              tsx: true,
            },
            transform: {
              react: {
                runtime: "automatic",
                development: isDevelopment,
                refresh: isDevelopment,
              },
            },
          },
        },
        type: "javascript/auto",
      },
      {
        test: /\.css$/,
        type: "css/auto",
      },
    ],
  },
  plugins: [
    new rspack.HtmlRspackPlugin({
      template: "./index.html",
    }),
    new rspack.CopyRspackPlugin({
      patterns: [{ from: "public", to: "." }],
    }),
    isDevelopment ? new ReactRefreshRspackPlugin() : null,
  ].filter(Boolean),
  devServer: {
    host: "127.0.0.1",
    port: 5173,
    hot: true,
    historyApiFallback: true,
    headers: crossOriginIsolationHeaders,
    static: [
      {
        directory: resolve(rootDir, "public"),
        publicPath: "/",
        watch: false,
      },
    ],
    setupMiddlewares: (middlewares, server) => {
      server?.app?.use(createModelFileMiddleware(modelDir));
      return middlewares;
    },
  },
  optimization: {
    splitChunks: {
      chunks: "all",
    },
  },
  performance: {
    hints: false,
  },
  devtool: isDevelopment ? "eval-cheap-module-source-map" : "source-map",
};

export default config;
