import { describe, expect, it, mock } from "bun:test";
import { getSidecarStatus } from "./sidecarClient";

describe("getSidecarStatus", () => {
  it("probes origin sidecar status before endpoint-local fallbacks", async () => {
    const fetchImpl = mock().mockResolvedValue({
      ok: true,
      json: async () => ({
        state: "available",
        backends: {
          cpu: "available",
          gpu: "unavailable",
          npu: "unknown",
          cuda: "not-a-litert-backend",
        },
        capabilities: {
          multimodal: {
            state: "available",
            endpoint: "/sidecar/v1/multimodal",
            detail: "Native attachments ready.",
            imageBackends: ["cpu", "gpu"],
            audioBackends: ["cpu"],
          },
        },
      }),
    });

    const status = await getSidecarStatus("http://127.0.0.1:9379/v1", fetchImpl);

    expect(fetchImpl).toHaveBeenCalledWith(
      "http://127.0.0.1:9379/sidecar/v1/status",
    );
    expect(status.state).toBe("available");
    expect(status.backends).toContainEqual({
      backend: "cuda",
      state: "not-a-litert-backend",
    });
    expect(status.capabilities.multimodal).toEqual({
      state: "available",
      endpoint: "/sidecar/v1/multimodal",
      detail: "Native attachments ready.",
      imageBackends: ["cpu", "gpu"],
      audioBackends: ["cpu"],
    });
  });

  it("returns default unavailable state when no status endpoint responds", async () => {
    const fetchImpl = mock().mockRejectedValue(new Error("offline"));

    const status = await getSidecarStatus("http://127.0.0.1:9379/v1", fetchImpl);

    expect(status.state).toBe("unavailable");
    expect(status.backends).toContainEqual({
      backend: "cuda",
      state: "not-a-litert-backend",
    });
    expect(status.capabilities.multimodal.state).toBe("unavailable");
  });

  it("preserves explicit unavailable sidecar status responses", async () => {
    const fetchImpl = mock().mockResolvedValue({
      ok: true,
      json: async () => ({
        state: "unavailable",
        message: "Sidecar is starting.",
      }),
    });

    const status = await getSidecarStatus("http://127.0.0.1:9379/v1", fetchImpl);

    expect(status).toMatchObject({
      state: "unavailable",
      detail: "Sidecar is starting.",
    });
  });
});
