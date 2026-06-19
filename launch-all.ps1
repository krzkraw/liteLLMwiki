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

if (-not $env:SIDECAR_HEADLESS) {
  $env:SIDECAR_HEADLESS = "1"
}

$WebUiProcess = $null
$SidecarProcess = $null

function cleanup {
  foreach ($Process in @($script:WebUiProcess, $script:SidecarProcess)) {
    if (($null -ne $Process) -and (-not $Process.HasExited)) {
      Stop-Process -Id $Process.Id -ErrorAction SilentlyContinue
      $Process.WaitForExit()
    }
  }
}

$SidecarScript = Join-Path $RepoRoot "launch-sidecar.ps1"
$WebUiScript = Join-Path $RepoRoot "launch-webui.ps1"

$SidecarProcessArgs = @(
  "-NoProfile",
  "-ExecutionPolicy",
  "Bypass",
  "-File",
  $SidecarScript,
  "-Headless"
) + $SidecarArgs

$WebUiProcessArgs = @(
  "-NoProfile",
  "-ExecutionPolicy",
  "Bypass",
  "-File",
  $WebUiScript
)

try {
  $SidecarProcess = Start-Process -FilePath $PowerShellExe -ArgumentList $SidecarProcessArgs -WorkingDirectory $RepoRoot -NoNewWindow -PassThru
  $WebUiProcess = Start-Process -FilePath $PowerShellExe -ArgumentList $WebUiProcessArgs -WorkingDirectory $RepoRoot -NoNewWindow -PassThru

  Write-Host "Sidecar started with PID $($SidecarProcess.Id)"
  Write-Host "Web UI started with PID $($WebUiProcess.Id)"

  while ((-not $SidecarProcess.HasExited) -and (-not $WebUiProcess.HasExited)) {
    Start-Sleep -Seconds 1
    $SidecarProcess.Refresh()
    $WebUiProcess.Refresh()
  }

  if ($SidecarProcess.HasExited) {
    exit $SidecarProcess.ExitCode
  }

  exit $WebUiProcess.ExitCode
} finally {
  cleanup
}
