package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"litert-sidecar/internal/proxy"
	"litert-sidecar/internal/server"
	"litert-sidecar/internal/supervisor"
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

func TestSupervisorRuntimeControllerMapsLegacyExternalConfig(t *testing.T) {
	t.Parallel()

	launch := false
	runtimeSupervisor := supervisor.New(supervisor.Config{
		DefaultLiteRT: supervisor.LiteRTConfig{
			Launch: true,
			Host:   "127.0.0.1",
			Port:   9381,
		},
	})
	controller := supervisorRuntimeController{
		supervisor: runtimeSupervisor,
	}

	if err := controller.Start(context.Background(), server.RuntimeModeRelease, server.RuntimeControlConfig{
		LaunchRuntime: &launch,
		Upstream:      "http://127.0.0.1:9999",
		RuntimeHost:   "127.0.0.1",
		RuntimePort:   9481,
	}); err != nil {
		t.Fatalf("start runtime: %v", err)
	}

	status := controller.Status()
	if status.State != "external" {
		t.Fatalf("state = %q, want external", status.State)
	}
	if status.Upstream != "http://127.0.0.1:9999" {
		t.Fatalf("upstream = %q, want explicit upstream", status.Upstream)
	}
}

func TestProxyTargetResolverUsesSupervisorRoutes(t *testing.T) {
	t.Parallel()

	upstreamProxy, err := proxy.New("http://127.0.0.1:9381")
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}
	runtimeSupervisor := supervisor.New(supervisor.Config{
		DefaultLiteRT: supervisor.LiteRTConfig{
			Launch: false,
			Host:   "127.0.0.1",
			Port:   9481,
		},
	})
	upstreamProxy.SetTargetResolver(proxyTargetResolver(runtimeSupervisor))

	if got := upstreamProxy.TargetForPath("/v1/chat/completions"); got != "http://127.0.0.1:9481" {
		t.Fatalf("proxy target = %q, want initial runtime port", got)
	}
}

func TestToLiteRTConfigPatchForwardsHuggingFaceToken(t *testing.T) {
	t.Parallel()

	huggingFaceToken := "hf_secret"
	got := toSupervisorLiteRTPatch(server.RuntimeControlConfig{
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

	status := supervisor.RuntimeStatus{
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
