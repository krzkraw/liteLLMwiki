import { describe, expect, it } from "bun:test";
import { getProviderOptionDefinition } from "./providerOptionMetadata";
import {
  createDefaultProviderOptionValues,
  getChangedProviderOptionIds,
  resetProviderOptionValue,
  setProviderOptionValue,
} from "./providerOptionState";

describe("providerOptionState", () => {
  it("creates provider defaults from metadata", () => {
    expect(createDefaultProviderOptionValues("web")).toMatchObject({
      wasmPath: "/vendor/litert-lm/core/wasm",
      maxNumTokens: 8192,
      maxOutputTokens: 1024,
      temperature: 0.7,
      visionModalityEnabled: false,
    });

    expect(createDefaultProviderOptionValues("executable")).toMatchObject({
      endpoint: "http://127.0.0.1:9379/v1",
      modelId: "gemma4-e2b",
      backend: "auto",
      stream: true,
      addr: "127.0.0.1:9379",
      runtimePort: 9381,
      huggingfaceToken: "",
    });
  });

  it("coerces changed option values and reports changed ids", () => {
    const temperature = getProviderOptionDefinition("web", "temperature");

    if (!temperature) {
      throw new Error("Missing temperature option.");
    }

    const defaults = createDefaultProviderOptionValues("web");
    const changed = setProviderOptionValue(defaults, temperature, "0.25");

    expect(changed.temperature).toBe(0.25);
    expect(defaults.temperature).toBe(0.7);
    expect(getChangedProviderOptionIds("web", changed)).toEqual(["temperature"]);
  });

  it("resets changed options to their defaults", () => {
    const seed = getProviderOptionDefinition("web", "seed");

    if (!seed) {
      throw new Error("Missing seed option.");
    }

    const changed = setProviderOptionValue(
      createDefaultProviderOptionValues("web"),
      seed,
      "123",
    );
    const reset = resetProviderOptionValue(changed, seed);

    expect(changed.seed).toBe(123);
    expect(reset.seed).toBe(0);
    expect(getChangedProviderOptionIds("web", reset)).toEqual([]);
  });
});
