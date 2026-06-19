export type ChatRole = "system" | "user" | "assistant";
export type ProviderKind = "browser" | "executable";
export type MessageStatus = "complete" | "streaming" | "error";

export interface ChatMessage {
  id: string;
  role: ChatRole;
  text: string;
  status?: MessageStatus;
  attachmentNames?: string[];
}

export interface ModelSource {
  kind: "url" | "file";
  value: string | ReadableStream<Uint8Array>;
}

export interface BrowserProviderConfig {
  model: ModelSource;
  wasmPath: string;
  maxNumTokens: number;
  maxOutputTokens: number;
  engineBackend?: string;
  samplerBackend?: string;
  samplerType?: string;
  temperature?: number;
  topK?: number;
  topP?: number;
  seed?: number;
  stopTokenIds?: number[][];
  startTokenId?: number;
  numOutputCandidates?: number;
  systemPrompt?: string;
  applyPromptTemplateInSession?: boolean;
  useExternalSampler?: boolean;
  enableConstrainedDecoding?: boolean;
  prefillPrefaceOnInit?: boolean;
  filterChannelContentFromKvCache?: boolean;
}

export interface ChatGenerateRequest {
  text: string;
  attachments?: ChatAttachment[];
  onToken: (token: string, fullText: string) => void;
  signal?: AbortSignal;
}

export interface ChatGenerateResult {
  text: string;
}

export interface ChatProvider {
  readonly id: string;
  load(config: BrowserProviderConfig): Promise<void>;
  generate(request: ChatGenerateRequest): Promise<ChatGenerateResult>;
  generateText(prompt: string, signal?: AbortSignal): Promise<string>;
  cancel(): void;
  dispose(): Promise<void>;
}

export interface ChatAttachment {
  id: string;
  name: string;
  mimeType: string;
  file: File;
}
