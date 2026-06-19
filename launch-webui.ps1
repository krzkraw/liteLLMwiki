param(
  [switch]$Inline,
  [string]$WebHost = "",
  [int]$Port = 0,
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$ExtraArgs = @()
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
$RepoRoot = $PSScriptRoot

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
  if (-not [string]::IsNullOrWhiteSpace($WebHost)) {
    $ProcessArgs += @("-WebHost", $WebHost)
  }
  if ($Port -ne 0) {
    $ProcessArgs += @("-Port", [string]$Port)
  }
  $ProcessArgs += $ExtraArgs

  Start-Process -FilePath (Get-PowerShellExe) -ArgumentList $ProcessArgs -WorkingDirectory $RepoRoot | Out-Null
  Write-Host "Opened LiteRT Web UI in a separate terminal."
  exit 0
}

if ($WebHost -eq "") {
  $WebHost = if ($env:WEBUI_HOST) { $env:WEBUI_HOST } else { "127.0.0.1" }
}

if ($Port -eq 0) {
  $Port = if ($env:WEBUI_PORT) { [int]$env:WEBUI_PORT } else { 5173 }
}

Set-Location $RepoRoot
$NpmArgs = @("run", "dev", "--", "--host", $WebHost, "--port", [string]$Port) + $ExtraArgs
& npm @NpmArgs
exit $LASTEXITCODE
