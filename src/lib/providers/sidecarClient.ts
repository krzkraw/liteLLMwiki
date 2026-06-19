import type { BackendStatus } from "./backendStatus";
import { getDefaultBackendStatuses } from "./backendStatus";
import {
  createSidecarEndpoint,
  normalizeExecutableEndpoint,
} from "./endpoint";

type FetchImpl = typeof fetch;

export interface SidecarStatus {
  state: "available" | "unavailable" | "unknown";
  backends: BackendStatus[];
  capabilities: SidecarCapabilities;
  runtime?: SidecarRuntimeStatus;
  detail?: string;
}

export interface SidecarCapabilities {
  multimodal: MultimodalCapability;
}

export interface SidecarRuntimeStatus {
  state: string;
  executable?: string;
  version?: string;
  modelId?: string;
  modelFile?: string;
  upstream?: string;
  mode?: string;
  logSequence?: number;
  detail?: string;
}

export interface MultimodalCapability {
  state: "available" | "unavailable" | "unknown";
  endpoint?: string;
  detail?: string;
  imageBackends?: string[];
  audioBackends?: string[];
}

type SidecarStatusResponse = Partial<{
  status: string;
  state: string;
  detail: string;
  message: string;
  backends: BackendStatus[] | Record<string, BackendStatus["state"]>;
  runtime: SidecarRuntimeStatus;
  capabilities: Partial<{
    multimodal: Partial<MultimodalCapability>;
  }>;
}>;

function getStatusUrls(baseUrl: string): string[] {
  const normalized = normalizeExecutableEndpoint(baseUrl);
  const urls = [
    createSidecarEndpoint(normalized, "/sidecar/v1/status"),
    `${normalized}/sidecar/v1/status`,
    `${normalized}/status`,
  ];

  try {
    const parsed = new URL(normalized);
    urls.unshift(`${parsed.origin}/sidecar/v1/status`);
  } catch {
    // Relative URLs are still useful for tests and future UI wiring.
  }

  return [...new Set(urls)];
}

function normalizeBackends(
  backends: SidecarStatusResponse["backends"],
): BackendStatus[] {
  if (Array.isArray(backends)) {
    return backends;
  }

  if (backends && typeof backends === "object") {
    return Object.entries(backends).map(([backend, state]) => ({
      backend: backend as BackendStatus["backend"],
      state,
    }));
  }

  return getDefaultBackendStatuses();
}

function normalizeSidecarState(
  body: SidecarStatusResponse,
): SidecarStatus["state"] {
  const state = body.state ?? body.status;

  if (state === "available" || state === "unavailable") {
    return state;
  }

  return "unknown";
}

export function getDefaultSidecarCapabilities(): SidecarCapabilities {
  return {
    multimodal: {
      state: "unavailable",
      detail: "Native multimodal capability has not been advertised.",
    },
  };
}

function normalizeMultimodalState(value: unknown): MultimodalCapability["state"] {
  return value === "available" || value === "unavailable" || value === "unknown"
    ? value
    : "unknown";
}

function normalizeCapabilities(
  capabilities: SidecarStatusResponse["capabilities"],
): SidecarCapabilities {
  const multimodal = capabilities?.multimodal;

  if (!multimodal) {
    return getDefaultSidecarCapabilities();
  }

  return {
    multimodal: {
      state: normalizeMultimodalState(multimodal.state),
      endpoint: multimodal.endpoint,
      detail: multimodal.detail,
      imageBackends: multimodal.imageBackends,
      audioBackends: multimodal.audioBackends,
    },
  };
}

export async function getSidecarStatus(
  baseUrl: string,
  fetchImpl: FetchImpl = fetch,
): Promise<SidecarStatus> {
  for (const url of getStatusUrls(baseUrl)) {
    try {
      const response = await fetchImpl(url);

      if (!response.ok) {
        continue;
      }

      const body = (await response.json()) as SidecarStatusResponse;

      return {
        state: normalizeSidecarState(body),
        backends: normalizeBackends(body.backends),
        capabilities: normalizeCapabilities(body.capabilities),
        runtime: body.runtime,
        detail: body.detail ?? body.message,
      };
    } catch {
      // Try the next pragmatic status URL before reporting unavailable.
    }
  }

  return {
    state: "unavailable",
    backends: getDefaultBackendStatuses(),
    capabilities: getDefaultSidecarCapabilities(),
    detail: "Sidecar status endpoint is not reachable.",
  };
}
