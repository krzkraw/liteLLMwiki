package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultCatalogContainsRequiredArtifacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	catalog := NewDefault(root)
	entries := catalog.Entries()

	assertEntry(t, entries, "gemma4-gguf", filepath.Join(root, "llamacpp", "main", "gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf"))
	assertEntry(t, entries, "qwen35-2b-gguf", filepath.Join(root, "llamacpp", "main", "Qwen3.5-2B-IQ4_NL.gguf"))
	assertEntry(t, entries, "qwen35-08b-gguf", filepath.Join(root, "llamacpp", "main", "Qwen3.5-0.8B-UD-Q8_K_XL.gguf"))
	assertEntry(t, entries, "qwen3-embedding-q8-mungert", filepath.Join(root, "llamacpp", "embedding", "Qwen3-Embedding-0.6B-q8_0.gguf"))
	assertEntry(t, entries, "qwen3-embedding-iq4-mungert", filepath.Join(root, "llamacpp", "embedding", "Qwen3-Embedding-0.6B-iq4_nl.gguf"))
	assertEntry(t, entries, "qwen3-reranker-q4km", filepath.Join(root, "llamacpp", "reranking", "Qwen3-Reranker-0.6B-Q4_K_M.gguf"))
	assertEntry(t, entries, "gemma4-litert", filepath.Join(root, "litert", "main", "gemma-4-E2B-it.litertlm"))
	assertEntry(t, entries, "gemma4-web-litert", filepath.Join(root, "litert", "browser", "gemma-4-E2B-it-web.litertlm"))
	assertEntry(t, entries, "embeddinggemma-litert", filepath.Join(root, "litert", "embedding", "embeddinggemma-300M_seq2048_mixed-precision.tflite"))

	if len(entries) != 9 {
		t.Fatalf("entry count = %d, want 9", len(entries))
	}

	for _, entry := range entries {
		if entry.Required != true {
			t.Fatalf("entry %q required = false, want true", entry.ID)
		}
		if entry.State != StateMissing {
			t.Fatalf("entry %q state = %q, want missing", entry.ID, entry.State)
		}
	}
}

func TestDefaultCatalogUsesRequestedHuggingFaceRepositories(t *testing.T) {
	t.Parallel()

	entries := NewDefault(t.TempDir()).Entries()

	assertRepoFilenameRole(
		t,
		entries,
		"gemma4-web-litert",
		"litert-community/gemma-4-E2B-it-litert-lm",
		"gemma-4-E2B-it-web.litertlm",
		"litert",
		"browser",
	)
	assertRepoFilenameRole(
		t,
		entries,
		"qwen3-reranker-q4km",
		"Voodisss/Qwen3-Reranker-0.6B-GGUF-llama_cpp",
		"Qwen3-Reranker-0.6B-Q4_K_M.gguf",
		"llamacpp",
		"reranking",
	)
	assertRepoFilenameRole(
		t,
		entries,
		"qwen3-embedding-q8-mungert",
		"Mungert/Qwen3-Embedding-0.6B-GGUF",
		"Qwen3-Embedding-0.6B-q8_0.gguf",
		"llamacpp",
		"embedding",
	)
	assertRepoFilenameRole(
		t,
		entries,
		"qwen3-embedding-iq4-mungert",
		"Mungert/Qwen3-Embedding-0.6B-GGUF",
		"Qwen3-Embedding-0.6B-iq4_nl.gguf",
		"llamacpp",
		"embedding",
	)
	assertRepoFilenameRole(
		t,
		entries,
		"qwen35-2b-gguf",
		"unsloth/Qwen3.5-2B-GGUF",
		"Qwen3.5-2B-IQ4_NL.gguf",
		"llamacpp",
		"main",
	)
	assertRepoFilenameRole(
		t,
		entries,
		"qwen35-08b-gguf",
		"unsloth/Qwen3.5-0.8B-GGUF",
		"Qwen3.5-0.8B-UD-Q8_K_XL.gguf",
		"llamacpp",
		"main",
	)

	for _, entry := range entries {
		if entry.ID == "qwen3-embedding-gguf" {
			t.Fatalf("removed legacy catalog entry qwen3-embedding-gguf is still present")
		}
	}
}

func TestDownloadUsesHuggingFaceTokenAndAtomicRename(t *testing.T) {
	t.Setenv("HF_TOKEN", "hf_secret")

	root := t.TempDir()
	var sawAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/unsloth/gemma-4-E2B-it-qat-GGUF/resolve/main/gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("content-length", "10")
		_, _ = w.Write([]byte("model-data"))
	}))
	defer server.Close()

	catalog := NewDefault(root, WithBaseURL(server.URL))
	entry, err := catalog.Download(context.Background(), "gemma4-gguf")
	if err != nil {
		t.Fatalf("download: %v", err)
	}

	if sawAuth != "Bearer hf_secret" {
		t.Fatalf("authorization = %q, want bearer token", sawAuth)
	}
	content, err := os.ReadFile(entry.TargetPath)
	if err != nil {
		t.Fatalf("read downloaded model: %v", err)
	}
	if string(content) != "model-data" {
		t.Fatalf("content = %q", string(content))
	}
	if _, err := os.Stat(entry.TargetPath + ".part"); !os.IsNotExist(err) {
		t.Fatalf("partial file still exists: %v", err)
	}
	if entry.State != StatePresent {
		t.Fatalf("state = %q, want present", entry.State)
	}
	if entry.BytesDownloaded != 10 || entry.SizeBytes != 10 {
		t.Fatalf("bytes = %d/%d, want 10/10", entry.BytesDownloaded, entry.SizeBytes)
	}
}

func TestDownloadErrorsRedactTokens(t *testing.T) {
	t.Setenv("HUGGING_FACE_HUB_TOKEN", "hf_secret")

	root := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad token hf_secret", http.StatusUnauthorized)
	}))
	defer server.Close()

	catalog := NewDefault(root, WithBaseURL(server.URL))
	_, err := catalog.Download(context.Background(), "gemma4-gguf")
	if err == nil {
		t.Fatal("expected download error")
	}
	if strings.Contains(err.Error(), "hf_secret") {
		t.Fatalf("error leaked token: %s", err)
	}
	if !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("error = %q, want redaction marker", err.Error())
	}
}

func assertEntry(t *testing.T, entries []Entry, id string, targetPath string) {
	t.Helper()

	for _, entry := range entries {
		if entry.ID == id {
			if entry.TargetPath != targetPath {
				t.Fatalf("%s target = %q, want %q", id, entry.TargetPath, targetPath)
			}
			return
		}
	}
	encoded, _ := json.Marshal(entries)
	t.Fatalf("entry %q not found in %s", id, encoded)
}

func assertRepoFilenameRole(
	t *testing.T,
	entries []Entry,
	id string,
	repo string,
	filename string,
	runtimeName string,
	role string,
) {
	t.Helper()

	for _, entry := range entries {
		if entry.ID != id {
			continue
		}
		if entry.Repo != repo {
			t.Fatalf("%s repo = %q, want %q", id, entry.Repo, repo)
		}
		if entry.Filename != filename {
			t.Fatalf("%s filename = %q, want %q", id, entry.Filename, filename)
		}
		if entry.Runtime != runtimeName || entry.Role != role {
			t.Fatalf("%s runtime/role = %q/%q, want %q/%q", id, entry.Runtime, entry.Role, runtimeName, role)
		}
		return
	}
	encoded, _ := json.Marshal(entries)
	t.Fatalf("entry %q not found in %s", id, encoded)
}
