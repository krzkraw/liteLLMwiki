import { describe, expect, it } from "vitest";
import {
  createSidecarEndpoint,
  normalizeExecutableEndpoint,
} from "./endpoint";

describe("normalizeExecutableEndpoint", () => {
  it("keeps the default sidecar OpenAI endpoint stable", () => {
    expect(normalizeExecutableEndpoint("http://127.0.0.1:9379/v1")).toBe(
      "http://127.0.0.1:9379/v1",
    );
  });

  it("trims pasted OpenAI resource URLs back to the v1 base", () => {
    expect(
      normalizeExecutableEndpoint(" http://127.0.0.1:9379/v1/models "),
    ).toBe("http://127.0.0.1:9379/v1");
  });

  it("maps the default LiteRT runtime port to the sidecar port", () => {
    expect(normalizeExecutableEndpoint("http://127.0.0.1:9381/v1/models")).toBe(
      "http://127.0.0.1:9379/v1",
    );
    expect(normalizeExecutableEndpoint("http://localhost:9381/v1")).toBe(
      "http://localhost:9379/v1",
    );
  });

  it("creates sidecar-only endpoints from normalized OpenAI endpoints", () => {
    expect(
      createSidecarEndpoint(
        "http://127.0.0.1:9381/v1/models",
        "/sidecar/v1/status",
      ),
    ).toBe("http://127.0.0.1:9379/sidecar/v1/status");
  });
});
