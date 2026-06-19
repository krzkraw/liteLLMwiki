const defaultExecutableEndpoint = "http://127.0.0.1:9379/v1";
const defaultSidecarPort = "9379";
const defaultRuntimePort = "9381";

export function trimTrailingSlash(value: string): string {
  return value.replace(/\/+$/, "");
}

function truncatePathAtV1(pathname: string): string {
  const marker = "/v1";
  const index = pathname.indexOf(marker);

  if (index >= 0) {
    return pathname.slice(0, index + marker.length);
  }

  return marker;
}

function isDefaultLocalRuntimeUrl(url: URL): boolean {
  return (
    url.port === defaultRuntimePort &&
    (url.hostname === "127.0.0.1" ||
      url.hostname === "localhost" ||
      url.hostname === "[::1]")
  );
}

export function normalizeExecutableEndpoint(
  endpoint: string,
  fallback = defaultExecutableEndpoint,
): string {
  const raw = endpoint.trim() || fallback;
  const normalized = trimTrailingSlash(raw);

  try {
    const url = new URL(normalized);
    if (isDefaultLocalRuntimeUrl(url)) {
      url.port = defaultSidecarPort;
    }

    url.pathname = truncatePathAtV1(url.pathname);
    url.search = "";
    url.hash = "";

    return trimTrailingSlash(url.toString());
  } catch {
    const v1Path = truncatePathAtV1(normalized);

    return trimTrailingSlash(v1Path);
  }
}

export function createSidecarEndpoint(endpoint: string, path: string): string {
  const normalized = normalizeExecutableEndpoint(endpoint);

  try {
    const url = new URL(normalized);

    return `${url.origin}${path}`;
  } catch {
    if (normalized.endsWith("/v1")) {
      return `${normalized.slice(0, -"/v1".length)}${path}`;
    }

    return `${normalized}${path}`;
  }
}
