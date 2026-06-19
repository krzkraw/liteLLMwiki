package scripts

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type releaseArtifact struct {
	dir      string
	binary   string
	magicSet [][]byte
}

var releaseArtifacts = []releaseArtifact{
	{
		dir:    "litert-sidecar-darwin-arm64",
		binary: "litert-sidecar",
		magicSet: [][]byte{
			{0xfe, 0xed, 0xfa, 0xcf},
			{0xcf, 0xfa, 0xed, 0xfe},
		},
	},
	{
		dir:    "litert-sidecar-darwin-amd64",
		binary: "litert-sidecar",
		magicSet: [][]byte{
			{0xfe, 0xed, 0xfa, 0xcf},
			{0xcf, 0xfa, 0xed, 0xfe},
		},
	},
	{
		dir:      "litert-sidecar-windows-amd64",
		binary:   "litert-sidecar.exe",
		magicSet: [][]byte{{'M', 'Z'}},
	},
	{
		dir:      "litert-sidecar-windows-arm64",
		binary:   "litert-sidecar.exe",
		magicSet: [][]byte{{'M', 'Z'}},
	},
}

func TestBuildReleaseArtifactsHaveExpectedShape(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve sidecar root: %v", err)
	}
	out := t.TempDir()
	cmd, err := releaseBuildCommand(root, out, runtime.GOOS, exec.LookPath)
	if err != nil {
		t.Skip(err)
	}
	cmd.Dir = root

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build-release.sh failed: %v\n%s", err, output)
	}

	for _, artifact := range releaseArtifacts {
		artifact := artifact
		t.Run(artifact.dir, func(t *testing.T) {
			dir := filepath.Join(out, artifact.dir)
			assertRegularFile(t, filepath.Join(dir, artifact.binary))
			assertRegularFile(t, filepath.Join(dir, "README.md"))
			assertMagic(t, filepath.Join(dir, artifact.binary), artifact.magicSet)
		})
	}
}

func TestShellReleaseScriptDefaultsToNativeSidecarArtifacts(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile("build-release.sh")
	if err != nil {
		t.Fatalf("read build-release.sh: %v", err)
	}
	if !strings.Contains(string(content), `repo_root/native/sidecar-artifacts`) {
		t.Fatalf("build-release.sh does not default to native/sidecar-artifacts")
	}
}

type pathLookup func(name string) (string, error)

func releaseBuildCommand(
	root string,
	out string,
	goos string,
	lookPath pathLookup,
) (*exec.Cmd, error) {
	if goos == "windows" {
		shellPath, err := findPowerShell(lookPath)
		if err != nil {
			return nil, err
		}
		return exec.Command(
			shellPath,
			"-NoProfile",
			"-ExecutionPolicy",
			"Bypass",
			"-File",
			filepath.Join(root, "scripts", "build-release.ps1"),
			"-OutDir",
			out,
		), nil
	}

	bashPath, err := lookPath("bash")
	if err != nil {
		return nil, err
	}

	return exec.Command(
		bashPath,
		filepath.Join(root, "scripts", "build-release.sh"),
		out,
	), nil
}

func findPowerShell(lookPath pathLookup) (string, error) {
	for _, candidate := range []string{"pwsh", "powershell"} {
		shellPath, err := lookPath(candidate)
		if err == nil {
			return shellPath, nil
		}
	}

	return "", exec.ErrNotFound
}

func TestReleaseBuildCommandUsesPowerShellOnWindows(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(`C:\repo\native\sidecar`)
	out := filepath.Clean(`C:\tmp\release`)
	cmd, err := releaseBuildCommand(root, out, "windows", func(name string) (string, error) {
		if name == "pwsh" {
			return filepath.Join(`C:\Program Files\PowerShell\7`, "pwsh.exe"), nil
		}

		return "", exec.ErrNotFound
	})
	if err != nil {
		t.Fatalf("releaseBuildCommand returned error: %v", err)
	}

	wantArgs := []string{
		"-NoProfile",
		"-ExecutionPolicy",
		"Bypass",
		"-File",
		filepath.Join(root, "scripts", "build-release.ps1"),
		"-OutDir",
		out,
	}
	if cmd.Path != filepath.Join(`C:\Program Files\PowerShell\7`, "pwsh.exe") {
		t.Fatalf("cmd.Path = %q", cmd.Path)
	}
	if !reflect.DeepEqual(cmd.Args[1:], wantArgs) {
		t.Fatalf("cmd.Args[1:] = %#v, want %#v", cmd.Args[1:], wantArgs)
	}
}

func TestPowerShellReleaseScriptRestoresGoEnvironmentWhenRun(t *testing.T) {
	t.Parallel()

	shellPath, err := findPowerShell(exec.LookPath)
	if err != nil {
		t.Skip("PowerShell is required to execute build-release.ps1")
	}

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve sidecar root: %v", err)
	}

	tmp := t.TempDir()
	fakeGoDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(fakeGoDir, 0o755); err != nil {
		t.Fatalf("create fake go dir: %v", err)
	}

	fakeGoName := "go"
	fakeGoContent := "#!/usr/bin/env sh\nexit 0\n"
	if runtime.GOOS == "windows" {
		fakeGoName = "go.cmd"
		fakeGoContent = "@echo off\r\nexit /b 0\r\n"
	}
	if err := os.WriteFile(
		filepath.Join(fakeGoDir, fakeGoName),
		[]byte(fakeGoContent),
		0o755,
	); err != nil {
		t.Fatalf("write fake go: %v", err)
	}

	out := filepath.Join(tmp, "dist")
	envLog := filepath.Join(tmp, "go-env.txt")
	wrapper := filepath.Join(tmp, "restore-check.ps1")
	wrapperContent := fmt.Sprintf(`
$ErrorActionPreference = "Stop"
$env:CGO_ENABLED = "original-cgo"
$env:GOOS = "original-os"
$env:GOARCH = "original-arch"
. %s -OutDir %s
Set-Content -NoNewline -Path %s -Value "$env:CGO_ENABLED|$env:GOOS|$env:GOARCH"
`,
		powershellQuote(filepath.Join(root, "scripts", "build-release.ps1")),
		powershellQuote(out),
		powershellQuote(envLog),
	)
	if err := os.WriteFile(wrapper, []byte(wrapperContent), 0o644); err != nil {
		t.Fatalf("write PowerShell wrapper: %v", err)
	}

	cmd := exec.Command(
		shellPath,
		"-NoProfile",
		"-ExecutionPolicy",
		"Bypass",
		"-File",
		wrapper,
	)
	cmd.Dir = root
	cmd.Env = append(
		os.Environ(),
		fmt.Sprintf(
			"PATH=%s%c%s",
			fakeGoDir,
			os.PathListSeparator,
			os.Getenv("PATH"),
		),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build-release.ps1 wrapper failed: %v\n%s", err, output)
	}

	content, err := os.ReadFile(envLog)
	if err != nil {
		t.Fatalf("read env log: %v", err)
	}
	if string(content) != "original-cgo|original-os|original-arch" {
		t.Fatalf("restored env = %q", content)
	}
}

func powershellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func TestPowerShellReleaseScriptRestoresGoEnvironment(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve sidecar root: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(root, "scripts", "build-release.ps1"))
	if err != nil {
		t.Fatalf("read build-release.ps1: %v", err)
	}
	text := string(content)

	for _, snippet := range []string{
		"$PreviousGoEnv",
		"Restore-GoEnv",
		"Set-GoEnv",
	} {
		if !bytes.Contains([]byte(text), []byte(snippet)) {
			t.Fatalf("build-release.ps1 does not contain %q", snippet)
		}
	}
}

func assertRegularFile(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("%s is a directory", path)
	}
	if info.Size() == 0 {
		t.Fatalf("%s is empty", path)
	}
}

func assertMagic(t *testing.T, path string, candidates [][]byte) {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()

	buf := make([]byte, 4)
	n, err := file.Read(buf)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	for _, candidate := range candidates {
		if n >= len(candidate) && bytes.Equal(buf[:len(candidate)], candidate) {
			return
		}
	}

	t.Fatalf("%s magic bytes = % x, want one of % x", path, buf[:n], candidates)
}
