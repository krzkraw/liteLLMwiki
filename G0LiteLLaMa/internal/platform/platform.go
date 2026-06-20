package platform

const (
	DefaultListenAddr  = "127.0.0.1:9379"
	DefaultUpstreamURL = "http://127.0.0.1:9381"
)

type BackendEvidence struct {
	Backend string
	State   string
	Detail  string
}

func BackendEvidenceList() []BackendEvidence {
	return BackendEvidenceForRuntime("")
}

func BackendEvidenceForRuntime(runtimeState string) []BackendEvidence {
	litertBackendState := "unknown"
	litertBackendDetail := "LiteRT backend probing is pending."

	switch runtimeState {
	case "running", "external":
		litertBackendState = "available"
		litertBackendDetail = "LiteRT-LM can select this backend via the request model field."
	case "unavailable", "exited", "stopped":
		litertBackendState = "unavailable"
		litertBackendDetail = "LiteRT-LM runtime is not available."
	}

	return []BackendEvidence{
		{Backend: "cpu", State: litertBackendState, Detail: litertBackendDetail},
		{Backend: "gpu", State: litertBackendState, Detail: litertBackendDetail},
		{Backend: "npu", State: litertBackendState, Detail: litertBackendDetail},
		{Backend: "cuda", State: "not-a-litert-backend", Detail: "CUDA is probe-only for this G0LiteLLaMa."},
	}
}

func BackendEvidenceFromModelIDs(modelID string, modelIDs []string) []BackendEvidence {
	available := make(map[string]struct{}, len(modelIDs))
	for _, id := range modelIDs {
		available[id] = struct{}{}
	}

	cpuState := "unavailable"
	cpuDetail := "LiteRT-LM server did not advertise the base model."
	if _, ok := available[modelID]; ok {
		cpuState = "available"
		cpuDetail = "LiteRT-LM server advertised the base model."
	}

	return []BackendEvidence{
		{Backend: "cpu", State: cpuState, Detail: cpuDetail},
		backendEvidenceFromSuffix(modelID, available, "gpu"),
		backendEvidenceFromSuffix(modelID, available, "npu"),
		{Backend: "cuda", State: "not-a-litert-backend", Detail: "CUDA is probe-only for this G0LiteLLaMa."},
	}
}

func backendEvidenceFromSuffix(
	modelID string,
	available map[string]struct{},
	backend string,
) BackendEvidence {
	modelBackendID := modelID + "," + backend
	if _, ok := available[modelBackendID]; ok {
		return BackendEvidence{
			Backend: backend,
			State:   "available",
			Detail:  "LiteRT-LM server advertised " + modelBackendID + ".",
		}
	}

	return BackendEvidence{
		Backend: backend,
		State:   "unavailable",
		Detail:  "LiteRT-LM server did not advertise " + modelBackendID + ".",
	}
}
