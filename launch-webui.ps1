param(
  [string]$WebHost = "",
  [int]$Port = 0,
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$ExtraArgs = @()
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

if ($WebHost -eq "") {
  $WebHost = if ($env:WEBUI_HOST) { $env:WEBUI_HOST } else { "127.0.0.1" }
}

if ($Port -eq 0) {
  $Port = if ($env:WEBUI_PORT) { [int]$env:WEBUI_PORT } else { 5173 }
}

Set-Location $PSScriptRoot
$NpmArgs = @("run", "dev", "--", "--host", $WebHost, "--port", [string]$Port) + $ExtraArgs
& npm @NpmArgs
exit $LASTEXITCODE
