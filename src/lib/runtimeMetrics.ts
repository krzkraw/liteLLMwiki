export interface RuntimeMetrics {
  startedAtMs: number | null;
  updatedAtMs: number | null;
  tokenCount: number;
  tokensPerSecond: number | null;
}

export function createInitialRuntimeMetrics(): RuntimeMetrics {
  return {
    startedAtMs: null,
    updatedAtMs: null,
    tokenCount: 0,
    tokensPerSecond: null,
  };
}

export function resetRuntimeMetrics(_metrics: RuntimeMetrics): RuntimeMetrics {
  return createInitialRuntimeMetrics();
}

export function estimateTokenCount(text: string): number {
  return text.match(/[\p{L}\p{N}]+|[^\s\p{L}\p{N}]+/gu)?.length ?? 0;
}

export function updateRuntimeMetrics(
  metrics: RuntimeMetrics,
  tokenText: string,
  nowMs: number,
): RuntimeMetrics {
  const startedAtMs = metrics.startedAtMs ?? nowMs;
  const tokenCount = metrics.tokenCount + estimateTokenCount(tokenText);
  const elapsedSeconds = Math.max((nowMs - startedAtMs) / 1000, 1);

  return {
    startedAtMs,
    updatedAtMs: nowMs,
    tokenCount,
    tokensPerSecond: tokenCount / elapsedSeconds,
  };
}
