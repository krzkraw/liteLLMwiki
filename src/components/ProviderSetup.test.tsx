import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
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
  modelPath: "/models/gemma-4-E2B-it-web.litertlm",
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
    const onConnectExecutable = vi.fn();

    await renderProviderSetup({ onConnectExecutable });

    const button = getByTestId<HTMLButtonElement>("connect-sidecar-button");

    expect(button.disabled).toBe(false);

    await act(async () => {
      button.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(onConnectExecutable).toHaveBeenCalledOnce();
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
    const onConnectRuntimeControl = vi.fn();
    const onStartRuntime = vi.fn();
    const onRestartRuntime = vi.fn();
    const onStopRuntime = vi.fn();

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

    expect(onConnectRuntimeControl).toHaveBeenCalledOnce();
    expect(onStartRuntime).toHaveBeenCalledWith("debug");
    expect(onRestartRuntime).toHaveBeenCalledWith("release");
    expect(onRestartRuntime).toHaveBeenCalledWith("debug");
    expect(onStopRuntime).toHaveBeenCalledOnce();
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
    const onWebProviderOptionChange = vi.fn();

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
