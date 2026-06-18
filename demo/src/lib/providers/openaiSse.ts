type OpenAiSseTokenCallback = (token: string) => void;

type OpenAiSseChoice = {
  delta?: {
    content?: unknown;
  };
};

type OpenAiSseEvent = {
  choices?: OpenAiSseChoice[];
};

function parseContentTokens(payload: string): string[] {
  let event: OpenAiSseEvent;

  try {
    event = JSON.parse(payload) as OpenAiSseEvent;
  } catch {
    return [];
  }

  if (!Array.isArray(event.choices)) {
    return [];
  }

  return event.choices
    .map((choice) => choice.delta?.content)
    .filter((content): content is string => typeof content === "string");
}

export async function collectOpenAiSseText(
  stream: ReadableStream<Uint8Array>,
  onToken: OpenAiSseTokenCallback,
): Promise<string> {
  const reader = stream.getReader();
  const decoder = new TextDecoder();
  let pending = "";
  let fullText = "";
  let done = false;

  const processLine = (rawLine: string) => {
    const line = rawLine.endsWith("\r") ? rawLine.slice(0, -1) : rawLine;

    if (!line.startsWith("data:")) {
      return;
    }

    const payload = line.slice("data:".length).trim();

    if (!payload) {
      return;
    }

    if (payload === "[DONE]") {
      done = true;
      return;
    }

    for (const token of parseContentTokens(payload)) {
      fullText += token;
      onToken(token);
    }
  };

  try {
    while (!done) {
      const result = await reader.read();

      if (result.done) {
        break;
      }

      pending += decoder.decode(result.value, { stream: true });
      const lines = pending.split("\n");
      pending = lines.pop() ?? "";

      for (const line of lines) {
        processLine(line);

        if (done) {
          break;
        }
      }
    }

    pending += decoder.decode();

    if (!done && pending) {
      processLine(pending);
    }
  } finally {
    reader.releaseLock();
  }

  return fullText;
}
