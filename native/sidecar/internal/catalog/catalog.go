package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"litert-sidecar/internal/redact"
)

const defaultBaseURL = "https://huggingface.co"

type State string

const (
	StateMissing     State = "missing"
	StatePresent     State = "present"
	StateDownloading State = "downloading"
	StateError       State = "error"
)

type Entry struct {
	ID              string `json:"id"`
	Repo            string `json:"repo"`
	Filename        string `json:"filename"`
	TargetPath      string `json:"targetPath"`
	Runtime         string `json:"runtime"`
	Role            string `json:"role"`
	Required        bool   `json:"required"`
	State           State  `json:"state"`
	BytesDownloaded int64  `json:"bytesDownloaded,omitempty"`
	SizeBytes       int64  `json:"sizeBytes,omitempty"`
	LastError       string `json:"lastError,omitempty"`
}

type Catalog struct {
	mu      sync.Mutex
	entries map[string]Entry
	order   []string
	baseURL string
	client  *http.Client
}

type Option func(*Catalog)

func WithBaseURL(baseURL string) Option {
	return func(c *Catalog) {
		c.baseURL = strings.TrimRight(baseURL, "/")
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(c *Catalog) {
		if client != nil {
			c.client = client
		}
	}
}

func NewDefault(modelRoot string, options ...Option) *Catalog {
	if modelRoot == "" {
		modelRoot = FindModelRoot()
	}

	catalog := &Catalog{
		entries: map[string]Entry{},
		order:   []string{},
		baseURL: defaultBaseURL,
		client:  http.DefaultClient,
	}
	for _, option := range options {
		option(catalog)
	}

	catalog.add(Entry{
		ID:         "gemma4-gguf",
		Repo:       "unsloth/gemma-4-E2B-it-qat-GGUF",
		Filename:   "gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf",
		TargetPath: filepath.Join(modelRoot, "llamacpp", "gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf"),
		Runtime:    "llamacpp",
		Role:       "main",
		Required:   true,
	})
	catalog.add(Entry{
		ID:         "qwen3-embedding-gguf",
		Repo:       "Qwen/Qwen3-Embedding-0.6B-GGUF",
		Filename:   "Qwen3-Embedding-0.6B-Q8_0.gguf",
		TargetPath: filepath.Join(modelRoot, "llamacpp", "Qwen3-Embedding-0.6B-Q8_0.gguf"),
		Runtime:    "llamacpp",
		Role:       "embedding",
		Required:   true,
	})
	catalog.add(Entry{
		ID:         "gemma4-litert",
		Repo:       "litert-community/gemma-4-E2B-it-litert-lm",
		Filename:   "gemma-4-E2B-it.litertlm",
		TargetPath: filepath.Join(modelRoot, "gemma-4-E2B-it.litertlm"),
		Runtime:    "litert",
		Role:       "main",
		Required:   true,
	})
	catalog.add(Entry{
		ID:         "embeddinggemma-litert",
		Repo:       "litert-community/embeddinggemma-300m",
		Filename:   "embeddinggemma-300M_seq2048_mixed-precision.tflite",
		TargetPath: filepath.Join(modelRoot, "litert", "embeddinggemma-300M_seq2048_mixed-precision.tflite"),
		Runtime:    "litert",
		Role:       "embedding",
		Required:   true,
	})
	catalog.refreshLocked()
	return catalog
}

func FindModelRoot() string {
	for _, candidate := range modelRootCandidates() {
		if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
			return candidate
		}
	}
	return "models"
}

func (c *Catalog) Entries() []Entry {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.refreshLocked()
	return c.entriesLocked()
}

func (c *Catalog) Download(ctx context.Context, id string) (Entry, error) {
	c.mu.Lock()
	entry, ok := c.entries[id]
	if !ok {
		c.mu.Unlock()
		return Entry{}, fmt.Errorf("model catalog entry %q not found", id)
	}
	entry.State = StateDownloading
	entry.LastError = ""
	c.entries[id] = entry
	c.mu.Unlock()

	updated, err := c.download(ctx, entry)
	if err != nil {
		c.mu.Lock()
		entry = c.entries[id]
		entry.State = StateError
		entry.LastError = redact.FromEnv(err.Error())
		c.entries[id] = entry
		c.mu.Unlock()
		return entry, err
	}

	c.mu.Lock()
	c.entries[id] = updated
	c.mu.Unlock()
	return updated, nil
}

func (c *Catalog) add(entry Entry) {
	entry.State = StateMissing
	c.entries[entry.ID] = entry
	c.order = append(c.order, entry.ID)
}

func (c *Catalog) refreshLocked() {
	for id, entry := range c.entries {
		stat, err := os.Stat(entry.TargetPath)
		if err == nil && !stat.IsDir() {
			entry.State = StatePresent
			entry.SizeBytes = stat.Size()
			entry.BytesDownloaded = stat.Size()
			entry.LastError = ""
		} else if entry.State != StateDownloading && entry.State != StateError {
			entry.State = StateMissing
			entry.SizeBytes = 0
			entry.BytesDownloaded = 0
		}
		c.entries[id] = entry
	}
}

func (c *Catalog) entriesLocked() []Entry {
	entries := make([]Entry, 0, len(c.order))
	for _, id := range c.order {
		entries = append(entries, c.entries[id])
	}
	return entries
}

func (c *Catalog) download(ctx context.Context, entry Entry) (Entry, error) {
	if err := os.MkdirAll(filepath.Dir(entry.TargetPath), 0o755); err != nil {
		return Entry{}, fmt.Errorf("create model directory: %w", err)
	}

	partialPath := entry.TargetPath + ".part"
	requestURL, err := c.downloadURL(entry)
	if err != nil {
		return Entry{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return Entry{}, fmt.Errorf("create model download request: %w", err)
	}
	if token := huggingFaceToken(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := c.client.Do(request)
	if err != nil {
		return Entry{}, fmt.Errorf("download model %q: %w", entry.ID, err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return Entry{}, fmt.Errorf(
			"download model %q: status %d: %s",
			entry.ID,
			response.StatusCode,
			redact.FromEnv(strings.TrimSpace(string(body))),
		)
	}

	file, err := os.OpenFile(partialPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return Entry{}, fmt.Errorf("open partial model file: %w", err)
	}
	written, copyErr := io.Copy(file, response.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return Entry{}, fmt.Errorf("write partial model file: %w", copyErr)
	}
	if closeErr != nil {
		return Entry{}, fmt.Errorf("close partial model file: %w", closeErr)
	}
	if err := os.Rename(partialPath, entry.TargetPath); err != nil {
		return Entry{}, fmt.Errorf("rename partial model file: %w", err)
	}

	entry.State = StatePresent
	entry.BytesDownloaded = written
	entry.SizeBytes = contentLength(response, written)
	entry.LastError = ""
	return entry, nil
}

func (c *Catalog) downloadURL(entry Entry) (string, error) {
	if c.baseURL == "" {
		return "", errors.New("model download base URL is not configured")
	}
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", fmt.Errorf("parse model download base URL: %w", err)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + entry.Repo + "/resolve/main/" + entry.Filename
	base.RawQuery = ""
	base.Fragment = ""
	return base.String(), nil
}

func huggingFaceToken() string {
	if token := strings.TrimSpace(os.Getenv("HF_TOKEN")); token != "" {
		return token
	}
	return strings.TrimSpace(os.Getenv("HUGGING_FACE_HUB_TOKEN"))
}

func contentLength(response *http.Response, fallback int64) int64 {
	value := strings.TrimSpace(response.Header.Get("content-length"))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func modelRootCandidates() []string {
	candidates := []string{"models", filepath.Join("..", "models"), filepath.Join("..", "..", "models")}
	if currentExe, err := os.Executable(); err == nil {
		dir := filepath.Dir(currentExe)
		candidates = append(candidates,
			filepath.Join(dir, "models"),
			filepath.Join(dir, "..", "models"),
			filepath.Join(dir, "..", "..", "models"),
			filepath.Join(dir, "..", "..", "..", "models"),
		)
	}
	return candidates
}
