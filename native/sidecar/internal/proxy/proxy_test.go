package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyForwardsV1Requests(t *testing.T) {
	t.Parallel()

	var sawPath string
	var sawBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}

		sawPath = r.URL.Path
		sawBody = string(body)
		w.Header().Set("content-type", "text/event-stream")
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	proxy, err := New(upstream.URL)
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/completions",
		strings.NewReader(`{"stream":true}`),
	)
	rec := httptest.NewRecorder()

	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if sawPath != "/v1/chat/completions" {
		t.Fatalf("upstream path = %q", sawPath)
	}
	if sawBody != `{"stream":true}` {
		t.Fatalf("upstream body = %q", sawBody)
	}
	if rec.Body.String() != "data: [DONE]\n\n" {
		t.Fatalf("response body = %q", rec.Body.String())
	}
}

func TestProxyRecordsUpstreamFailure(t *testing.T) {
	t.Parallel()

	proxy, err := New("http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()

	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if proxy.LastError() == "" {
		t.Fatal("expected last upstream error")
	}
}

func TestProxyCanRetargetV1Requests(t *testing.T) {
	t.Parallel()

	var saw []string
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saw = append(saw, "first:"+r.URL.Path)
		_, _ = w.Write([]byte("first"))
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saw = append(saw, "second:"+r.URL.Path)
		_, _ = w.Write([]byte("second"))
	}))
	defer second.Close()

	proxy, err := New(first.URL)
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}
	if err := proxy.SetTarget(second.URL); err != nil {
		t.Fatalf("set target: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Body.String() != "second" {
		t.Fatalf("response = %q, want second", rec.Body.String())
	}
	if len(saw) != 1 || saw[0] != "second:/v1/models" {
		t.Fatalf("requests = %#v, want only second upstream", saw)
	}
}

func TestProxyUsesTargetResolverPerRequest(t *testing.T) {
	t.Parallel()

	var saw []string
	main := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saw = append(saw, "main:"+r.URL.Path)
		_, _ = w.Write([]byte("main"))
	}))
	defer main.Close()
	embedding := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saw = append(saw, "embedding:"+r.URL.Path)
		_, _ = w.Write([]byte("embedding"))
	}))
	defer embedding.Close()

	proxy, err := New(main.URL)
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}
	proxy.SetTargetResolver(func(r *http.Request) (string, bool) {
		if r.URL.Path == "/v1/embeddings" {
			return embedding.URL, true
		}
		return "", false
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Body.String() != "embedding" {
		t.Fatalf("response = %q, want embedding", rec.Body.String())
	}
	if len(saw) != 1 || saw[0] != "embedding:/v1/embeddings" {
		t.Fatalf("requests = %#v, want only embedding upstream", saw)
	}
	if got := proxy.TargetForPath("/v1/embeddings"); got != embedding.URL {
		t.Fatalf("target for embeddings = %q, want embedding URL", got)
	}
	if got := proxy.TargetForPath("/v1/chat/completions"); got != main.URL {
		t.Fatalf("target for chat = %q, want main URL", got)
	}
}
