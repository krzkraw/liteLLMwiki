import { describe, expect, it } from "bun:test";
import {
  getProviderOptionDefinition,
  getProviderOptionMetadata,
  providerOptionMetadata,
} from "./providerOptionMetadata";

describe("providerOptionMetadata", () => {
  it("exposes the required web option groups with sourced defaults", () => {
    const ids = getProviderOptionMetadata("web").map((option) => option.id);

    expect(ids).toEqual(
      expect.arrayContaining([
        "wasmPath",
        "engineBackend",
        "maxNumTokens",
        "maxOutputTokens",
        "samplerBackend",
        "samplerType",
        "topK",
        "topP",
        "temperature",
        "seed",
        "stopTokenIds",
        "startTokenId",
        "numOutputCandidates",
        "systemPrompt",
        "enableConstrainedDecoding",
        "prefillPrefaceOnInit",
        "filterChannelContentFromKvCache",
        "applyPromptTemplateInSession",
        "useExternalSampler",
        "visionModalityEnabled",
        "audioModalityEnabled",
      ]),
    );

    expect(getProviderOptionDefinition("web", "maxNumTokens")).toMatchObject({
      provider: "web",
      group: "Engine",
      type: "number",
      defaultValue: 8192,
      min: 256,
      requiresReload: true,
    });
    expect(getProviderOptionDefinition("web", "samplerType")?.choices).toEqual([
      { value: "TYPE_UNSPECIFIED", label: "Unspecified" },
      { value: "TOP_K", label: "Top K" },
      { value: "TOP_P", label: "Top P" },
      { value: "GREEDY", label: "Greedy" },
    ]);
    expect(getProviderOptionDefinition("web", "samplerBackend")).toMatchObject({
      provider: "web",
      group: "Sampling",
      type: "select",
      defaultValue: "UNSPECIFIED",
      source: "node_modules/@litert-lm/core/dist/session_config.d.ts",
      choices: [
        { value: "UNSPECIFIED", label: "Unspecified" },
        { value: "CPU", label: "CPU" },
        { value: "GPU", label: "GPU" },
        { value: "NPU", label: "NPU" },
      ],
    });
    expect(getProviderOptionDefinition("web", "stopTokenIds")).toMatchObject({
      provider: "web",
      group: "Sampling",
      type: "text",
      defaultValue: "",
      source: "node_modules/@litert-lm/core/dist/session_config.d.ts",
    });
    expect(getProviderOptionDefinition("web", "startTokenId")).toMatchObject({
      provider: "web",
      group: "Sampling",
      type: "number",
      defaultValue: 0,
      min: 0,
      step: 1,
      source: "node_modules/@litert-lm/core/dist/session_config.d.ts",
    });
    expect(
      getProviderOptionDefinition("web", "numOutputCandidates"),
    ).toMatchObject({
      provider: "web",
      group: "Sampling",
      type: "number",
      defaultValue: 0,
      min: 0,
      step: 1,
      source: "node_modules/@litert-lm/core/dist/session_config.d.ts",
    });
    expect(
      getProviderOptionDefinition("web", "visionModalityEnabled"),
    ).toMatchObject({
      defaultValue: false,
      locked: true,
    });
  });

  it("exposes executable connection, sidecar, and run options", () => {
    const ids = getProviderOptionMetadata("executable").map((option) => option.id);

    expect(ids).toEqual(
      expect.arrayContaining([
        "endpoint",
        "modelId",
	        "backend",
	        "maxNumTokens",
	        "maxTokens",
	        "stream",
        "addr",
        "upstream",
        "launchRuntime",
        "runtimeExe",
        "runtimeHost",
        "runtimePort",
        "modelFile",
        "importModel",
        "runtimeVerbose",
        "preset",
        "noTemplate",
        "filterChannelContentFromKvCache",
        "visionBackend",
        "audioBackend",
        "topK",
        "topP",
        "temperature",
        "seed",
        "enableSpeculativeDecoding",
        "cache",
        "verbose",
        "fromHuggingFaceRepo",
        "huggingfaceToken",
      ]),
    );

    expect(getProviderOptionDefinition("executable", "stream")).toMatchObject({
      type: "boolean",
      defaultValue: true,
      locked: true,
    });
    expect(getProviderOptionDefinition("executable", "runtimePort")).toMatchObject({
      type: "number",
      defaultValue: 9381,
      min: 1,
      max: 65535,
    });
    expect(
      getProviderOptionDefinition("executable", "enableSpeculativeDecoding"),
    ).toMatchObject({
      type: "select",
      defaultValue: "auto",
      choices: [
        { value: "auto", label: "Auto" },
        { value: "true", label: "True" },
        { value: "false", label: "False" },
      ],
    });
    expect(getProviderOptionDefinition("executable", "cache")).toMatchObject({
      type: "select",
      defaultValue: "disk",
      choices: [
        { value: "disk", label: "Disk" },
        { value: "memory", label: "Memory" },
        { value: "no", label: "No cache" },
      ],
    });
    expect(
      getProviderOptionDefinition("executable", "huggingfaceToken"),
    ).toMatchObject({
      type: "secret",
      defaultValue: "",
      secret: true,
    });
    expect(
      getProviderOptionDefinition("executable", "huggingfaceToken")?.tooltip,
    ).toContain("local sidecar");
  });

  it("keeps every option displayable with a tooltip and source", () => {
    for (const option of providerOptionMetadata) {
      expect(option.label).not.toBe("");
      expect(option.tooltip).not.toBe("");
      expect(option.source).not.toBe("");
      expect(option.defaultValue).not.toBeUndefined();
    }
  });
});
