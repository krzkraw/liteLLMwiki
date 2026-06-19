import {
  Activity,
  CheckCircle2,
  ChevronRight,
  Cpu,
  Database,
  Globe2,
  Loader2,
  MonitorCog,
  PlugZap,
  Search,
  Server,
  Settings2,
} from "lucide-react";
import type { ChangeEvent } from "react";
import type { ModelProbeState } from "../lib/modelConfig";
import type { ProviderOptionValues } from "../lib/providers/providerOptionState";
import type { ProviderOptionValue } from "../lib/providers/providerOptionMetadata";
import type {
  SidecarModelCatalogState,
  SidecarModelEntry,
  SidecarRuntimeStatus,
} from "../lib/providers/sidecarClient";
import type {
  SidecarLogEntry,
  SidecarRuntimeMode,
} from "../lib/providers/sidecarControlClient";
import { createSidecarWebSocketUrl } from "../lib/providers/sidecarControlClient";
import {
  createSidecarEndpoint,
  normalizeExecutableEndpoint,
} from "../lib/providers/endpoint";
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
  sidecarModelCatalog: SidecarModelCatalogState;
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
  sidecarModelCatalog,
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
  const endpointRows = createEndpointRows(executableEndpoint);
  const configRows = createExecutableConfigRows({
    endpoint: executableEndpoint,
    backend,
    runtimeStatus,
    values: executableProviderOptions,
  });

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

          <EndpointMapPanel rows={endpointRows} />
          <ModelCatalogPanel catalog={sidecarModelCatalog} />
          <ConfigPanel rows={configRows} />

          <details
            className="setup-compact-panel runtime-control"
            data-testid="sidecar-runtime-panel"
            open
          >
            <summary>
              <span className="compact-summary-title">
                <Activity size={15} aria-hidden="true" />
                Runtime
              </span>
              <span className="compact-summary-meta">
                {sidecarControlConnected
                  ? "WebSocket connected"
                  : "Manual sidecar required"}
              </span>
              <ChevronRight size={15} aria-hidden="true" />
            </summary>
            <div className="compact-panel-body">
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
                <summary>Logs</summary>
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
            </div>
          </details>

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

interface EndpointRow {
  label: string;
  value: string;
}

interface ConfigRow {
  label: string;
  value: string;
}

function EndpointMapPanel({ rows }: { rows: EndpointRow[] }) {
  return (
    <details
      className="setup-compact-panel endpoint-map"
      data-testid="sidecar-endpoints-panel"
      open
    >
      <summary>
        <span className="compact-summary-title">
          <Globe2 size={15} aria-hidden="true" />
          Endpoints
        </span>
        <span className="compact-summary-meta">{rows.length} routes</span>
        <ChevronRight size={15} aria-hidden="true" />
      </summary>
      <dl className="compact-kv-grid">
        {rows.map((row) => (
          <div key={row.label}>
            <dt>{row.label}</dt>
            <dd>{row.value}</dd>
          </div>
        ))}
      </dl>
    </details>
  );
}

function ModelCatalogPanel({ catalog }: { catalog: SidecarModelCatalogState }) {
  return (
    <details
      className={`setup-compact-panel model-catalog ${catalog.state}`}
      data-testid="sidecar-models-panel"
      open
    >
      <summary>
        <span className="compact-summary-title">
          <Database size={15} aria-hidden="true" />
          Models
        </span>
        <span className="compact-summary-meta">{catalog.detail}</span>
        <ChevronRight size={15} aria-hidden="true" />
      </summary>
      <div className="model-catalog-list">
        {catalog.models.length > 0 ? (
          catalog.models.map((model) => (
            <ModelCatalogRow key={model.id} model={model} />
          ))
        ) : (
          <p className="empty-panel-copy">{catalog.detail}</p>
        )}
      </div>
    </details>
  );
}

function ModelCatalogRow({ model }: { model: SidecarModelEntry }) {
  return (
    <article className="model-catalog-row">
      <div>
        <strong>{model.id}</strong>
        <span>{model.filename || model.targetPath}</span>
      </div>
      <div className="model-catalog-meta" aria-label={`${model.id} metadata`}>
        <span>{model.runtime}</span>
        <span>{model.role}</span>
        <span className={`state-chip ${model.state}`}>{model.state}</span>
        {model.required ? <span>required</span> : null}
        {model.sizeBytes ? <span>{formatBytes(model.sizeBytes)}</span> : null}
      </div>
      <small>{model.targetPath}</small>
      {model.lastError ? <em>{model.lastError}</em> : null}
    </article>
  );
}

function ConfigPanel({ rows }: { rows: ConfigRow[] }) {
  return (
    <details
      className="setup-compact-panel config-map"
      data-testid="sidecar-config-panel"
      open
    >
      <summary>
        <span className="compact-summary-title">
          <Settings2 size={15} aria-hidden="true" />
          Config
        </span>
        <span className="compact-summary-meta">{rows.length} keys</span>
        <ChevronRight size={15} aria-hidden="true" />
      </summary>
      <dl className="compact-kv-grid">
        {rows.map((row) => (
          <div key={row.label}>
            <dt>{row.label}</dt>
            <dd>{row.value}</dd>
          </div>
        ))}
      </dl>
    </details>
  );
}

function createEndpointRows(endpoint: string): EndpointRow[] {
  const normalized = normalizeExecutableEndpoint(endpoint);

  return [
    { label: "Chat", value: `${normalized}/chat/completions` },
    { label: "Embeddings", value: `${normalized}/embeddings` },
    { label: "Rerank", value: `${normalized}/rerank` },
    { label: "OpenAI models", value: `${normalized}/models` },
    {
      label: "Sidecar status",
      value: createSidecarEndpoint(normalized, "/sidecar/v1/status"),
    },
    {
      label: "Sidecar models",
      value: createSidecarEndpoint(normalized, "/sidecar/v1/models"),
    },
    {
      label: "Multimodal",
      value: createSidecarEndpoint(normalized, "/sidecar/v1/multimodal"),
    },
    { label: "Control WebSocket", value: createSidecarWebSocketUrl(normalized) },
  ];
}

function createExecutableConfigRows({
  endpoint,
  backend,
  runtimeStatus,
  values,
}: {
  endpoint: string;
  backend: string;
  runtimeStatus: SidecarRuntimeStatus | null;
  values: Partial<ProviderOptionValues>;
}): ConfigRow[] {
  const modelId = optionText(
    values.modelId,
    runtimeStatus?.modelId ?? "gemma4-e2b",
  );
  const runtimeHost = optionText(values.runtimeHost, "127.0.0.1");
  const runtimePort = optionText(values.runtimePort, "9381");
  const modelFile = optionText(
    values.modelFile,
    runtimeStatus?.modelFile ?? "models/litert/main/gemma-4-E2B-it.litertlm",
  );

  return [
    { label: "Endpoint", value: normalizeExecutableEndpoint(endpoint) },
    { label: "Backend", value: backend },
    { label: "Model ID", value: modelId },
    { label: "Runtime host", value: runtimeHost },
    { label: "Runtime port", value: runtimePort },
    { label: "Model file", value: modelFile },
    {
      label: "Upstream",
      value: optionText(values.upstream, runtimeStatus?.upstream ?? "auto"),
    },
    { label: "Output tokens", value: optionText(values.maxTokens, "1024") },
    { label: "Launch runtime", value: optionText(values.launchRuntime, "true") },
    { label: "Import model", value: optionText(values.importModel, "true") },
  ];
}

function optionText(
  value: ProviderOptionValue | undefined,
  fallback: string,
): string {
  if (value === undefined || value === "") {
    return fallback;
  }

  return String(value);
}

function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) {
    return "0 B";
  }

  const units = ["B", "KB", "MB", "GB"];
  let amount = value;
  let unitIndex = 0;

  while (amount >= 1024 && unitIndex < units.length - 1) {
    amount /= 1024;
    unitIndex += 1;
  }

  const rounded =
    amount >= 10 || unitIndex === 0 ? Math.round(amount) : amount.toFixed(1);
  return `${rounded} ${units[unitIndex]}`;
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
