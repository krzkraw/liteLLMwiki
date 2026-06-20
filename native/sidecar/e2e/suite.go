package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"litert-sidecar/internal/catalog"
	"litert-sidecar/internal/runtimeconfig"
)

type SuiteOptions struct {
	RepoRoot          string
	BackendConfigPath string
	ModelRoot         string
	LiteRTExecutable  string
	LlamaExecutable   string
	LiteRTRuntimeRoot string
	LlamaRuntimeRoot  string
	ForceReal         bool
}

type RuntimeBackendPlan struct {
	ConfigPath string
	Combos     []RuntimeBackendCombo

	skipReason string
}

type RuntimeBackendCombo struct {
	Runtime       string
	ConfigBackend string
	RunnerBackend string
	Role          string
	ModelID       string
	ModelPath     string
	Executable    string
	LlamaVariant  string
	ConfigResult  runtimeconfig.BackendResult
	SkipReason    string
}

func (p RuntimeBackendPlan) ReadyCount() int {
	ready := 0
	for _, combo := range p.Combos {
		if combo.SkipReason == "" {
			ready++
		}
	}
	return ready
}

func (p RuntimeBackendPlan) SkipReason() string {
	if p.skipReason != "" {
		return p.skipReason
	}
	if p.ReadyCount() > 0 {
		return ""
	}
	if len(p.Combos) == 0 {
		return "no working backend combos found"
	}

	parts := make([]string, 0, len(p.Combos))
	for _, combo := range p.Combos {
		if combo.SkipReason == "" {
			continue
		}
		parts = append(parts, combo.Name()+": "+combo.SkipReason)
	}
	if len(parts) == 0 {
		return "no runnable working backend combos found"
	}
	return "no runnable working backend combos: " + strings.Join(parts, "; ")
}

func (c RuntimeBackendCombo) Name() string {
	role := c.Role
	if role == "" {
		role = "main"
	}
	return c.Runtime + "/" + c.ConfigBackend + "/" + role
}

func PlanRuntimeBackendCombos(options SuiteOptions) (RuntimeBackendPlan, error) {
	repoRoot := strings.TrimSpace(options.RepoRoot)
	if repoRoot == "" {
		repoRoot = findRepoRoot()
	}
	if repoRoot == "" {
		return RuntimeBackendPlan{}, fmt.Errorf("repo root could not be found")
	}

	configPath := strings.TrimSpace(options.BackendConfigPath)
	if configPath == "" {
		if envPath := strings.TrimSpace(os.Getenv("RUNTIME_BACKEND_CONFIG")); envPath != "" {
			configPath = envPath
		} else {
			configPath = runtimeconfig.DefaultPath(repoRoot)
		}
	}

	plan := RuntimeBackendPlan{ConfigPath: configPath}
	if stat, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			plan.skipReason = "backend config not found: " + configPath
			return plan, nil
		}
		return RuntimeBackendPlan{}, fmt.Errorf("stat backend config: %w", err)
	} else if stat.IsDir() {
		plan.skipReason = "backend config path is a directory: " + configPath
		return plan, nil
	}

	status, err := runtimeconfig.Load(configPath)
	if err != nil {
		return RuntimeBackendPlan{}, fmt.Errorf("load backend config: %w", err)
	}
	working := status.WorkingBackends()
	if len(working) == 0 {
		plan.skipReason = "backend config has no working:true combos: " + configPath
		return plan, nil
	}

	modelRoot := strings.TrimSpace(options.ModelRoot)
	if modelRoot == "" {
		modelRoot = filepath.Join(repoRoot, "models")
	}
	modelCatalog := catalog.NewDefault(modelRoot)
	litertExecutable := resolveLiteRTExecutable(repoRoot, options)
	llamaExecutable := resolveLlamaExecutable(repoRoot, options)
	llamaVariants := discoverLlamaRuntimeVariants(repoRoot, options)

	for _, item := range working {
		combo := RuntimeBackendCombo{
			Runtime:       item.Runtime,
			ConfigBackend: item.Backend,
			RunnerBackend: runnerBackend(item.Runtime, item.Backend),
			Role:          "main",
			ConfigResult:  item.Result,
		}
		switch item.Runtime {
		case "litert":
			combo.Executable = litertExecutable
			combo = attachCatalogModel(combo, modelCatalog)
			combo.SkipReason = litertSkipReason(combo)
		case "llamacpp":
			combo.Executable = llamaExecutable
			if combo.Executable == "" {
				if variant, ok := llamaVariants[item.Backend]; ok {
					combo.Executable = variant.Executable
					combo.LlamaVariant = variant.Name
					combo.RunnerBackend = variant.RunnerBackend
				}
			}
			combo = attachCatalogModel(combo, modelCatalog)
			combo.SkipReason = llamaSkipReason(combo)
		default:
			combo.SkipReason = "unsupported runtime " + item.Runtime
		}
		plan.Combos = append(plan.Combos, combo)
	}
	sort.Slice(plan.Combos, func(left int, right int) bool {
		return plan.Combos[left].Name() < plan.Combos[right].Name()
	})
	return plan, nil
}

type llamaRuntimeVariant struct {
	Name          string
	Type          string
	RunnerBackend string
	Executable    string
}

func attachCatalogModel(combo RuntimeBackendCombo, modelCatalog *catalog.Catalog) RuntimeBackendCombo {
	for _, entry := range modelCatalog.Entries() {
		if entry.Runtime != combo.Runtime || entry.Role != combo.Role {
			continue
		}
		if entry.State != catalog.StatePresent {
			continue
		}
		combo.ModelID = entry.ID
		combo.ModelPath = entry.TargetPath
		return combo
	}
	return combo
}

func litertSkipReason(combo RuntimeBackendCombo) string {
	if combo.ModelPath == "" {
		return "missing model for litert/main under models/litert/main"
	}
	if combo.Executable == "" {
		return "missing executable for litert; set LITERT_LM_BIN or install native/litert-runtimes"
	}
	return ""
}

func llamaSkipReason(combo RuntimeBackendCombo) string {
	if combo.Executable == "" {
		return "no installed runtime variant for backend " + combo.ConfigBackend + "; set LLAMA_SERVER_BIN or install native/llama-runtimes"
	}
	if combo.ModelPath == "" {
		return "missing model for llamacpp/main under models/llamacpp/main"
	}
	return ""
}

func resolveLiteRTExecutable(repoRoot string, options SuiteOptions) string {
	if executable := firstExecutable(
		options.LiteRTExecutable,
		os.Getenv("LITERT_LM_BIN"),
	); executable != "" {
		return executable
	}

	root := options.LiteRTRuntimeRoot
	if strings.TrimSpace(root) == "" {
		root = filepath.Join(repoRoot, "native", "litert-runtimes")
	}
	if selected := os.Getenv("LITERT_RUNTIME"); strings.TrimSpace(selected) != "" {
		if executable := findNamedExecutable(filepath.Join(root, selected), litertExecutableNames()); executable != "" {
			return executable
		}
	}
	if selected := selectedRuntimeName(filepath.Join(root, ".selected")); selected != "" {
		if executable := findNamedExecutable(filepath.Join(root, selected), litertExecutableNames()); executable != "" {
			return executable
		}
	}
	if executable := findNamedExecutable(root, litertExecutableNames()); executable != "" {
		return executable
	}
	if path, err := exec.LookPath(litertExecutableNames()[0]); err == nil {
		return path
	}
	return ""
}

func resolveLlamaExecutable(repoRoot string, options SuiteOptions) string {
	if executable := firstExecutable(
		options.LlamaExecutable,
		os.Getenv("LLAMA_SERVER_BIN"),
	); executable != "" {
		return executable
	}

	root := options.LlamaRuntimeRoot
	if strings.TrimSpace(root) == "" {
		root = filepath.Join(repoRoot, "native", "llama-runtimes")
	}
	if selected := os.Getenv("LLAMA_RUNTIME"); strings.TrimSpace(selected) != "" {
		if executable := findNamedExecutable(filepath.Join(root, selected), llamaExecutableNames()); executable != "" {
			return executable
		}
	}
	if selected := selectedRuntimeName(filepath.Join(root, ".selected")); selected != "" {
		if executable := findNamedExecutable(filepath.Join(root, selected), llamaExecutableNames()); executable != "" {
			return executable
		}
	}
	return ""
}

func discoverLlamaRuntimeVariants(
	repoRoot string,
	options SuiteOptions,
) map[string]llamaRuntimeVariant {
	root := options.LlamaRuntimeRoot
	if strings.TrimSpace(root) == "" {
		root = filepath.Join(repoRoot, "native", "llama-runtimes")
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return map[string]llamaRuntimeVariant{}
	}

	variants := map[string]llamaRuntimeVariant{}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		executable := findNamedExecutable(dir, llamaExecutableNames())
		if executable == "" {
			continue
		}
		runtimeType := llamaRuntimeType(entry.Name())
		if _, exists := variants[runtimeType]; exists {
			continue
		}
		variants[runtimeType] = llamaRuntimeVariant{
			Name:          entry.Name(),
			Type:          runtimeType,
			RunnerBackend: runnerBackend("llamacpp", runtimeType),
			Executable:    executable,
		}
	}
	return variants
}

func firstExecutable(paths ...string) string {
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if executableUsable(path) {
			return path
		}
	}
	return ""
}

func executableUsable(path string) bool {
	stat, err := os.Stat(path)
	if err != nil || stat.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return stat.Mode()&0o111 != 0
}

func findNamedExecutable(root string, names []string) string {
	if strings.TrimSpace(root) == "" {
		return ""
	}
	var found string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || found != "" || entry.IsDir() {
			return nil
		}
		for _, name := range names {
			if entry.Name() == name && executableUsable(path) {
				found = path
				return filepath.SkipAll
			}
		}
		return nil
	})
	return found
}

func selectedRuntimeName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func litertExecutableNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"litert-lm.exe", "litert-lm"}
	}
	return []string{"litert-lm", "litert-lm.exe"}
}

func llamaExecutableNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"llama-server.exe", "llama-server"}
	}
	return []string{"llama-server", "llama-server.exe"}
}

func runnerBackend(runtimeName string, configBackend string) string {
	backend := strings.ToLower(strings.TrimSpace(configBackend))
	if runtimeName != "llamacpp" {
		return backend
	}
	switch backend {
	case "cuda12", "cuda13":
		return "cuda"
	default:
		return backend
	}
}

func llamaRuntimeType(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "cuda-13") || strings.Contains(lower, "cuda13") || strings.Contains(lower, "cuda-13."):
		return "cuda13"
	case strings.Contains(lower, "cuda-12") || strings.Contains(lower, "cuda12") || strings.Contains(lower, "cuda-12."):
		return "cuda12"
	case strings.Contains(lower, "openvino"):
		return "openvino"
	case strings.Contains(lower, "sycl"):
		return "sycl"
	case strings.Contains(lower, "vulkan"), strings.Contains(lower, "hip"), strings.Contains(lower, "radeon"), strings.Contains(lower, "opencl"):
		return "gpu"
	default:
		return "cpu"
	}
}

func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if stat, err := os.Stat(filepath.Join(dir, "package.json")); err == nil && !stat.IsDir() {
			if sidecarStat, sidecarErr := os.Stat(filepath.Join(dir, "native", "sidecar", "go.mod")); sidecarErr == nil && !sidecarStat.IsDir() {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
