package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"litert-sidecar/internal/litert"
	"litert-sidecar/internal/proxy"
	"litert-sidecar/internal/server"
)

func TestModelsEndpointHandlesBaseAndV1Upstreams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		upstream string
		want     string
	}{
		{
			name:     "base upstream",
			upstream: "http://127.0.0.1:9381",
			want:     "http://127.0.0.1:9381/v1/models",
		},
		{
			name:     "v1 upstream",
			upstream: "http://127.0.0.1:9381/v1",
			want:     "http://127.0.0.1:9381/v1/models",
		},
		{
			name:     "v1 upstream with trailing slash",
			upstream: "http://127.0.0.1:9381/v1/",
			want:     "http://127.0.0.1:9381/v1/models",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := modelsEndpoint(tt.upstream)
			if err != nil {
				t.Fatalf("modelsEndpoint() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("modelsEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRuntimeControllerRetargetsProxyToManagedRuntimePort(t *testing.T) {
	t.Parallel()

	upstreamProxy, err := proxy.New("http://127.0.0.1:9381")
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}
	manager := litert.NewManager(litert.Config{
		Launch: false,
		Host:   "127.0.0.1",
		Port:   9381,
	})
	controller := runtimeControllerAdapter{
		manager: manager,
		proxy:   upstreamProxy,
	}
	launch := false

	if err := controller.Start(context.Background(), server.RuntimeModeRelease, server.RuntimeControlConfig{
		LaunchRuntime: &launch,
		Upstream:      "http://127.0.0.1:9999",
		RuntimeHost:   "127.0.0.1",
		RuntimePort:   9481,
	}); err != nil {
		t.Fatalf("start runtime: %v", err)
	}

	if got := upstreamProxy.Target(); got != "http://127.0.0.1:9999" {
		t.Fatalf("external proxy target = %q, want explicit upstream", got)
	}
	if got := manager.Status().Upstream; got != "http://127.0.0.1:9999" {
		t.Fatalf("external manager upstream = %q, want explicit upstream", got)
	}

	launch = true
	if err := manager.ApplyConfigPatch(litert.ConfigPatch{
		Launch: &launch,
		Host:   "127.0.0.1",
		Port:   9481,
	}); err != nil {
		t.Fatalf("apply config patch: %v", err)
	}
	if err := controller.retargetProxy(server.RuntimeControlConfig{
		LaunchRuntime: &launch,
		Upstream:      "http://127.0.0.1:9999",
	}); err != nil {
		t.Fatalf("retarget proxy: %v", err)
	}

	if got := upstreamProxy.Target(); got != "http://127.0.0.1:9481" {
		t.Fatalf("managed proxy target = %q, want manager runtime port", got)
	}
}

func TestRetargetProxyToRuntimeManagerUsesInitialRuntimePort(t *testing.T) {
	t.Parallel()

	upstreamProxy, err := proxy.New("http://127.0.0.1:9381")
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}
	manager := litert.NewManager(litert.Config{
		Launch: true,
		Host:   "127.0.0.1",
		Port:   9481,
	})

	if err := retargetProxyToRuntimeManager(upstreamProxy, manager); err != nil {
		t.Fatalf("retarget initial proxy: %v", err)
	}

	if got := upstreamProxy.Target(); got != "http://127.0.0.1:9481" {
		t.Fatalf("proxy target = %q, want initial runtime port", got)
	}
}

func TestToLiteRTConfigPatchForwardsHuggingFaceToken(t *testing.T) {
	t.Parallel()

	huggingFaceToken := "hf_secret"
	got := toLiteRTConfigPatch(server.RuntimeControlConfig{
		HuggingFaceToken: &huggingFaceToken,
	})
	if got.HuggingFaceToken == nil {
		t.Fatal("hugging face token patch is nil")
	}
	if *got.HuggingFaceToken != "hf_secret" {
		t.Fatalf("hugging face token = %q", *got.HuggingFaceToken)
	}
}

func TestBackendReporterUsesCurrentManagerStatus(t *testing.T) {
	t.Parallel()

	modelsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"runtime-model,gpu"}]}`))
	}))
	defer modelsServer.Close()

	status := litert.RuntimeStatus{
		State:    "running",
		ModelID:  "runtime-model",
		Upstream: modelsServer.URL,
	}
	backends, err := reportBackends(context.Background(), status.Upstream, status.ModelID)
	if err != nil {
		t.Fatalf("report backends: %v", err)
	}

	foundGPU := false
	for _, backend := range backends {
		if backend.Backend == "gpu" && backend.State == "available" {
			foundGPU = true
		}
	}
	if !foundGPU {
		t.Fatalf("backend evidence = %#v, want gpu available from current model id", backends)
	}
}
