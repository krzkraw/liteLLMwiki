import { useEffect, useMemo, useRef, useState } from "react";
import { ChatWorkspace } from "./components/ChatWorkspace";
import { FolderPanel } from "./components/FolderPanel";
import { KnowledgeGraphView } from "./components/KnowledgeGraphView";
import {
  type AppProviderKind,
  type BackendOption,
  ProviderSetup,
  type StatusPanel,
} from "./components/ProviderSetup";
import type {
  BrowserProviderConfig,
  ChatMessage,
  ChatProvider,
  ModelSource,
} from "./lib/chatProvider";
import { createChatAttachments } from "./lib/attachments";
import {
  appendAssistantText,
  failAssistantMessage,
  startAssistantTurn,
} from "./lib/chatState";
import {
  addChatSession,
  closeChatSession,
  createChatSession,
  updateChatSessionMessages,
  updateChatSessionPrompt,
  type ChatSession,
} from "./lib/chatSessions";
import {
  collectFilesFromDirectory,
  collectFilesFromFileList,
  detectFolderCapabilities,
  selectDirectoryHandle,
  type SourceFile,
} from "./lib/folderAccess";
import { indexFolder, type FolderManifest } from "./lib/folderIndex";
import { buildKnowledgeGraph, type KnowledgeGraph } from "./lib/knowledgeGraph";
import { createLiteRtBrowserProvider } from "./lib/litertBrowserProvider";
import {
  classifySelectedModelFile,
  defaultModelPath,
  defaultWasmPath,
  getModelStatusMessage,
  getSuggestedModelUrl,
  probeModelPath,
  type ModelProbeState,
} from "./lib/modelConfig";
import {
  type BackendStatus,
  getDefaultBackendStatuses,
  type NativeBackend,
} from "./lib/providers/backendStatus";
import {
  getProviderOptionDefinition,
  type ProviderOptionProvider,
  type ProviderOptionValue,
} from "./lib/providers/providerOptionMetadata";
import {
  createDefaultProviderOptionValues,
  setProviderOptionValue,
  type ProviderOptionValues,
} from "./lib/providers/providerOptionState";
import {
  createExecutableProvider,
  type ExecutableProviderConfig,
  type ExecutableProvider,
} from "./lib/providers/executableProvider";
import {
  createSidecarControlClient,
  type SidecarControlClient,
  type SidecarLogEntry,
  type SidecarRuntimeConfig,
  type SidecarRuntimeMode,
} from "./lib/providers/sidecarControlClient";
import {
  getDefaultSidecarCapabilities,
  type MultimodalCapability,
  type SidecarModelCatalogState,
  type SidecarModelEntry,
  type SidecarStatus,
} from "./lib/providers/sidecarClient";
import { normalizeExecutableEndpoint } from "./lib/providers/endpoint";
import { summarizeRuntimeAudit } from "./lib/runtimeAudit";
import {
  createInitialRuntimeMetrics,
  resetRuntimeMetrics,
  updateRuntimeMetrics,
} from "./lib/runtimeMetrics";
import {
  createProviderSummarizer,
  createDeterministicSummarizer,
  summarizeFolderManifest,
  type FolderSummarizer,
  type ProviderSummarizerOptions,
  type SummaryResult,
  type TextGenerationProvider,
} from "./lib/summarization";
import { detectWebGpu, type WebGpuStatus } from "./lib/webgpu";
import "./styles.css";

const defaultPrompt = "Explain how Gemma runs locally with LiteRT.";
const defaultSystemPrompt = "You are a concise local assistant running through LiteRT-LM.";
const defaultContextLength = 8192;
const thinkingPromptSuffix =
  "Thinking mode: reason internally when useful, then return only the final answer unless the user asks for reasoning.";
const executableEndpointDefault = "http://127.0.0.1:9379/v1";
const executableModelId = "gemma4-e2b";
const initialChatSessionId = "chat-1";
const initialModelSourceKey = createModelSourceKey(defaultModelPath, null);
const initialSidecarModelCatalog: SidecarModelCatalogState = {
  state: "idle",
  models: [],
  detail: "Connect sidecar to inspect model catalog.",
};
const assistantWelcome: ChatMessage = {
  id: "assistant-welcome",
  role: "assistant",
  text: "Load the Gemma 4 E2B web model, then send a text prompt. Executable provider controls are staged for the local sidecar.",
  status: "complete",
};

function createInitialAppChatSession(): ChatSession {
  return createChatSession({
    id: initialChatSessionId,
    title: "Chat 1",
    messages: [assistantWelcome],
    prompt: defaultPrompt,
  });
}

interface ModelFileIdentity {
  name: string;
  size: number;
  lastModified: number;
}

export type FolderSummaryMode = "provider" | "deterministic";

export interface AsyncModelResultGuard {
  requestId: number;
  latestRequestId: number;
  sourceKey: string;
  currentSourceKey: string;
}

export interface AsyncFolderResultGuard {
  requestId: number;
  latestRequestId: number;
  selectionKey: string;
  currentSelectionKey: string;
  summaryKey?: string;
  currentSummaryKey?: string;
}

export function createModelSourceKey(
  modelPath: string,
  localModelFile: ModelFileIdentity | null,
): string {
  if (localModelFile) {
    return `file:${localModelFile.name}:${localModelFile.size}:${localModelFile.lastModified}`;
  }

  return `path:${modelPath}`;
}

export function createFolderSelectionKey(files: SourceFile[]): string {
  if (files.length === 0) {
    return "empty";
  }

  return [...files]
    .sort((left, right) => left.path.localeCompare(right.path))
    .map(
      (source) =>
        `${source.path}:${source.file.size}:${source.file.lastModified}`,
    )
    .join("|");
}

export function createFolderSummaryKey({
  providerKind,
  summaryMode,
  modelSourceKey,
  executableEndpoint,
  backend,
}: {
  providerKind: AppProviderKind;
  summaryMode: FolderSummaryMode;
  modelSourceKey: string;
  executableEndpoint: string;
  backend: NativeBackend;
}): string {
  if (providerKind === "web") {
    return `${summaryMode}:web:${modelSourceKey}`;
  }

  return `${summaryMode}:executable:${executableEndpoint.trim() || executableEndpointDefault}:${backend}`;
}

export function shouldApplyAsyncModelResult({
  requestId,
  latestRequestId,
  sourceKey,
  currentSourceKey,
}: AsyncModelResultGuard): boolean {
  return requestId === latestRequestId && sourceKey === currentSourceKey;
}

export function shouldApplyAsyncFolderResult({
  requestId,
  latestRequestId,
  selectionKey,
  currentSelectionKey,
  summaryKey,
  currentSummaryKey,
}: AsyncFolderResultGuard): boolean {
  const summaryKeyMatches =
    summaryKey === undefined && currentSummaryKey === undefined
      ? true
      : summaryKey === currentSummaryKey;

  return (
    requestId === latestRequestId &&
    selectionKey === currentSelectionKey &&
    summaryKeyMatches
  );
}

export function buildEffectiveSystemPrompt(
  systemPrompt: string,
  thinkingEnabled: boolean,
): string {
  const trimmedSystemPrompt = systemPrompt.trim();

  if (!thinkingEnabled) {
    return trimmedSystemPrompt;
  }

  return trimmedSystemPrompt
    ? `${trimmedSystemPrompt}\n\n${thinkingPromptSuffix}`
    : thinkingPromptSuffix;
}

function backendLabel(backend: NativeBackend): string {
  if (backend === "npu") {
    return "NPU";
  }

  if (backend === "gpu") {
    return "GPU";
  }

  if (backend === "cpu") {
    return "CPU";
  }

  if (backend === "cuda") {
    return "CUDA";
  }

  return "Auto";
}

function isStoppedRuntimeState(state: string | undefined): boolean {
  return state === "stopped" || state === "exited" || state === "unavailable";
}

function optionNumber(
  values: ProviderOptionValues,
  id: string,
  fallback: number,
): number {
  const value = values[id];

  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function optionalInteger(
  values: ProviderOptionValues,
  id: string,
): number | undefined {
  const value = values[id];
  const parsed =
    typeof value === "number"
      ? value
      : typeof value === "string"
        ? Number(value.trim())
        : NaN;

  return Number.isSafeInteger(parsed) ? parsed : undefined;
}

function positiveOptionInteger(
  values: ProviderOptionValues,
  id: string,
): number | undefined {
  const value = optionalInteger(values, id);
  return value !== undefined && value > 0 ? value : undefined;
}

function optionString(
  values: ProviderOptionValues,
  id: string,
  fallback: string,
): string {
  const value = values[id];

  return typeof value === "string" ? value : fallback;
}

function optionBoolean(
  values: ProviderOptionValues,
  id: string,
  fallback: boolean,
): boolean {
  const value = values[id];

  return typeof value === "boolean" ? value : fallback;
}

function normalizeStopTokenIds(value: unknown): number[][] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }

  const rows: number[][] = [];

  for (const row of value) {
    if (!Array.isArray(row)) {
      return undefined;
    }

    const parsedRow: number[] = [];

    for (const tokenId of row) {
      const parsed =
        typeof tokenId === "number"
          ? tokenId
          : typeof tokenId === "string" && tokenId.trim()
            ? Number(tokenId.trim())
            : NaN;

      if (!Number.isSafeInteger(parsed) || parsed < 0) {
        return undefined;
      }

      parsedRow.push(parsed);
    }

    if (parsedRow.length > 0) {
      rows.push(parsedRow);
    }
  }

  return rows.length > 0 ? rows : undefined;
}

function parseStopTokenIds(
  value: ProviderOptionValue | undefined,
): number[][] | undefined {
  if (typeof value !== "string") {
    return undefined;
  }

  const trimmed = value.trim();

  if (!trimmed) {
    return undefined;
  }

  if (trimmed.startsWith("[")) {
    try {
      return normalizeStopTokenIds(JSON.parse(trimmed));
    } catch {
      return undefined;
    }
  }

  const rows = trimmed
    .split(";")
    .map((row) => row.split(",").map((item) => item.trim()).filter(Boolean))
    .filter((row) => row.length > 0);

  return normalizeStopTokenIds(rows);
}

export function createBrowserProviderLoadConfig({
  model,
  values,
  contextLength,
  systemPrompt,
}: {
  model: ModelSource;
  values: ProviderOptionValues;
  contextLength: number;
  systemPrompt: string;
}): BrowserProviderConfig {
  const stopTokenIds = parseStopTokenIds(values.stopTokenIds);
  const startTokenId = positiveOptionInteger(values, "startTokenId");
  const numOutputCandidates = positiveOptionInteger(
    values,
    "numOutputCandidates",
  );

  return {
    model,
    wasmPath: optionString(values, "wasmPath", defaultWasmPath),
    maxNumTokens: optionNumber(values, "maxNumTokens", contextLength),
    maxOutputTokens: optionNumber(values, "maxOutputTokens", 1024),
    engineBackend: optionString(values, "engineBackend", "GPU"),
    samplerBackend: optionString(values, "samplerBackend", "UNSPECIFIED"),
    samplerType: optionString(values, "samplerType", "TOP_K"),
    temperature: optionNumber(values, "temperature", 0.7),
    topK: optionNumber(values, "topK", 40),
    topP: optionNumber(values, "topP", 0.95),
    seed: optionNumber(values, "seed", 0),
    ...(stopTokenIds ? { stopTokenIds } : {}),
    ...(startTokenId !== undefined ? { startTokenId } : {}),
    ...(numOutputCandidates !== undefined ? { numOutputCandidates } : {}),
    applyPromptTemplateInSession: optionBoolean(
      values,
      "applyPromptTemplateInSession",
      true,
    ),
    useExternalSampler: optionBoolean(values, "useExternalSampler", false),
    enableConstrainedDecoding: optionBoolean(
      values,
      "enableConstrainedDecoding",
      false,
    ),
    prefillPrefaceOnInit: optionBoolean(values, "prefillPrefaceOnInit", false),
    filterChannelContentFromKvCache: optionBoolean(
      values,
      "filterChannelContentFromKvCache",
      false,
    ),
    systemPrompt,
  };
}

export function createSidecarRuntimeConfig(
  values: ProviderOptionValues,
): SidecarRuntimeConfig {
  const runtimeExe = optionString(values, "runtimeExe", "");
  const upstream = optionString(values, "upstream", "");
  const modelFile = optionString(
    values,
    "modelFile",
    "models/gemma-4-E2B-it.litertlm",
  );

  return {
    ...(upstream ? { upstream } : {}),
    ...(runtimeExe ? { runtimeExe } : {}),
    runtimeHost: optionString(values, "runtimeHost", "127.0.0.1"),
    runtimePort: optionNumber(values, "runtimePort", 9381),
    ...(modelFile ? { modelFile } : {}),
    modelId: optionString(values, "modelId", executableModelId),
    huggingfaceToken: optionString(values, "huggingfaceToken", ""),
    importModel: optionBoolean(values, "importModel", true),
    launchRuntime: optionBoolean(values, "launchRuntime", true),
    runtimeVerbose: optionBoolean(values, "runtimeVerbose", false),
  };
}

export function createExecutableProviderLoadConfig({
  endpoint,
  backend,
  values,
  contextLength,
  systemPrompt,
}: {
  endpoint: string;
  backend: NativeBackend;
  values: ProviderOptionValues;
  contextLength: number;
  systemPrompt: string;
}): ExecutableProviderConfig {
  return {
    endpoint,
    modelId: optionString(values, "modelId", executableModelId),
    backend,
    maxNumTokens: optionNumber(values, "maxNumTokens", contextLength),
    maxTokens: optionNumber(values, "maxTokens", 1024),
    systemPrompt,
    temperature: optionNumber(values, "temperature", 0.7),
    topK: optionNumber(values, "topK", 40),
    topP: optionNumber(values, "topP", 0.95),
    seed: optionNumber(values, "seed", 0),
    visionBackend: optionString(values, "visionBackend", "auto"),
    audioBackend: optionString(values, "audioBackend", "auto"),
    preset: optionString(values, "preset", ""),
    noTemplate: optionBoolean(values, "noTemplate", false),
    filterChannelContentFromKvCache: optionBoolean(
      values,
      "filterChannelContentFromKvCache",
      false,
    ),
    enableSpeculativeDecoding: optionString(
      values,
      "enableSpeculativeDecoding",
      "auto",
    ),
    cache: optionString(values, "cache", "disk"),
    verbose: optionBoolean(values, "verbose", false),
    fromHuggingFaceRepo: optionString(values, "fromHuggingFaceRepo", ""),
    huggingfaceToken: optionString(values, "huggingfaceToken", ""),
  };
}

export function createBackendOptions(statuses: BackendStatus[]): BackendOption[] {
  const statusByBackend = new Map(
    statuses.map((status) => [status.backend, status.state]),
  );
  const nativeBackends: NativeBackend[] = ["auto", "cpu", "gpu", "npu", "cuda"];

  return nativeBackends.map((backend) => {
    if (backend === "auto") {
      return { value: backend, label: "Auto" };
    }

    const state = statusByBackend.get(backend) ?? "unknown";

    if (state === "available") {
      return { value: backend, label: backendLabel(backend) };
    }

    if (state === "not-a-litert-backend") {
      return {
        value: backend,
        label: `${backendLabel(backend)} probe-only`,
        disabled: true,
      };
    }

    return {
      value: backend,
      label: `${backendLabel(backend)} ${state}`,
      disabled: true,
    };
  });
}

export function createSidecarStatusPanel(status: SidecarStatus): StatusPanel {
  if (status.state === "available") {
    const detail =
      status.detail ??
      status.backends
        .map((backendStatus) => `${backendStatus.backend}: ${backendStatus.state}`)
        .join(", ");

    return {
      state: "ready",
      label: "Sidecar connected",
      detail: detail || "Executable provider is reachable.",
    };
  }

  if (status.state === "unavailable") {
    return {
      state: "blocked",
      label: "Sidecar unavailable",
      detail: status.detail ?? "Start the local executable provider sidecar.",
    };
  }

  return {
    state: "idle",
    label: "Sidecar status unknown",
    detail: status.detail ?? "Connect to probe native backends.",
  };
}

function isObjectRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function stringField(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

function optionalNumberField(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function normalizeSidecarModelEntry(value: unknown): SidecarModelEntry | null {
  if (!isObjectRecord(value)) {
    return null;
  }

  const id = stringField(value.id);
  if (!id) {
    return null;
  }

  return {
    id,
    repo: stringField(value.repo),
    filename: stringField(value.filename),
    targetPath: stringField(value.targetPath),
    runtime: stringField(value.runtime, "unknown"),
    role: stringField(value.role, "unknown"),
    required: Boolean(value.required),
    state: stringField(value.state, "unknown"),
    bytesDownloaded: optionalNumberField(value.bytesDownloaded),
    sizeBytes: optionalNumberField(value.sizeBytes),
    lastError: stringField(value.lastError),
  };
}

export function normalizeSidecarModelCatalogResponse(
  body: unknown,
): SidecarModelEntry[] {
  if (!isObjectRecord(body) || !Array.isArray(body.models)) {
    return [];
  }

  return body.models
    .map((entry) => normalizeSidecarModelEntry(entry))
    .filter((entry): entry is SidecarModelEntry => entry !== null);
}

export function resolveExecutableBackend(
  selectedBackend: NativeBackend,
  statuses: BackendStatus[],
): NativeBackend {
  if (selectedBackend === "auto") {
    return "auto";
  }

  return statuses.some(
    (status) =>
      status.backend === selectedBackend && status.state === "available",
  )
    ? selectedBackend
    : "auto";
}

export function canUseNativeAttachments({
  providerKind,
  executableLoaded,
  multimodalState,
}: {
  providerKind: AppProviderKind;
  executableLoaded: boolean;
  multimodalState: MultimodalCapability["state"];
}): boolean {
  return (
    providerKind === "executable" &&
    executableLoaded &&
    multimodalState === "available"
  );
}

export function createFolderSummarizerForProvider(
  provider: TextGenerationProvider | null,
  providerLoaded: boolean,
  options: ProviderSummarizerOptions = {},
): { mode: FolderSummaryMode; summarizer: FolderSummarizer } {
  if (provider && providerLoaded) {
    return {
      mode: "provider",
      summarizer: createProviderSummarizer(provider, options),
    };
  }

  return {
    mode: "deterministic",
    summarizer: createDeterministicSummarizer(),
  };
}

export function getDisplayedFolderSummaryMode({
  hasSummaryResult,
  summaryMode,
  currentMode,
}: {
  hasSummaryResult: boolean;
  summaryMode: FolderSummaryMode;
  currentMode: FolderSummaryMode;
}): FolderSummaryMode {
  return hasSummaryResult ? summaryMode : currentMode;
}

function completeAssistantMessage(messages: ChatMessage[], assistantId: string): ChatMessage[] {
  return messages.map((message) =>
    message.id === assistantId && message.status !== "error"
      ? { ...message, status: "complete" }
      : message,
  );
}

function finishAssistantWithText(
  messages: ChatMessage[],
  assistantId: string,
  text: string,
): ChatMessage[] {
  return messages.map((message) =>
    message.id === assistantId
      ? { ...message, text: message.text || text, status: "complete" }
      : message,
  );
}

function isAbortError(error: unknown): boolean {
  return (
    typeof DOMException !== "undefined" &&
    error instanceof DOMException &&
    error.name === "AbortError"
  );
}

function createEmptyKnowledgeGraph(): KnowledgeGraph {
  return { nodes: [], edges: [] };
}

export default function App() {
  const browserProviderRef = useRef<ChatProvider | null>(null);
  const executableProviderRef = useRef<ExecutableProvider | null>(null);
  const sidecarControlClientRef = useRef<SidecarControlClient | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const folderSummaryAbortRef = useRef<AbortController | null>(null);
  const modelSourceKeyRef = useRef(initialModelSourceKey);
  const modelProbeRequestIdRef = useRef(0);
  const modelLoadRequestIdRef = useRef(0);
  const folderRequestIdRef = useRef(0);
  const folderSummaryRequestIdRef = useRef(0);
  const folderSelectionKeyRef = useRef(createFolderSelectionKey([]));
  const folderSummaryKeyRef = useRef(
    createFolderSummaryKey({
      providerKind: "web",
      summaryMode: "deterministic",
      modelSourceKey: initialModelSourceKey,
      executableEndpoint: executableEndpointDefault,
      backend: "auto",
    }),
  );
  const executableLoadRequestIdRef = useRef(0);
  const sidecarCatalogRequestIdRef = useRef(0);
  const [providerKind, setProviderKind] = useState<AppProviderKind>("web");
  const [webGpu, setWebGpu] = useState<WebGpuStatus>({
    state: "checking",
    label: "Checking WebGPU",
    detail: "Requesting a high-performance GPU adapter.",
  });
  const [modelPath, setModelPath] = useState(defaultModelPath);
  const [localModelFile, setLocalModelFile] = useState<File | null>(null);
  const [modelProbe, setModelProbe] = useState<{
    state: ModelProbeState;
    message: string;
  }>({
    state: "idle",
    message: "Check the configured path or choose the Gemma 4 E2B web model file.",
  });
  const [modelLoaded, setModelLoaded] = useState(false);
  const [executableLoaded, setExecutableLoaded] = useState(false);
  const [isConnectingExecutable, setIsConnectingExecutable] = useState(false);
  const [isLoadingModel, setIsLoadingModel] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [chatSessions, setChatSessions] = useState<ChatSession[]>(() => [
    createInitialAppChatSession(),
  ]);
  const [activeChatId, setActiveChatId] = useState(initialChatSessionId);
  const [systemPrompt, setSystemPrompt] = useState(defaultSystemPrompt);
  const [contextLength, setContextLength] = useState(defaultContextLength);
  const [thinkingEnabled, setThinkingEnabled] = useState(false);
  const [runtimeMetrics, setRuntimeMetrics] = useState(createInitialRuntimeMetrics);
  const [chatDebugLines, setChatDebugLines] = useState<string[]>([]);
  const [webProviderOptions, setWebProviderOptions] =
    useState<ProviderOptionValues>(() => createDefaultProviderOptionValues("web"));
  const [executableProviderOptions, setExecutableProviderOptions] =
    useState<ProviderOptionValues>(() =>
      createDefaultProviderOptionValues("executable"),
    );
  const [isGenerating, setIsGenerating] = useState(false);
  const [executableEndpoint, setExecutableEndpoint] = useState(executableEndpointDefault);
  const [backend, setBackend] = useState<NativeBackend>("auto");
  const [sidecarStatus, setSidecarStatus] = useState<SidecarStatus>({
    state: "unknown",
    backends: getDefaultBackendStatuses(),
    capabilities: getDefaultSidecarCapabilities(),
    detail: `Endpoint ${executableEndpointDefault} is configured.`,
  });
  const [sidecarControlConnected, setSidecarControlConnected] = useState(false);
  const [sidecarModelCatalog, setSidecarModelCatalog] =
    useState<SidecarModelCatalogState>(initialSidecarModelCatalog);
  const [sidecarLogs, setSidecarLogs] = useState<SidecarLogEntry[]>([]);
  const [pendingAttachments, setPendingAttachments] = useState<
    ReturnType<typeof createChatAttachments>
  >([]);
  const [folderSources, setFolderSources] = useState<SourceFile[]>([]);
  const [folderCapabilities] = useState(() => detectFolderCapabilities());
  const [folderManifest, setFolderManifest] = useState<FolderManifest | null>(null);
  const [folderSummaryResult, setFolderSummaryResult] =
    useState<SummaryResult | null>(null);
  const [folderSummaryMode, setFolderSummaryMode] =
    useState<FolderSummaryMode>("deterministic");
  const [knowledgeGraph, setKnowledgeGraph] = useState<KnowledgeGraph>(
    createEmptyKnowledgeGraph,
  );
  const [isSelectingDirectory, setIsSelectingDirectory] = useState(false);
  const [isIndexingFolder, setIsIndexingFolder] = useState(false);
  const [isSummarizingFolder, setIsSummarizingFolder] = useState(false);
  const [folderError, setFolderError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;

    void detectWebGpu()
      .then((status) => {
        if (active) {
          setWebGpu(status);
        }
      })
      .catch((error: unknown) => {
        if (active) {
          setWebGpu({
            state: "blocked",
            label: "WebGPU check failed",
            detail: error instanceof Error ? error.message : String(error),
          });
        }
      });

    return () => {
      active = false;
      abortControllerRef.current?.abort();
      abortActiveFolderSummary();
      sidecarControlClientRef.current?.close();
      sidecarControlClientRef.current = null;
      disposeBrowserProvider();
      disposeExecutableProvider();
    };
  }, []);

  const runtimeAudit = useMemo(
    () =>
      summarizeRuntimeAudit({
        webGpuState: webGpu.state,
        modelState: modelProbe.state,
        hasWebAssembly: typeof WebAssembly !== "undefined",
        crossOriginIsolated: typeof window !== "undefined" && window.crossOriginIsolated,
      }),
    [modelProbe.state, webGpu.state],
  );

  const executableStatus = useMemo<StatusPanel>(() => {
    if (isConnectingExecutable) {
      return {
        state: "checking",
        label: "Connecting sidecar",
        detail: `Opening WebSocket to ${executableEndpoint || executableEndpointDefault}.`,
      };
    }

    return createSidecarStatusPanel(sidecarStatus);
  }, [executableEndpoint, isConnectingExecutable, sidecarStatus]);
  const backendOptions = useMemo(
    () => createBackendOptions(sidecarStatus.backends),
    [sidecarStatus.backends],
  );
  const multimodalCapability = sidecarStatus.capabilities.multimodal;
  const nativeAttachmentsEnabled = canUseNativeAttachments({
    providerKind,
    executableLoaded,
    multimodalState: multimodalCapability.state,
  });
  const attachmentStatus = nativeAttachmentsEnabled
    ? (multimodalCapability.detail ??
      "Native executable provider accepts image and audio attachments.")
    : providerKind === "executable"
      ? "Connect a sidecar with native multimodal capability for image/audio attachments."
      : "Switch to the executable provider for native image/audio attachments.";

  const selectedProviderLoaded =
    providerKind === "web" ? modelLoaded : executableLoaded;
  const selectedTextProvider =
    providerKind === "web"
      ? browserProviderRef.current
      : executableProviderRef.current;
  const currentFolderSummaryMode: FolderSummaryMode =
    selectedProviderLoaded && selectedTextProvider ? "provider" : "deterministic";
  const currentFolderSummaryKey = createFolderSummaryKey({
    providerKind,
    summaryMode: currentFolderSummaryMode,
    modelSourceKey: modelSourceKeyRef.current,
    executableEndpoint,
    backend,
  });
  const displayedFolderSummaryMode = getDisplayedFolderSummaryMode({
    hasSummaryResult: folderSummaryResult !== null,
    summaryMode: folderSummaryMode,
    currentMode: currentFolderSummaryMode,
  });
  const activeChat =
    chatSessions.find((session) => session.id === activeChatId) ?? chatSessions[0];
  const effectiveSystemPrompt = buildEffectiveSystemPrompt(
    systemPrompt,
    thinkingEnabled,
  );

  function updateActiveChatPrompt(nextPrompt: string) {
    setChatSessions((current) =>
      updateChatSessionPrompt(current, activeChat.id, nextPrompt),
    );
  }

  function updateChatMessages(
    sessionId: string,
    updater: (messages: ChatMessage[]) => ChatMessage[],
  ) {
    setChatSessions((current) =>
      updateChatSessionMessages(current, sessionId, updater),
    );
  }

  function invalidateLoadedProvidersForChatOptions() {
    if (isGenerating) {
      stopGeneration();
    }

    invalidateFolderSummary({ clearResult: true });
    setModelLoaded(false);
    setExecutableLoaded(false);
    setLoadError(null);
    disposeBrowserProvider();
    disposeExecutableProvider();
  }

  function abortActiveFolderSummary() {
    folderSummaryAbortRef.current?.abort();
    folderSummaryAbortRef.current = null;
  }

  function invalidateFolderSummary(options: { clearResult: boolean }) {
    folderSummaryRequestIdRef.current += 1;
    abortActiveFolderSummary();
    setIsSummarizingFolder(false);

    if (options.clearResult) {
      setFolderSummaryResult(null);
      setKnowledgeGraph(createEmptyKnowledgeGraph());
    }
  }

  useEffect(() => {
    if (folderSummaryKeyRef.current === currentFolderSummaryKey) {
      return;
    }

    folderSummaryKeyRef.current = currentFolderSummaryKey;
    invalidateFolderSummary({ clearResult: true });
  }, [currentFolderSummaryKey]);

  function disposeBrowserProvider() {
    const provider = browserProviderRef.current;

    if (!provider) {
      return;
    }

    provider.cancel();
    browserProviderRef.current = null;
    void provider.dispose();
  }

  function disposeExecutableProvider() {
    const provider = executableProviderRef.current;

    if (!provider) {
      return;
    }

    provider.cancel();
    executableProviderRef.current = null;
    void provider.dispose();
  }

  function closeSidecarControl() {
    sidecarControlClientRef.current?.close();
    sidecarControlClientRef.current = null;
    setSidecarControlConnected(false);
  }

  function resetSidecarModelCatalog(detail = initialSidecarModelCatalog.detail) {
    sidecarCatalogRequestIdRef.current += 1;
    setSidecarModelCatalog({
      state: "idle",
      models: [],
      detail,
    });
  }

  function invalidateModelSource(nextModelPath: string, nextLocalModelFile: File | null) {
    modelSourceKeyRef.current = createModelSourceKey(nextModelPath, nextLocalModelFile);
    modelProbeRequestIdRef.current += 1;
    modelLoadRequestIdRef.current += 1;
    setIsLoadingModel(false);
    invalidateFolderSummary({ clearResult: true });
    disposeBrowserProvider();
  }

  function stopGeneration() {
    abortControllerRef.current?.abort();
    browserProviderRef.current?.cancel();
    executableProviderRef.current?.cancel();
    appendChatDebugLine("stop requested");
    setIsGenerating(false);
  }

  function appendChatDebugLine(line: string) {
    setChatDebugLines((current) => [...current.slice(-199), line]);
  }

  function applySidecarStatus(status: SidecarStatus) {
    setSidecarStatus(status);

    if (isStoppedRuntimeState(status.runtime?.state)) {
      setExecutableLoaded(false);
      setPendingAttachments([]);
      disposeExecutableProvider();
    }
  }

  function handleProviderKindChange(nextProviderKind: AppProviderKind) {
    if (isGenerating) {
      stopGeneration();
    }

    invalidateFolderSummary({ clearResult: true });
    setProviderKind(nextProviderKind);
    setLoadError(null);
    if (nextProviderKind !== "executable") {
      setPendingAttachments([]);
      closeSidecarControl();
      resetSidecarModelCatalog();
    }
  }

  function handleExecutableEndpointChange(endpoint: string) {
    executableLoadRequestIdRef.current += 1;
    invalidateFolderSummary({ clearResult: true });
    setExecutableEndpoint(endpoint);
    updateProviderOptionState("executable", "endpoint", endpoint);
    setExecutableLoaded(false);
    setLoadError(null);
    setSidecarStatus({
      state: "unknown",
      backends: getDefaultBackendStatuses(),
      capabilities: getDefaultSidecarCapabilities(),
      detail: `Endpoint ${endpoint || executableEndpointDefault} is configured.`,
    });
    resetSidecarModelCatalog(
      `Endpoint ${endpoint || executableEndpointDefault} is configured.`,
    );
    setPendingAttachments([]);
    closeSidecarControl();
    disposeExecutableProvider();
  }

  function handleBackendChange(nextBackend: string) {
    executableLoadRequestIdRef.current += 1;
    invalidateFolderSummary({ clearResult: true });
    setBackend(nextBackend as NativeBackend);
    updateProviderOptionState("executable", "backend", nextBackend);
    setExecutableLoaded(false);
    setLoadError(null);
    setPendingAttachments([]);
    disposeExecutableProvider();
  }

  function handleNewChat() {
    const result = addChatSession(chatSessions);
    setChatSessions(result.sessions);
    setActiveChatId(result.activeSessionId);
  }

  function handleCloseChat(sessionId: string) {
    const result = closeChatSession(chatSessions, activeChatId, sessionId);
    setChatSessions(result.sessions);
    setActiveChatId(result.activeSessionId);
  }

  function handleSystemPromptChange(nextSystemPrompt: string) {
    setSystemPrompt(nextSystemPrompt);
    updateProviderOptionState("web", "systemPrompt", nextSystemPrompt);
    invalidateLoadedProvidersForChatOptions();
  }

  function handleContextLengthChange(nextContextLength: number) {
    if (!Number.isFinite(nextContextLength) || nextContextLength < 512) {
      return;
    }

    const roundedContextLength = Math.round(nextContextLength);
    setContextLength(roundedContextLength);
    updateProviderOptionState("web", "maxNumTokens", roundedContextLength);
    updateProviderOptionState("executable", "maxNumTokens", roundedContextLength);
    invalidateLoadedProvidersForChatOptions();
  }

  function updateProviderOptionState(
    provider: ProviderOptionProvider,
    id: string,
    value: ProviderOptionValue,
  ) {
    const definition = getProviderOptionDefinition(provider, id);

    if (!definition) {
      return;
    }

    const updater = (current: ProviderOptionValues) =>
      setProviderOptionValue(current, definition, value);

    if (provider === "web") {
      setWebProviderOptions(updater);
      return;
    }

    setExecutableProviderOptions(updater);
  }

  function handleWebProviderOptionChange(
    id: string,
    value: ProviderOptionValue,
    values: ProviderOptionValues,
  ) {
    setWebProviderOptions(values);

    if (id === "maxNumTokens") {
      handleContextLengthChange(Number(value));
      return;
    }

    if (id === "systemPrompt") {
      handleSystemPromptChange(String(value));
      return;
    }

    invalidateLoadedProvidersForChatOptions();
  }

  function handleExecutableProviderOptionChange(
    id: string,
    value: ProviderOptionValue,
    values: ProviderOptionValues,
  ) {
    setExecutableProviderOptions(values);

    if (id === "endpoint") {
      handleExecutableEndpointChange(String(value));
      return;
    }

    if (id === "backend") {
      handleBackendChange(String(value));
      return;
    }

    if (id === "maxNumTokens") {
      handleContextLengthChange(Number(value));
      return;
    }

    executableLoadRequestIdRef.current += 1;
    invalidateFolderSummary({ clearResult: true });
    setExecutableLoaded(false);
    setLoadError(null);
    setPendingAttachments([]);
    disposeExecutableProvider();
  }

  function handleThinkingEnabledChange(nextThinkingEnabled: boolean) {
    setThinkingEnabled(nextThinkingEnabled);
    invalidateLoadedProvidersForChatOptions();
  }

  async function refreshSidecarModelCatalog(client: SidecarControlClient) {
    const requestId = sidecarCatalogRequestIdRef.current + 1;
    sidecarCatalogRequestIdRef.current = requestId;
    setSidecarModelCatalog((current) => ({
      state: "checking",
      models: current.models,
      detail: "Reading sidecar model catalog.",
    }));

    try {
      const response = await client.request({
        method: "GET",
        path: "/sidecar/v1/models",
      });

      if (response.status < 200 || response.status >= 300) {
        const message = (await response.text()).trim();
        throw new Error(
          message || `Model catalog request failed with ${response.status}.`,
        );
      }

      const models = normalizeSidecarModelCatalogResponse(await response.json());

      if (requestId !== sidecarCatalogRequestIdRef.current) {
        return;
      }

      setSidecarModelCatalog({
        state: "ready",
        models,
        detail:
          models.length === 1
            ? "1 model detected."
            : `${models.length} models detected.`,
      });
    } catch (error) {
      if (requestId !== sidecarCatalogRequestIdRef.current) {
        return;
      }

      setSidecarModelCatalog({
        state: "blocked",
        models: [],
        detail: error instanceof Error ? error.message : String(error),
      });
    }
  }

  function openSidecarControl(
    endpoint: string,
    options: {
      onStatus?: (status: SidecarStatus) => void;
      onConnectionError?: (error: Error) => void;
      onControlError?: (error: Error) => void;
      onClose?: () => void;
    } = {},
  ) {
    closeSidecarControl();

    const client = createSidecarControlClient({
      endpoint,
      onOpen: () => {
        setSidecarControlConnected(true);
        setSidecarModelCatalog((current) =>
          current.state === "ready"
            ? current
            : {
                state: "checking",
                models: current.models,
                detail: "Opening sidecar model catalog.",
              },
        );
      },
      onClose: () => {
        setSidecarControlConnected(false);
        options.onClose?.();
      },
      onError: () => {
        const error = new Error("Sidecar WebSocket control connection failed.");
        setSidecarControlConnected(false);
        setLoadError(error.message);
        options.onConnectionError?.(error);
      },
      onEvent: (event) => {
        if (event.type === "status") {
          applySidecarStatus(event.status);
          if (sidecarControlClientRef.current) {
            void refreshSidecarModelCatalog(sidecarControlClientRef.current);
          }
          options.onStatus?.(event.status);
          return;
        }

        if (event.type === "log") {
          setSidecarLogs((current) => [...current.slice(-199), event.entry]);
          return;
        }

        options.onControlError?.(new Error(event.message));
        setLoadError(event.message);
      },
    });

    sidecarControlClientRef.current = client;
    client.connect();

    return client;
  }

  function requestSidecarStatusOverWebSocket(endpoint: string): Promise<SidecarStatus> {
    return new Promise((resolve, reject) => {
      let settled = false;
      const timeout = window.setTimeout(() => {
        finish(() => {
          closeSidecarControl();
          reject(new Error("Sidecar WebSocket did not return status in time."));
        });
      }, 8000);

      function finish(action: () => void) {
        if (settled) {
          return;
        }

        settled = true;
        window.clearTimeout(timeout);
        action();
      }

      openSidecarControl(endpoint, {
        onStatus: (status) => finish(() => resolve(status)),
        onConnectionError: (error) => finish(() => reject(error)),
        onControlError: (error) => finish(() => reject(error)),
        onClose: () =>
          finish(() =>
            reject(new Error("Sidecar WebSocket closed before status arrived.")),
          ),
      });
    });
  }

  function handleConnectRuntimeControl() {
    openSidecarControl(executableEndpoint);
  }

  function startRuntime(mode: SidecarRuntimeMode) {
    sidecarControlClientRef.current?.startRuntime(
      mode,
      createSidecarRuntimeConfig(executableProviderOptions),
    );
  }

  function restartRuntime(mode: SidecarRuntimeMode) {
    sidecarControlClientRef.current?.restartRuntime(
      mode,
      createSidecarRuntimeConfig(executableProviderOptions),
    );
  }

  function stopRuntime() {
    sidecarControlClientRef.current?.stopRuntime();
  }

  function handleAttachmentsSelected(fileList: FileList | null) {
    if (!nativeAttachmentsEnabled) {
      return;
    }

    try {
      setPendingAttachments(createChatAttachments(fileList));
      setLoadError(null);
    } catch (error) {
      setLoadError(error instanceof Error ? error.message : String(error));
    }
  }

  function handleRemoveAttachment(id: string) {
    setPendingAttachments((current) =>
      current.filter((attachment) => attachment.id !== id),
    );
  }

  function applyFolderSources(sources: SourceFile[]) {
    folderSelectionKeyRef.current = createFolderSelectionKey(sources);
    folderSummaryRequestIdRef.current += 1;
    abortActiveFolderSummary();
    setFolderSources(sources);
    setFolderManifest(null);
    setFolderSummaryResult(null);
    setKnowledgeGraph(createEmptyKnowledgeGraph());
    setFolderError(null);
    setIsIndexingFolder(false);
    setIsSummarizingFolder(false);
  }

  function handleModelPathChange(nextModelPath: string) {
    invalidateModelSource(nextModelPath, null);
    setModelPath(nextModelPath);
    setLocalModelFile(null);
    setModelLoaded(false);
    setLoadError(null);
    setModelProbe({
      state: "idle",
      message: "Check the configured path before loading the web model.",
    });
  }

  function handleLocalModelFileChange(fileList: FileList | null) {
    const file = fileList?.[0] ?? null;
    setModelLoaded(false);
    setLoadError(null);

    if (!file) {
      invalidateModelSource(modelPath, null);
      setLocalModelFile(null);
      setModelProbe({
        state: "idle",
        message: "Choose the Gemma 4 E2B web model file or check a model path.",
      });
      return;
    }

    const result = classifySelectedModelFile(file);
    const nextLocalModelFile = result.state === "ready" ? file : null;

    invalidateModelSource(modelPath, nextLocalModelFile);
    setLocalModelFile(nextLocalModelFile);
    setModelProbe({
      state: result.state,
      message: result.message,
    });
  }

  function handleFolderFilesSelected(fileList: FileList | null) {
    const sources = fileList ? collectFilesFromFileList(fileList) : [];

    folderRequestIdRef.current += 1;
    setIsSelectingDirectory(false);
    applyFolderSources(sources);
  }

  async function handleChooseDirectory() {
    if (isSelectingDirectory || isIndexingFolder || isSummarizingFolder) {
      return;
    }

    const requestId = folderRequestIdRef.current + 1;
    folderRequestIdRef.current = requestId;
    setIsSelectingDirectory(true);
    setFolderError(null);

    try {
      const handle = await selectDirectoryHandle();

      if (!handle) {
        return;
      }

      const sources = await collectFilesFromDirectory(handle);

      if (requestId !== folderRequestIdRef.current) {
        return;
      }

      applyFolderSources(sources);
    } catch (error) {
      if (requestId === folderRequestIdRef.current) {
        setFolderError(error instanceof Error ? error.message : String(error));
      }
    } finally {
      if (requestId === folderRequestIdRef.current) {
        setIsSelectingDirectory(false);
      }
    }
  }

  async function indexSelectedFolder() {
    const sources = folderSources;

    if (sources.length === 0 || isIndexingFolder || isSummarizingFolder) {
      return;
    }

    const selectionKey = createFolderSelectionKey(sources);
    const requestId = folderRequestIdRef.current + 1;
    folderRequestIdRef.current = requestId;
    folderSelectionKeyRef.current = selectionKey;
    setIsIndexingFolder(true);
    setFolderError(null);
    setFolderManifest(null);
    setFolderSummaryResult(null);
    setKnowledgeGraph(createEmptyKnowledgeGraph());

    try {
      const manifest = await indexFolder(sources);

      if (
        !shouldApplyAsyncFolderResult({
          requestId,
          latestRequestId: folderRequestIdRef.current,
          selectionKey,
          currentSelectionKey: folderSelectionKeyRef.current,
        })
      ) {
        return;
      }

      setFolderManifest(manifest);
    } catch (error) {
      if (
        shouldApplyAsyncFolderResult({
          requestId,
          latestRequestId: folderRequestIdRef.current,
          selectionKey,
          currentSelectionKey: folderSelectionKeyRef.current,
        })
      ) {
        setFolderError(error instanceof Error ? error.message : String(error));
      }
    } finally {
      if (requestId === folderRequestIdRef.current) {
        setIsIndexingFolder(false);
      }
    }
  }

  async function summarizeSelectedFolder() {
    const manifest = folderManifest;

    if (!manifest || isIndexingFolder || isSummarizingFolder) {
      return;
    }

    const selectionKey = folderSelectionKeyRef.current;
    const summaryKey = currentFolderSummaryKey;
    const requestId = folderSummaryRequestIdRef.current + 1;
    const controller = new AbortController();
    folderSummaryRequestIdRef.current = requestId;
    folderSummaryKeyRef.current = summaryKey;
    abortActiveFolderSummary();
    folderSummaryAbortRef.current = controller;
    setIsSummarizingFolder(true);
    setFolderError(null);

    try {
      const { mode, summarizer } = createFolderSummarizerForProvider(
        providerKind === "web"
          ? browserProviderRef.current
          : executableProviderRef.current,
        selectedProviderLoaded,
        { signal: controller.signal },
      );
      const summary = await summarizeFolderManifest(
        manifest,
        summarizer,
      );
      const graph = buildKnowledgeGraph(manifest, summary);

      if (
        !shouldApplyAsyncFolderResult({
          requestId,
          latestRequestId: folderSummaryRequestIdRef.current,
          selectionKey,
          currentSelectionKey: folderSelectionKeyRef.current,
          summaryKey,
          currentSummaryKey: folderSummaryKeyRef.current,
        })
      ) {
        return;
      }

      setFolderSummaryResult(summary);
      setFolderSummaryMode(mode);
      setKnowledgeGraph(graph);
    } catch (error) {
      if (isAbortError(error)) {
        return;
      }

      if (
        shouldApplyAsyncFolderResult({
          requestId,
          latestRequestId: folderSummaryRequestIdRef.current,
          selectionKey,
          currentSelectionKey: folderSelectionKeyRef.current,
          summaryKey,
          currentSummaryKey: folderSummaryKeyRef.current,
        })
      ) {
        setFolderError(error instanceof Error ? error.message : String(error));
      }
    } finally {
      if (folderSummaryAbortRef.current === controller) {
        folderSummaryAbortRef.current = null;
      }

      if (requestId === folderSummaryRequestIdRef.current) {
        setIsSummarizingFolder(false);
      }
    }
  }

  async function checkModel() {
    const checkedPath = modelPath;
    const sourceKey = createModelSourceKey(checkedPath, null);
    const requestId = modelProbeRequestIdRef.current + 1;

    modelProbeRequestIdRef.current = requestId;
    modelLoadRequestIdRef.current += 1;
    modelSourceKeyRef.current = sourceKey;
    invalidateFolderSummary({ clearResult: true });
    disposeBrowserProvider();
    setLocalModelFile(null);
    setModelLoaded(false);
    setLoadError(null);
    setModelProbe({
      state: "checking",
      message: "Checking model path with a HEAD request.",
    });

    const result = await probeModelPath(checkedPath);

    if (
      !shouldApplyAsyncModelResult({
        requestId,
        latestRequestId: modelProbeRequestIdRef.current,
        sourceKey,
        currentSourceKey: modelSourceKeyRef.current,
      })
    ) {
      return;
    }

    setModelProbe({
      state: result.state,
      message: getModelStatusMessage(result.state, result.sizeBytes),
    });
  }

  async function loadModel() {
    if (runtimeAudit.state !== "ready") {
      return;
    }

    const sourceFile = localModelFile;
    const sourcePath = modelPath;
    const sourceKey = createModelSourceKey(sourcePath, sourceFile);
    const requestId = modelLoadRequestIdRef.current + 1;

    modelLoadRequestIdRef.current = requestId;
    modelSourceKeyRef.current = sourceKey;
    invalidateFolderSummary({ clearResult: true });
    setIsLoadingModel(true);
    setModelLoaded(false);
    setLoadError(null);

    const provider = browserProviderRef.current ?? createLiteRtBrowserProvider();
    browserProviderRef.current = provider;

    try {
      const model: ModelSource = sourceFile
        ? { kind: "file", value: sourceFile.stream() }
        : { kind: "url", value: sourcePath };

      await provider.load(
        createBrowserProviderLoadConfig({
          model,
          values: webProviderOptions,
          contextLength,
          systemPrompt: effectiveSystemPrompt,
        }),
      );

      if (
        !shouldApplyAsyncModelResult({
          requestId,
          latestRequestId: modelLoadRequestIdRef.current,
          sourceKey,
          currentSourceKey: modelSourceKeyRef.current,
        })
      ) {
        if (browserProviderRef.current === provider) {
          browserProviderRef.current = null;
          await provider.dispose();
        }
        return;
      }

      invalidateFolderSummary({ clearResult: true });
      setModelLoaded(true);
    } catch (error) {
      if (
        !shouldApplyAsyncModelResult({
          requestId,
          latestRequestId: modelLoadRequestIdRef.current,
          sourceKey,
          currentSourceKey: modelSourceKeyRef.current,
        })
      ) {
        return;
      }

      setLoadError(error instanceof Error ? error.message : String(error));
      setModelLoaded(false);
      if (browserProviderRef.current === provider) {
        browserProviderRef.current = null;
        await provider.dispose();
      }
    } finally {
      if (requestId === modelLoadRequestIdRef.current) {
        setIsLoadingModel(false);
      }
    }
  }

  async function connectExecutableProvider() {
    const endpoint = normalizeExecutableEndpoint(
      executableEndpoint,
      executableEndpointDefault,
    );
    const requestId = executableLoadRequestIdRef.current + 1;
    executableLoadRequestIdRef.current = requestId;
    invalidateFolderSummary({ clearResult: true });
    setIsConnectingExecutable(true);
    setExecutableLoaded(false);
    setLoadError(null);
    setExecutableEndpoint(endpoint);

    try {
      const status = await requestSidecarStatusOverWebSocket(endpoint);

      if (requestId !== executableLoadRequestIdRef.current) {
        return;
      }

      applySidecarStatus(status);

      if (status.state !== "available") {
        disposeExecutableProvider();
        return;
      }

      if (sidecarControlClientRef.current) {
        await refreshSidecarModelCatalog(sidecarControlClientRef.current);
      }

      disposeExecutableProvider();
      const provider = createExecutableProvider({
        transport: sidecarControlClientRef.current ?? undefined,
      });
      executableProviderRef.current = provider;
      const resolvedBackend = resolveExecutableBackend(backend, status.backends);

      if (resolvedBackend !== backend) {
        setBackend(resolvedBackend);
      }

      await provider.load(
        createExecutableProviderLoadConfig({
          endpoint,
          backend: resolvedBackend,
          values: executableProviderOptions,
          contextLength,
          systemPrompt: effectiveSystemPrompt,
        }),
      );

      if (requestId !== executableLoadRequestIdRef.current) {
        if (executableProviderRef.current === provider) {
          executableProviderRef.current = null;
          await provider.dispose();
        }
        return;
      }

      invalidateFolderSummary({ clearResult: true });
      setExecutableLoaded(true);
    } catch (error) {
      if (requestId === executableLoadRequestIdRef.current) {
        setLoadError(error instanceof Error ? error.message : String(error));
        setSidecarStatus({
          state: "unavailable",
          backends: getDefaultBackendStatuses(),
          capabilities: getDefaultSidecarCapabilities(),
          detail: error instanceof Error ? error.message : String(error),
        });
        setSidecarModelCatalog({
          state: "blocked",
          models: [],
          detail: "Model catalog is not reachable.",
        });
        setExecutableLoaded(false);
        setPendingAttachments([]);
        disposeExecutableProvider();
      }
    } finally {
      if (requestId === executableLoadRequestIdRef.current) {
        setIsConnectingExecutable(false);
      }
    }
  }

  async function sendPrompt() {
    const provider =
      providerKind === "web"
        ? browserProviderRef.current
        : executableProviderRef.current;
    const sourceChat = activeChat;
    const sourceChatId = sourceChat.id;
    const userText = sourceChat.prompt.trim();

    if (!provider || !selectedProviderLoaded || !userText) {
      return;
    }

    const outgoingAttachments = nativeAttachmentsEnabled ? pendingAttachments : [];
    const generationProviderKind = providerKind;
    const turn = startAssistantTurn(
      sourceChat.messages,
      userText,
      outgoingAttachments.map((attachment) => attachment.name),
    );
    const controller = new AbortController();
    abortControllerRef.current = controller;
    updateChatMessages(sourceChatId, () => turn.messages);
    setChatSessions((current) =>
      updateChatSessionPrompt(current, sourceChatId, ""),
    );
    setPendingAttachments([]);
    setRuntimeMetrics((current) => resetRuntimeMetrics(current));
    setChatDebugLines([
      `provider=${generationProviderKind}`,
      `chat=${sourceChat.title}`,
      `promptChars=${userText.length}`,
      `attachments=${outgoingAttachments.length}`,
    ]);
    setIsGenerating(true);

    try {
      const result = await provider.generate({
        text: userText,
        attachments: outgoingAttachments,
        signal: controller.signal,
        onToken: (token, fullText) => {
          setRuntimeMetrics((current) =>
            updateRuntimeMetrics(current, token, performance.now()),
          );
          appendChatDebugLine(
            `chunkChars=${token.length} totalChars=${fullText.length}`,
          );
          updateChatMessages(sourceChatId, (current) =>
            appendAssistantText(current, turn.assistantId, token),
          );
        },
      });

      updateChatMessages(sourceChatId, (current) =>
        finishAssistantWithText(current, turn.assistantId, result.text),
      );
      appendChatDebugLine(`completeChars=${result.text.length}`);
    } catch (error) {
      if (isAbortError(error)) {
        updateChatMessages(sourceChatId, (current) =>
          completeAssistantMessage(current, turn.assistantId),
        );
        appendChatDebugLine("aborted");
      } else {
        const errorText = error instanceof Error ? error.message : String(error);
        updateChatMessages(sourceChatId, (current) =>
          failAssistantMessage(
            current,
            turn.assistantId,
            errorText,
          ),
        );
        appendChatDebugLine(`error=${errorText}`);
      }
    } finally {
      if (abortControllerRef.current === controller) {
        abortControllerRef.current = null;
      }
      setIsGenerating(false);
    }
  }

  return (
    <main className="app-shell">
      <div className="workspace">
        <ProviderSetup
          providerKind={providerKind}
          modelPath={modelPath}
          localModelFileName={localModelFile?.name ?? null}
          webGpu={webGpu}
          runtimeAudit={runtimeAudit}
          modelProbe={modelProbe}
          loadError={loadError}
          isLoadingModel={isLoadingModel}
          modelLoaded={selectedProviderLoaded}
          executableEndpoint={executableEndpoint}
          backend={backend}
          backendOptions={backendOptions}
          executableStatus={executableStatus}
          runtimeStatus={sidecarStatus.runtime ?? null}
          sidecarControlConnected={sidecarControlConnected}
          sidecarModelCatalog={sidecarModelCatalog}
          sidecarLogs={sidecarLogs}
          webProviderOptions={webProviderOptions}
          executableProviderOptions={executableProviderOptions}
          suggestedModelUrl={getSuggestedModelUrl()}
          onProviderKindChange={handleProviderKindChange}
          onModelPathChange={handleModelPathChange}
          onLocalModelFileChange={handleLocalModelFileChange}
          onCheckModel={() => void checkModel()}
          onLoadModel={() => void loadModel()}
          onExecutableEndpointChange={handleExecutableEndpointChange}
          onBackendChange={handleBackendChange}
          onConnectExecutable={() => void connectExecutableProvider()}
          onConnectRuntimeControl={handleConnectRuntimeControl}
          onStartRuntime={startRuntime}
          onRestartRuntime={restartRuntime}
          onStopRuntime={stopRuntime}
          onWebProviderOptionChange={handleWebProviderOptionChange}
          onExecutableProviderOptionChange={handleExecutableProviderOptionChange}
        />
        <ChatWorkspace
          sessions={chatSessions}
          activeSessionId={activeChat.id}
          isGenerating={isGenerating}
          modelLoaded={selectedProviderLoaded}
          attachmentsEnabled={nativeAttachmentsEnabled}
          attachments={pendingAttachments}
          attachmentStatus={attachmentStatus}
          systemPrompt={systemPrompt}
          contextLength={contextLength}
          thinkingEnabled={thinkingEnabled}
          tokensPerSecond={runtimeMetrics.tokensPerSecond}
          debugLines={chatDebugLines}
          onActiveSessionChange={setActiveChatId}
          onNewSession={handleNewChat}
          onCloseSession={handleCloseChat}
          onPromptChange={updateActiveChatPrompt}
          onSend={() => void sendPrompt()}
          onStop={stopGeneration}
          onAttachmentsSelected={handleAttachmentsSelected}
          onRemoveAttachment={handleRemoveAttachment}
          onSystemPromptChange={handleSystemPromptChange}
          onContextLengthChange={handleContextLengthChange}
          onThinkingEnabledChange={handleThinkingEnabledChange}
        />
        <aside className="insight-panel" aria-label="Folder insights">
          <FolderPanel
            selectedFileCount={folderSources.length}
            manifest={folderManifest}
            summaryResult={folderSummaryResult}
            summaryMode={displayedFolderSummaryMode}
            hasDirectoryPicker={folderCapabilities.hasShowDirectoryPicker}
            isSelectingDirectory={isSelectingDirectory}
            isIndexing={isIndexingFolder}
            isSummarizing={isSummarizingFolder}
            error={folderError}
            onFilesSelected={handleFolderFilesSelected}
            onChooseDirectory={() => void handleChooseDirectory()}
            onIndexFolder={() => void indexSelectedFolder()}
            onSummarizeFolder={() => void summarizeSelectedFolder()}
          />
          <KnowledgeGraphView graph={knowledgeGraph} />
        </aside>
      </div>
    </main>
  );
}
