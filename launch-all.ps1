param(
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$SidecarArgs = @()
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$RepoRoot = $PSScriptRoot
$PowerShellExe = if (Get-Command pwsh -ErrorAction SilentlyContinue) {
  (Get-Command pwsh).Source
} else {
  (Get-Command powershell).Source
}

$SidecarScript = Join-Path $RepoRoot "launch-sidecar.ps1"
$WebUiScript = Join-Path $RepoRoot "launch-webui.ps1"

$SidecarProcessArgs = @(
  "-NoExit",
  "-NoProfile",
  "-ExecutionPolicy",
  "Bypass",
  "-File",
  $SidecarScript,
  "-Inline"
) + $SidecarArgs

$WebUiProcessArgs = @(
  "-NoExit",
  "-NoProfile",
  "-ExecutionPolicy",
  "Bypass",
  "-File",
  $WebUiScript,
  "-Inline"
)

Start-Process -FilePath $PowerShellExe -ArgumentList $SidecarProcessArgs -WorkingDirectory $RepoRoot | Out-Null
Start-Process -FilePath $PowerShellExe -ArgumentList $WebUiProcessArgs -WorkingDirectory $RepoRoot | Out-Null

Write-Host "Opened LiteRT Sidecar TUI in a separate terminal."
Write-Host "Opened LiteRT Web UI in a separate terminal."
