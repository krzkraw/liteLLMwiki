param(
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$SidecarArgs = @()
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$RepoRoot = $PSScriptRoot
$WebUiScript = Join-Path $RepoRoot "launch-webui.ps1"
$SidecarScript = Join-Path $RepoRoot "launch-sidecar.ps1"

& $WebUiScript
& $SidecarScript -Tui @SidecarArgs

Write-Host "Opened LiteRT Web UI in a separate terminal."
Write-Host "Opened LiteRT Sidecar TUI in a separate terminal."
