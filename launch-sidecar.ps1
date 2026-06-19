param(
  [switch]$Inline,
  [switch]$Headless,
  [string]$SidecarBin = "",
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$ExtraArgs = @()
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$RepoRoot = $PSScriptRoot
$LlamaRuntimeRoot = Join-Path $RepoRoot "native\llama-runtimes"
$LlamaSelectedFile = Join-Path $LlamaRuntimeRoot ".selected"
$SidecarArgs = @()

function Get-PowerShellExe {
  if (Get-Command pwsh -ErrorAction SilentlyContinue) {
    return (Get-Command pwsh).Source
  }

  return (Get-Command powershell).Source
}

if (-not $Inline) {
  $ProcessArgs = @(
    "-NoExit",
    "-ExecutionPolicy",
    "Bypass",
    "-File",
    $PSCommandPath,
    "-Inline"
  )
  if ($Headless) {
    $ProcessArgs += "-Headless"
  }
  if (-not [string]::IsNullOrWhiteSpace($SidecarBin)) {
    $ProcessArgs += @("-SidecarBin", $SidecarBin)
  }
  $ProcessArgs += $ExtraArgs

  Start-Process -FilePath (Get-PowerShellExe) -ArgumentList $ProcessArgs -WorkingDirectory $RepoRoot | Out-Null
  Write-Host "Opened LiteRT Sidecar TUI in a separate terminal."
  exit 0
}

function Get-EnvValue {
  param([string]$Name)
  return [Environment]::GetEnvironmentVariable($Name)
}

function Add-ValueFlag {
  param(
    [string]$EnvName,
    [string]$FlagName
  )

  $Value = Get-EnvValue $EnvName
  if (-not [string]::IsNullOrWhiteSpace($Value)) {
    $script:SidecarArgs += @($FlagName, $Value)
  }
}

function Add-BoolFlag {
  param(
    [string]$EnvName,
    [string]$FlagName
  )

  $Value = Get-EnvValue $EnvName
  if (-not [string]::IsNullOrWhiteSpace($Value)) {
    $script:SidecarArgs += "$FlagName=$Value"
  }
}

function Get-DefaultSidecarBin {
  $Architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
  $ArchSuffix = switch ($Architecture.ToString()) {
    "Arm64" { "arm64" }
    "X64" { "amd64" }
    default { "" }
  }

  if ($ArchSuffix -eq "") {
    return ""
  }

  return Join-Path $RepoRoot "native\sidecar-artifacts\litert-sidecar-windows-$ArchSuffix\litert-sidecar.exe"
}

function Find-LlamaServerBin {
  if ($env:LLAMA_SERVER_BIN -and (Test-Path $env:LLAMA_SERVER_BIN)) {
    return $env:LLAMA_SERVER_BIN
  }

  if ($env:LLAMA_RUNTIME) {
    $RuntimeDir = Join-Path $LlamaRuntimeRoot $env:LLAMA_RUNTIME
    if (Test-Path $RuntimeDir) {
      $Match = Get-ChildItem -Path $RuntimeDir -Filter "llama-server.exe" -File -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
      if ($null -ne $Match) {
        return $Match.FullName
      }
    }
  }

  if (Test-Path $LlamaSelectedFile) {
    $RuntimeName = (Get-Content $LlamaSelectedFile -Raw).Trim()
    if ($RuntimeName) {
      $RuntimeDir = Join-Path $LlamaRuntimeRoot $RuntimeName
      if (Test-Path $RuntimeDir) {
        $Match = Get-ChildItem -Path $RuntimeDir -Filter "llama-server.exe" -File -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($null -ne $Match) {
          return $Match.FullName
        }
      }
    }
  }

  if (Test-Path $LlamaRuntimeRoot) {
    $Match = Get-ChildItem -Path $LlamaRuntimeRoot -Filter "llama-server.exe" -File -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($null -ne $Match) {
      return $Match.FullName
    }
  }

  return ""
}

function Add-LlamaRuntimeToPath {
  $LlamaServerBin = Find-LlamaServerBin
  if (-not [string]::IsNullOrWhiteSpace($LlamaServerBin)) {
    $Directory = Split-Path -Parent $LlamaServerBin
    $env:PATH = "$Directory$([IO.Path]::PathSeparator)$env:PATH"
  }
}

if ($SidecarBin -eq "") {
  $SidecarBin = if ($env:SIDECAR_BIN) { $env:SIDECAR_BIN } else { Get-DefaultSidecarBin }
}

if ($Headless -or ($env:SIDECAR_HEADLESS -match "^(1|true|yes)$")) {
  $SidecarArgs += "--headless"
}

Add-LlamaRuntimeToPath

Add-ValueFlag "SIDECAR_ADDR" "-addr"
Add-ValueFlag "SIDECAR_UPSTREAM" "-upstream"
Add-ValueFlag "LITERT_LM_BIN" "-runtime-exe"
Add-ValueFlag "SIDECAR_RUNTIME_HOST" "-runtime-host"
Add-ValueFlag "SIDECAR_RUNTIME_PORT" "-runtime-port"
Add-ValueFlag "MODEL_FILE" "-model-file"
Add-ValueFlag "MODEL_ID" "-model-id"
Add-BoolFlag "SIDECAR_LAUNCH_RUNTIME" "-launch-runtime"
Add-BoolFlag "SIDECAR_IMPORT_MODEL" "-import-model"
Add-BoolFlag "SIDECAR_RUNTIME_VERBOSE" "-runtime-verbose"

if (($SidecarBin -ne "") -and (Test-Path $SidecarBin)) {
  & $SidecarBin @SidecarArgs @ExtraArgs
  exit $LASTEXITCODE
}

Push-Location (Join-Path $RepoRoot "native\sidecar")
try {
  & go run .\cmd\litert-sidecar @SidecarArgs @ExtraArgs
  exit $LASTEXITCODE
} finally {
  Pop-Location
}
