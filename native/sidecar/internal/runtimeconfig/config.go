package runtimeconfig

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const defaultConfigRelativePath = "native/runtime-config/backends.json"

type BackendResult struct {
	Working bool   `json:"working"`
	Command string `json:"command,omitempty"`
	Model   string `json:"model,omitempty"`
	Tested  string `json:"testedAt,omitempty"`
	Output  string `json:"output,omitempty"`
}

type fileConfig struct {
	Version  int                                 `json:"version"`
	Runtimes map[string]map[string]BackendResult `json:"runtimes"`
}

type Status struct {
	configured bool
	runtimes   map[string]map[string]BackendResult
}

func DefaultPath(repoRoot string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(defaultConfigRelativePath))
}

func Load(path string) (Status, error) {
	if strings.TrimSpace(path) == "" {
		return Status{
			runtimes: map[string]map[string]BackendResult{},
		}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Status{
				runtimes: map[string]map[string]BackendResult{},
			}, nil
		}
		return Status{}, err
	}

	var decoded fileConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		return Status{}, err
	}

	runtimes := make(map[string]map[string]BackendResult, len(decoded.Runtimes))
	for runtimeName, backendResults := range decoded.Runtimes {
		normalizedRuntime := normalize(runtimeName)
		if normalizedRuntime == "" {
			continue
		}
		runtimes[normalizedRuntime] = make(map[string]BackendResult, len(backendResults))
		for backend, result := range backendResults {
			normalizedBackend := normalize(backend)
			if normalizedBackend == "" {
				continue
			}
			runtimes[normalizedRuntime][normalizedBackend] = result
		}
	}

	return Status{
		configured: true,
		runtimes:   runtimes,
	}, nil
}

func (s Status) Configured() bool {
	return s.configured
}

func (s Status) Visible(runtimeName string, backend string) bool {
	if !s.configured {
		return true
	}
	backends := s.runtimes[normalize(runtimeName)]
	if backends == nil {
		return true
	}
	result, ok := backends[normalize(backend)]
	if !ok {
		return true
	}
	return result.Working
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
