package platform

import "testing"

func TestBackendEvidenceForRunningRuntimeEnablesLiteRTBackends(t *testing.T) {
	t.Parallel()

	evidence := BackendEvidenceForRuntime("running")

	assertBackendEvidence(t, evidence, "cpu", "available")
	assertBackendEvidence(t, evidence, "gpu", "available")
	assertBackendEvidence(t, evidence, "npu", "available")
	assertBackendEvidence(t, evidence, "cuda", "not-a-litert-backend")
}

func TestBackendEvidenceForUnavailableRuntimeDisablesLiteRTBackends(t *testing.T) {
	t.Parallel()

	evidence := BackendEvidenceForRuntime("unavailable")

	assertBackendEvidence(t, evidence, "cpu", "unavailable")
	assertBackendEvidence(t, evidence, "gpu", "unavailable")
	assertBackendEvidence(t, evidence, "npu", "unavailable")
	assertBackendEvidence(t, evidence, "cuda", "not-a-litert-backend")
}

func TestBackendEvidenceFromModelIDsUsesAdvertisedServerBackends(t *testing.T) {
	t.Parallel()

	evidence := BackendEvidenceFromModelIDs(
		"gemma4-e2b",
		[]string{"gemma4-e2b", "gemma4-e2b,gpu"},
	)

	assertBackendEvidence(t, evidence, "cpu", "available")
	assertBackendEvidence(t, evidence, "gpu", "available")
	assertBackendEvidence(t, evidence, "npu", "unavailable")
	assertBackendEvidence(t, evidence, "cuda", "not-a-litert-backend")
}

func assertBackendEvidence(
	t *testing.T,
	evidence []BackendEvidence,
	backend string,
	state string,
) {
	t.Helper()

	for _, item := range evidence {
		if item.Backend == backend {
			if item.State != state {
				t.Fatalf("%s state = %q, want %q", backend, item.State, state)
			}
			return
		}
	}

	t.Fatalf("backend %q not found in %#v", backend, evidence)
}
