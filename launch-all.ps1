param(
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$SidecarArgs = @()
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$RepoRoot = $PSScriptRoot
$SidecarScript = Join-Path $RepoRoot "launch-sidecar.ps1"
$WebUiScript = Join-Path $RepoRoot "launch-webui.ps1"

& $SidecarScript -Tui @SidecarArgs
& $WebUiScript

Write-Host "Opened LiteRT Sidecar TUI in a separate terminal."
Write-Host "Opened LiteRT Web UI in a separate terminal."
