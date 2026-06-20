package runtimeconfig

import (
	"encoding/json"
	"errors"
	"fmt"
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

// SetBackendWorking stores the enabled state for one runtime backend.
func SetBackendWorking(
	path string,
	runtimeName string,
	backend string,
	working bool,
) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("backend config path is empty")
	}

	normalizedRuntime := normalize(runtimeName)
	if normalizedRuntime == "" {
		return fmt.Errorf("runtime name is empty")
	}
	normalizedBackend := normalize(backend)
	if normalizedBackend == "" {
		return fmt.Errorf("backend name is empty")
	}

	config, err := readFileConfig(path)
	if err != nil {
		return err
	}
	if config.Version == 0 {
		config.Version = 1
	}
	config.Runtimes = normalizedRuntimeResults(config.Runtimes)
	if config.Runtimes == nil {
		config.Runtimes = map[string]map[string]BackendResult{}
	}
	if config.Runtimes[normalizedRuntime] == nil {
		config.Runtimes[normalizedRuntime] = map[string]BackendResult{}
	}

	result := config.Runtimes[normalizedRuntime][normalizedBackend]
	result.Working = working
	config.Runtimes[normalizedRuntime][normalizedBackend] = result

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode backend config: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create backend config directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write backend config: %w", err)
	}
	return nil
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

func readFileConfig(path string) (fileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fileConfig{
				Version:  1,
				Runtimes: map[string]map[string]BackendResult{},
			}, nil
		}
		return fileConfig{}, fmt.Errorf("read backend config: %w", err)
	}

	var decoded fileConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fileConfig{}, fmt.Errorf("decode backend config: %w", err)
	}
	return decoded, nil
}

func normalizedRuntimeResults(
	runtimes map[string]map[string]BackendResult,
) map[string]map[string]BackendResult {
	if runtimes == nil {
		return nil
	}

	normalized := make(map[string]map[string]BackendResult, len(runtimes))
	for runtimeName, backendResults := range runtimes {
		normalizedRuntime := normalize(runtimeName)
		if normalizedRuntime == "" {
			continue
		}
		if normalized[normalizedRuntime] == nil {
			normalized[normalizedRuntime] = map[string]BackendResult{}
		}
		for backend, result := range backendResults {
			normalizedBackend := normalize(backend)
			if normalizedBackend == "" {
				continue
			}
			normalized[normalizedRuntime][normalizedBackend] = result
		}
	}
	return normalized
}
