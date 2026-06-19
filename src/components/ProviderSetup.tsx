import {
  CheckCircle2,
  Cpu,
  Loader2,
  MonitorCog,
  PlugZap,
  Search,
  Server,
} from "lucide-react";
import type { ChangeEvent } from "react";
import type { ModelProbeState } from "../lib/modelConfig";
import type { ProviderOptionValues } from "../lib/providers/providerOptionState";
import type { ProviderOptionValue } from "../lib/providers/providerOptionMetadata";
import type { SidecarRuntimeStatus } from "../lib/providers/sidecarClient";
import type {
  SidecarLogEntry,
  SidecarRuntimeMode,
} from "../lib/providers/sidecarControlClient";
import type { RuntimeAuditSummary } from "../lib/runtimeAudit";
import type { WebGpuStatus } from "../lib/webgpu";
import { ProviderOptionBoxes } from "./ProviderOptionBoxes";

export type AppProviderKind = "web" | "executable";

export interface StatusPanel {
  state: "ready" | "checking" | "idle" | "blocked" | "missing" | "needs-model";
  label: string;
  detail: string;
}

export interface BackendOption {
  value: string;
  label: string;
  disabled?: boolean;
}

export interface ProviderSetupProps {
  providerKind: AppProviderKind;
  modelPath: string;
  localModelFileName: string | null;
  webGpu: WebGpuStatus;
  runtimeAudit: RuntimeAuditSummary;
  modelProbe: {
    state: ModelProbeState;
    message: string;
  };
  loadError: string | null;
  isLoadingModel: boolean;
  modelLoaded: boolean;
  executableEndpoint: string;
  backend: string;
  backendOptions: BackendOption[];
  executableStatus: StatusPanel;
  runtimeStatus: SidecarRuntimeStatus | null;
  sidecarControlConnected: boolean;
  sidecarLogs: SidecarLogEntry[];
  webProviderOptions: Partial<ProviderOptionValues>;
  executableProviderOptions: Partial<ProviderOptionValues>;
  suggestedModelUrl: string;
  onProviderKindChange: (providerKind: AppProviderKind) => void;
  onModelPathChange: (modelPath: string) => void;
  onLocalModelFileChange: (files: FileList | null) => void;
  onCheckModel: () => void;
  onLoadModel: () => void;
  onExecutableEndpointChange: (endpoint: string) => void;
  onBackendChange: (backend: string) => void;
  onConnectExecutable: () => void;
  onConnectRuntimeControl: () => void;
  onStartRuntime: (mode: SidecarRuntimeMode) => void;
  onRestartRuntime: (mode: SidecarRuntimeMode) => void;
  onStopRuntime: () => void;
  onWebProviderOptionChange: (
    id: string,
    value: ProviderOptionValue,
    values: ProviderOptionValues,
  ) => void;
  onExecutableProviderOptionChange: (
    id: string,
    value: ProviderOptionValue,
    values: ProviderOptionValues,
  ) => void;
}

const manualSidecarCommand = [
  "macOS:",
  "./native/sidecar-artifacts/litert-sidecar-darwin-arm64/litert-sidecar \\",
  "  -runtime-exe /path/to/litert-lm",
  "",
  "Windows PowerShell:",
  ".\\native\\sidecar-artifacts\\litert-sidecar-windows-amd64\\litert-sidecar.exe `",
  "  -runtime-exe C:\\path\\to\\litert-lm.exe",
].join("\n");

function getStatusIcon(state: StatusPanel["state"] | WebGpuStatus["state"]) {
  if (state === "ready") {
    return <CheckCircle2 size={18} aria-hidden="true" />;
  }

  if (state === "checking") {
    return <Loader2 className="spin" size={18} aria-hidden="true" />;
  }

  return <Cpu size={18} aria-hidden="true" />;
}

export function ProviderSetup({
  providerKind,
  modelPath,
  localModelFileName,
  webGpu,
  runtimeAudit,
  modelProbe,
  loadError,
  isLoadingModel,
  modelLoaded,
  executableEndpoint,
  backend,
  backendOptions,
  executableStatus,
  runtimeStatus,
  sidecarControlConnected,
  sidecarLogs,
  webProviderOptions,
  executableProviderOptions,
  suggestedModelUrl,
  onProviderKindChange,
  onModelPathChange,
  onLocalModelFileChange,
  onCheckModel,
  onLoadModel,
  onExecutableEndpointChange,
  onBackendChange,
  onConnectExecutable,
  onConnectRuntimeControl,
  onStartRuntime,
  onRestartRuntime,
  onStopRuntime,
  onWebProviderOptionChange,
  onExecutableProviderOptionChange,
}: ProviderSetupProps) {
  const canLoadWebModel =
    providerKind === "web" && runtimeAudit.state === "ready" && !isLoadingModel;
  const executableConnected = providerKind === "executable" && modelLoaded;
  const runtimeStatusPanel = createRuntimeStatusPanel(runtimeStatus);

  return (
    <aside className="setup-panel" aria-label="Provider setup">
      <header className="setup-header">
        <div className="brand-mark" aria-hidden="true">
          <PlugZap size={22} />
        </div>
        <div>
          <h1>Gemma Local Chat</h1>
          <p>Text-first LiteRT workbench for local Gemma 4 sessions.</p>
        </div>
      </header>

      <div className="segmented-control" aria-label="Provider">
        <button
          type="button"
          className={providerKind === "web" ? "is-selected" : ""}
          data-testid="provider-web-button"
          aria-pressed={providerKind === "web"}
          onClick={() => onProviderKindChange("web")}
        >
          <MonitorCog size={16} aria-hidden="true" />
          <span>Web</span>
        </button>
        <button
          type="button"
          className={providerKind === "executable" ? "is-selected" : ""}
          data-testid="provider-executable-button"
          aria-pressed={providerKind === "executable"}
          onClick={() => onProviderKindChange("executable")}
        >
          <Server size={16} aria-hidden="true" />
          <span>Executable</span>
        </button>
      </div>

      {providerKind === "web" ? (
        <section className="setup-section" aria-label="Web provider">
          <label className="field">
            <span>Gemma 4 E2B web model</span>
            <input
              value={modelPath}
              onChange={(event) => onModelPathChange(event.target.value)}
              spellCheck={false}
              disabled={isLoadingModel}
            />
          </label>

          <label className="file-picker">
            <span>{localModelFileName ?? "Choose local .litertlm"}</span>
            <input
              type="file"
              accept=".litertlm"
              data-testid="local-model-input"
              disabled={isLoadingModel}
              onChange={(event: ChangeEvent<HTMLInputElement>) =>
                onLocalModelFileChange(event.target.files)
              }
            />
          </label>

          <div className="button-grid">
            <button
              type="button"
              className="secondary-button"
              data-testid="check-model-button"
              onClick={onCheckModel}
              disabled={modelProbe.state === "checking"}
            >
              {modelProbe.state === "checking" ? (
                <Loader2 className="spin" size={16} aria-hidden="true" />
              ) : (
                <Search size={16} aria-hidden="true" />
              )}
              <span>Check model</span>
            </button>
            <button
              type="button"
              className="primary-button"
              data-testid="load-model-button"
              onClick={onLoadModel}
              disabled={!canLoadWebModel}
            >
              {isLoadingModel ? (
                <Loader2 className="spin" size={16} aria-hidden="true" />
              ) : (
                <PlugZap size={16} aria-hidden="true" />
              )}
              <span>{modelLoaded ? "Reload model" : "Load model"}</span>
            </button>
          </div>

          {loadError ? <p className="error-text">{loadError}</p> : null}

          <a className="text-link" href={suggestedModelUrl} target="_blank" rel="noreferrer">
            Model source
          </a>

          <StatusCard
            state={webGpu.state}
            label={webGpu.label}
            detail={webGpu.detail}
          />
          <StatusCard
            state={runtimeAudit.state}
            label={runtimeAudit.label}
            detail={runtimeAudit.detail}
          />
          <StatusCard
            state={modelProbe.state}
            label="Model preflight"
            detail={modelProbe.message}
          />

          <details
            className="provider-advanced"
            data-testid="provider-advanced-options"
          >
            <summary>Advanced options</summary>
            <ProviderOptionBoxes
              provider="web"
              values={webProviderOptions}
              onValueChange={onWebProviderOptionChange}
            />
          </details>
        </section>
      ) : (
        <section className="setup-section" aria-label="Executable provider">
          <label className="field">
            <span>Executable endpoint</span>
            <input
              value={executableEndpoint}
              data-testid="executable-endpoint-input"
              onChange={(event) => onExecutableEndpointChange(event.target.value)}
              spellCheck={false}
            />
          </label>

          <label className="field">
            <span>Backend</span>
            <select
              value={backend}
              data-testid="backend-select"
              onChange={(event) => onBackendChange(event.target.value)}
            >
              {backendOptions.map((option) => (
                <option key={option.value} value={option.value} disabled={option.disabled}>
                  {option.label}
                </option>
              ))}
            </select>
          </label>

          <button
            type="button"
            className="secondary-button full-width"
            data-testid="connect-sidecar-button"
            onClick={onConnectExecutable}
            disabled={executableConnected || executableStatus.state === "checking"}
          >
            <Server size={16} aria-hidden="true" />
            <span>{executableConnected ? "Connected" : "Connect sidecar"}</span>
          </button>

          <StatusCard
            state={executableStatus.state}
            label={executableStatus.label}
            detail={executableStatus.detail}
          />

          <section className="runtime-control" aria-label="Runtime control">
            <div className="runtime-control-header">
              <strong>Runtime control</strong>
              <span>{sidecarControlConnected ? "WebSocket connected" : "Manual sidecar required"}</span>
            </div>
            <pre
              className="sidecar-command"
              data-testid="manual-sidecar-command"
            >
              {manualSidecarCommand}
            </pre>
            <StatusCard
              state={runtimeStatusPanel.state}
              label={runtimeStatusPanel.label}
              detail={runtimeStatusPanel.detail}
            />
            <div className="runtime-button-grid">
              <button
                type="button"
                className="secondary-button"
                data-testid="connect-runtime-control-button"
                onClick={onConnectRuntimeControl}
              >
                Connect control
              </button>
              <button
                type="button"
                className="secondary-button"
                data-testid="start-runtime-release-button"
                disabled={!sidecarControlConnected}
                onClick={() => onStartRuntime("release")}
              >
                Start release
              </button>
              <button
                type="button"
                className="secondary-button"
                data-testid="start-runtime-debug-button"
                disabled={!sidecarControlConnected}
                onClick={() => onStartRuntime("debug")}
              >
                Start debug
              </button>
              <button
                type="button"
                className="secondary-button"
                data-testid="restart-runtime-release-button"
                disabled={!sidecarControlConnected}
                onClick={() => onRestartRuntime("release")}
              >
                Restart release
              </button>
              <button
                type="button"
                className="secondary-button"
                data-testid="restart-runtime-debug-button"
                disabled={!sidecarControlConnected}
                onClick={() => onRestartRuntime("debug")}
              >
                Restart debug
              </button>
              <button
                type="button"
                className="secondary-button"
                data-testid="stop-runtime-button"
                disabled={!sidecarControlConnected}
                onClick={onStopRuntime}
              >
                Stop runtime
              </button>
            </div>
            <details className="debug-log">
              <summary>Debug output</summary>
              <pre data-testid="runtime-log-output">
                {sidecarLogs.length > 0
                  ? sidecarLogs
                      .map(
                        (entry) =>
                          `${entry.seq} ${entry.source} ${entry.stream} ${entry.line}`,
                      )
                      .join("\n")
                  : "No runtime output yet."}
              </pre>
            </details>
          </section>

          <details
            className="provider-advanced"
            data-testid="provider-advanced-options"
          >
            <summary>Advanced options</summary>
            <ProviderOptionBoxes
              provider="executable"
              values={{
                ...executableProviderOptions,
                endpoint: executableEndpoint,
                backend,
              }}
              onValueChange={onExecutableProviderOptionChange}
            />
          </details>
        </section>
      )}
    </aside>
  );
}

function createRuntimeStatusPanel(
  runtimeStatus: SidecarRuntimeStatus | null,
): StatusPanel {
  if (!runtimeStatus) {
    return {
      state: "idle",
      label: "Runtime unknown",
      detail: "Connect WebSocket control to inspect LiteRT-LM.",
    };
  }

  const mode = runtimeStatus.mode ? ` ${runtimeStatus.mode}` : "";
  const detail = runtimeStatus.detail ?? "No runtime detail is available.";

  if (runtimeStatus.state === "running") {
    return {
      state: "ready",
      label: `Runtime running${mode}`,
      detail,
    };
  }

  if (runtimeStatus.state === "starting") {
    return {
      state: "checking",
      label: `Runtime starting${mode}`,
      detail,
    };
  }

  if (runtimeStatus.state === "unavailable" || runtimeStatus.state === "exited") {
    return {
      state: "blocked",
      label: `Runtime ${runtimeStatus.state}`,
      detail,
    };
  }

  return {
    state: "idle",
    label: `Runtime ${runtimeStatus.state}${mode}`,
    detail,
  };
}

function StatusCard({ state, label, detail }: StatusPanel) {
  return (
    <div className={`status-card ${state}`}>
      {getStatusIcon(state)}
      <div>
        <strong>{label}</strong>
        <span>{detail}</span>
      </div>
    </div>
  );
}
