import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, mock } from "bun:test";
import { ProviderSetup, type ProviderSetupProps } from "./ProviderSetup";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT =
  true;

let root: Root | null = null;
let container: HTMLDivElement | null = null;

function getByTestId<T extends Element = Element>(testId: string): T {
  const element = container?.querySelector(`[data-testid="${testId}"]`);

  if (!element) {
    throw new Error(`Unable to find element with data-testid="${testId}".`);
  }

  return element as T;
}

const defaultProps: ProviderSetupProps = {
  providerKind: "executable",
  modelPath: "/models/litert/gemma-4-E2B-it-web.litertlm",
  localModelFileName: null,
  webGpu: {
    state: "blocked",
    label: "WebGPU blocked",
    detail: "No adapter.",
  },
  runtimeAudit: {
    state: "blocked",
    label: "Runtime blocked",
    detail: "No adapter.",
  },
  modelProbe: {
    state: "idle",
    message: "Idle.",
  },
  loadError: null,
  isLoadingModel: false,
  modelLoaded: false,
  executableEndpoint: "http://127.0.0.1:9379/v1",
  backend: "auto",
  backendOptions: [{ value: "auto", label: "Auto" }],
  executableStatus: {
    state: "idle",
    label: "Sidecar not connected",
    detail: "Endpoint configured.",
  },
  sidecarModelCatalog: {
    state: "idle",
    models: [],
    detail: "Connect sidecar to inspect model catalog.",
  },
  runtimeStatus: null,
  suggestedModelUrl: "https://example.com",
  onProviderKindChange: () => undefined,
  onModelPathChange: () => undefined,
  onLocalModelFileChange: () => undefined,
  onCheckModel: () => undefined,
  onLoadModel: () => undefined,
  onExecutableEndpointChange: () => undefined,
  onBackendChange: () => undefined,
  onConnectExecutable: () => undefined,
  onConnectRuntimeControl: () => undefined,
  onStartRuntime: () => undefined,
  onRestartRuntime: () => undefined,
  onStopRuntime: () => undefined,
  webProviderOptions: { temperature: 0.7 },
  executableProviderOptions: {
    endpoint: "http://127.0.0.1:9379/v1",
    backend: "auto",
  },
  onWebProviderOptionChange: () => undefined,
  onExecutableProviderOptionChange: () => undefined,
  sidecarControlConnected: false,
  sidecarLogs: [],
};

async function renderProviderSetup(props: Partial<ProviderSetupProps> = {}) {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);

  await act(async () => {
    root?.render(<ProviderSetup {...defaultProps} {...props} />);
  });
}

describe("ProviderSetup", () => {
  afterEach(() => {
    act(() => {
      root?.unmount();
    });
    container?.remove();
    root = null;
    container = null;
  });

  it("enables executable sidecar connection", async () => {
    const onConnectExecutable = mock();

    await renderProviderSetup({ onConnectExecutable });

    const button = getByTestId<HTMLButtonElement>("connect-sidecar-button");

    expect(button.disabled).toBe(false);

    await act(async () => {
      button.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(onConnectExecutable).toHaveBeenCalledTimes(1);
  });

  it("shows connected executable provider state", async () => {
    await renderProviderSetup({
      modelLoaded: true,
      executableStatus: {
        state: "ready",
        label: "Sidecar connected",
        detail: "CPU available.",
      },
    });

    expect(container?.textContent).toContain("Connected");
    expect(container?.textContent).toContain("Sidecar connected");
    expect(getByTestId<HTMLButtonElement>("connect-sidecar-button").disabled).toBe(
      true,
    );
  });

  it("disables executable connection while checking", async () => {
    await renderProviderSetup({
      executableStatus: {
        state: "checking",
        label: "Connecting sidecar",
        detail: "Probing endpoint.",
      },
    });

    expect(getByTestId<HTMLButtonElement>("connect-sidecar-button").disabled).toBe(
      true,
    );
  });

  it("renders runtime websocket controls and debug output", async () => {
    const onConnectRuntimeControl = mock();
    const onStartRuntime = mock();
    const onRestartRuntime = mock();
    const onStopRuntime = mock();

    await renderProviderSetup({
      sidecarControlConnected: true,
      sidecarLogs: [
        {
          seq: 1,
          source: "runtime",
          stream: "stdout",
          line: "runtime ready",
        },
      ],
      onConnectRuntimeControl,
      onStartRuntime,
      onRestartRuntime,
      onStopRuntime,
    });

    await act(async () => {
      getByTestId<HTMLButtonElement>("connect-runtime-control-button").dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
      getByTestId<HTMLButtonElement>("start-runtime-debug-button").dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
      getByTestId<HTMLButtonElement>("restart-runtime-release-button").dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
      getByTestId<HTMLButtonElement>("restart-runtime-debug-button").dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
      getByTestId<HTMLButtonElement>("stop-runtime-button").dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(onConnectRuntimeControl).toHaveBeenCalledTimes(1);
    expect(onStartRuntime).toHaveBeenCalledWith("debug");
    expect(onRestartRuntime).toHaveBeenCalledWith("release");
    expect(onRestartRuntime).toHaveBeenCalledWith("debug");
    expect(onStopRuntime).toHaveBeenCalledTimes(1);
    const manualCommand = getByTestId("manual-sidecar-command").textContent ?? "";
    expect(manualCommand).toContain(
      "./native/sidecar-artifacts/litert-sidecar-darwin-arm64/litert-sidecar",
    );
    expect(manualCommand).toContain(
      ".\\native\\sidecar-artifacts\\litert-sidecar-windows-amd64\\litert-sidecar.exe",
    );
    expect(getByTestId("runtime-log-output").textContent).toContain(
      "runtime stdout runtime ready",
    );
  });

  it("renders executable models, endpoints, and compact config", async () => {
    await renderProviderSetup({
      sidecarControlConnected: true,
      modelLoaded: true,
      sidecarModelCatalog: {
        state: "ready",
        detail: "4 models detected.",
        models: [
          {
            id: "gemma4-gguf",
            repo: "unsloth/gemma-4-E2B-it-qat-GGUF",
            filename: "gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf",
            targetPath: "models/llamacpp/gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf",
            runtime: "llamacpp",
            role: "main",
            required: true,
            state: "present",
            bytesDownloaded: 1024,
            sizeBytes: 1024,
          },
          {
            id: "qwen3-embedding-gguf",
            repo: "Qwen/Qwen3-Embedding-0.6B-GGUF",
            filename: "Qwen3-Embedding-0.6B-Q8_0.gguf",
            targetPath: "models/llamacpp/Qwen3-Embedding-0.6B-Q8_0.gguf",
            runtime: "llamacpp",
            role: "embedding",
            required: true,
            state: "present",
            bytesDownloaded: 2048,
            sizeBytes: 2048,
          },
        ],
      },
      runtimeStatus: {
        state: "running",
        executable: "/opt/homebrew/bin/llama-server",
        version: "9700",
        modelId: "gemma4-e2b",
        modelFile: "models/llamacpp/gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf",
        upstream: "http://127.0.0.1:9381",
        mode: "release",
      },
      executableProviderOptions: {
        endpoint: "http://127.0.0.1:9379/v1",
        modelId: "gemma4-e2b",
        backend: "cpu",
        runtimeHost: "127.0.0.1",
        runtimePort: 9381,
        modelFile: "models/litert/gemma-4-E2B-it.litertlm",
        maxTokens: 512,
      },
    });

    const endpoints = getByTestId("sidecar-endpoints-panel");
    const models = getByTestId("sidecar-models-panel");
    const config = getByTestId("sidecar-config-panel");

    expect(endpoints.tagName).toBe("DETAILS");
    expect(endpoints.textContent).toContain("/v1/chat/completions");
    expect(endpoints.textContent).toContain("/v1/embeddings");
    expect(endpoints.textContent).toContain("/sidecar/v1/models");
    expect(endpoints.textContent).toContain("/sidecar/v1/ws");

    expect(models.tagName).toBe("DETAILS");
    expect(models.textContent).toContain("gemma4-gguf");
    expect(models.textContent).toContain("llamacpp");
    expect(models.textContent).toContain("main");
    expect(models.textContent).toContain("present");
    expect(models.textContent).toContain("Qwen3-Embedding-0.6B-Q8_0.gguf");

    expect(config.tagName).toBe("DETAILS");
    expect(config.textContent).toContain("Endpoint");
    expect(config.textContent).toContain("http://127.0.0.1:9379/v1");
    expect(config.textContent).toContain("Runtime host");
    expect(config.textContent).toContain("127.0.0.1");
    expect(config.textContent).toContain("Model file");
    expect(config.textContent).toContain("models/litert/gemma-4-E2B-it.litertlm");
  });

  it("renders collapsible advanced options for the selected provider", async () => {
    await renderProviderSetup({
      providerKind: "web",
    });

    const advanced = getByTestId<HTMLDetailsElement>("provider-advanced-options");

    expect(advanced.tagName).toBe("DETAILS");
    expect(advanced.textContent).toContain("Advanced options");
    expect(advanced.textContent).toContain("WASM path");
    expect(getByTestId("provider-option-pill-wasmPath").getAttribute("title")).toContain(
      "LiteRT-LM WASM",
    );
  });

  it("forwards provider option edits from advanced pills", async () => {
    const onWebProviderOptionChange = mock();

    await renderProviderSetup({
      providerKind: "web",
      onWebProviderOptionChange,
    });

    const temperature = getByTestId<HTMLInputElement>(
      "provider-option-input-temperature",
    );

    await act(async () => {
      temperature.value = "0.25";
      temperature.dispatchEvent(new Event("input", { bubbles: true }));
    });

    expect(onWebProviderOptionChange).toHaveBeenCalledWith(
      "temperature",
      0.25,
      expect.objectContaining({ temperature: 0.25 }),
    );
  });
});
