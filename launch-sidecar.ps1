param(
  [switch]$Headless,
  [string]$SidecarBin = "",
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$ExtraArgs = @()
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$RepoRoot = $PSScriptRoot
$SidecarArgs = @()

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

if ($SidecarBin -eq "") {
  $SidecarBin = if ($env:SIDECAR_BIN) { $env:SIDECAR_BIN } else { Get-DefaultSidecarBin }
}

if ($Headless -or ($env:SIDECAR_HEADLESS -match "^(1|true|yes)$")) {
  $SidecarArgs += "--headless"
}

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
