package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"g0litellama/internal/proxy"
)

func TestMultimodalEndpointWritesAttachmentsAndCallsRunner(t *testing.T) {
	t.Parallel()

	var captured MultimodalRunRequest
	handler := newMultimodalTestHandler(t, func(
		_ context.Context,
		request MultimodalRunRequest,
	) (MultimodalRunResponse, error) {
		captured = request

		if len(request.AttachmentPaths) != 1 {
			t.Fatalf("attachment paths = %#v, want one", request.AttachmentPaths)
		}

		content, err := os.ReadFile(request.AttachmentPaths[0])
		if err != nil {
			t.Fatalf("read attachment: %v", err)
		}
		if string(content) != "image bytes" {
			t.Fatalf("attachment content = %q", string(content))
		}

		return MultimodalRunResponse{Text: "image summary"}, nil
	})

	body := `{
			"prompt": "Describe the image",
			"modelId": "gemma4-e2b",
			"backend": "gpu",
			"visionBackend": "gpu",
			"audioBackend": "cpu",
			"maxNumTokens": 4096,
			"topK": 8,
			"topP": 0.75,
			"temperature": 0.2,
			"seed": 123,
			"preset": "creative.json",
			"noTemplate": true,
			"filterChannelContentFromKvCache": true,
			"enableSpeculativeDecoding": "false",
			"cache": "memory",
			"verbose": true,
			"fromHuggingFaceRepo": "google/gemma",
			"huggingfaceToken": "hf_secret",
			"attachments": [
			{
				"name": "sample.png",
				"mimeType": "image/png",
				"dataBase64": "` + base64.StdEncoding.EncodeToString([]byte("image bytes")) + `"
			}
		]
	}`
	req := httptest.NewRequest(
		http.MethodPost,
		"/g0litellama/v1/multimodal",
		strings.NewReader(body),
	)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if captured.Prompt != "Describe the image" {
		t.Fatalf("prompt = %q", captured.Prompt)
	}
	if captured.ModelID != "gemma4-e2b" {
		t.Fatalf("model id = %q", captured.ModelID)
	}
	if captured.Backend != "gpu" {
		t.Fatalf("backend = %q", captured.Backend)
	}
	if captured.VisionBackend != "gpu" {
		t.Fatalf("vision backend = %q", captured.VisionBackend)
	}
	if captured.AudioBackend != "cpu" {
		t.Fatalf("audio backend = %q", captured.AudioBackend)
	}
	if captured.MaxNumTokens != 4096 {
		t.Fatalf("max num tokens = %d", captured.MaxNumTokens)
	}
	if captured.TopK != 8 {
		t.Fatalf("top k = %d", captured.TopK)
	}
	if captured.TopP == nil || *captured.TopP != 0.75 {
		t.Fatalf("top p = %v", captured.TopP)
	}
	if captured.Temperature == nil || *captured.Temperature != 0.2 {
		t.Fatalf("temperature = %v", captured.Temperature)
	}
	if captured.Seed != 123 {
		t.Fatalf("seed = %d", captured.Seed)
	}
	if captured.Preset != "creative.json" {
		t.Fatalf("preset = %q", captured.Preset)
	}
	if !captured.NoTemplate {
		t.Fatal("no template = false, want true")
	}
	if !captured.FilterChannelContentFromKVCache {
		t.Fatal("filter channel content from kv cache = false, want true")
	}
	if captured.EnableSpeculativeDecoding != "false" {
		t.Fatalf("enable speculative decoding = %q", captured.EnableSpeculativeDecoding)
	}
	if captured.Cache != "memory" {
		t.Fatalf("cache = %q", captured.Cache)
	}
	if !captured.Verbose {
		t.Fatal("verbose = false, want true")
	}
	if captured.FromHuggingFaceRepo != "google/gemma" {
		t.Fatalf("from huggingface repo = %q", captured.FromHuggingFaceRepo)
	}
	if captured.HuggingFaceToken != "hf_secret" {
		t.Fatalf("hugging face token = %q", captured.HuggingFaceToken)
	}

	var response MultimodalRunResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Text != "image summary" {
		t.Fatalf("text = %q", response.Text)
	}
}

func TestMultimodalEndpointRejectsMalformedAttachment(t *testing.T) {
	t.Parallel()

	handler := newMultimodalTestHandler(t, func(
		context.Context,
		MultimodalRunRequest,
	) (MultimodalRunResponse, error) {
		t.Fatal("runner should not be called")
		return MultimodalRunResponse{}, nil
	})

	body := `{
		"prompt": "Describe the image",
		"attachments": [
			{
				"name": "sample.png",
				"mimeType": "image/png",
				"dataBase64": "%%%not-base64%%%"
			}
		]
	}`
	req := httptest.NewRequest(
		http.MethodPost,
		"/g0litellama/v1/multimodal",
		strings.NewReader(body),
	)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func newMultimodalTestHandler(
	t *testing.T,
	runner MultimodalRunner,
) http.Handler {
	t.Helper()

	upstreamProxy, err := proxy.New("http://127.0.0.1:9381")
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}

	return New(Options{
		Proxy:            upstreamProxy,
		MultimodalRunner: runner,
	}).Handler()
}
