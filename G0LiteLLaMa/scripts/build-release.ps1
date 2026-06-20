param(
  [string]$OutDir = ""
)

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Root = Resolve-Path (Join-Path $ScriptDir "..")
if ($OutDir -eq "") {
  $RepoRoot = Resolve-Path (Join-Path $Root "..\..")
  $OutDir = Join-Path $RepoRoot "G0LiteLLaMa\dist"
}

$Targets = @(
  @{ GOOS = "darwin"; GOARCH = "arm64"; Suffix = "darwin-arm64"; Binary = "g0litellama" },
  @{ GOOS = "darwin"; GOARCH = "amd64"; Suffix = "darwin-amd64"; Binary = "g0litellama" },
  @{ GOOS = "windows"; GOARCH = "amd64"; Suffix = "windows-amd64"; Binary = "g0litellama.exe" },
  @{ GOOS = "windows"; GOARCH = "arm64"; Suffix = "windows-arm64"; Binary = "g0litellama.exe" }
)

function Set-GoEnv {
  param(
    [string]$Name,
    [string]$Value
  )

  Set-Item -Path "Env:$Name" -Value $Value
}

function Restore-GoEnv {
  param(
    [hashtable]$PreviousGoEnv
  )

  foreach ($Name in $PreviousGoEnv.Keys) {
    if ($null -eq $PreviousGoEnv[$Name]) {
      Remove-Item -Path "Env:$Name" -ErrorAction SilentlyContinue
    } else {
      Set-GoEnv -Name $Name -Value $PreviousGoEnv[$Name]
    }
  }
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

foreach ($Target in $Targets) {
  $Dir = Join-Path $OutDir ("g0litellama-" + $Target.Suffix)
  if (Test-Path $Dir) {
    Remove-Item -Recurse -Force $Dir
  }
  New-Item -ItemType Directory -Force -Path $Dir | Out-Null

  $PreviousGoEnv = @{
    CGO_ENABLED = [Environment]::GetEnvironmentVariable("CGO_ENABLED")
    GOOS = [Environment]::GetEnvironmentVariable("GOOS")
    GOARCH = [Environment]::GetEnvironmentVariable("GOARCH")
  }

  Push-Location $Root
  try {
    Set-GoEnv -Name "CGO_ENABLED" -Value "0"
    Set-GoEnv -Name "GOOS" -Value $Target.GOOS
    Set-GoEnv -Name "GOARCH" -Value $Target.GOARCH
    go build -trimpath -ldflags "-s -w" -o (Join-Path $Dir $Target.Binary) .\cmd\g0litellama
  } finally {
    Pop-Location
    Restore-GoEnv -PreviousGoEnv $PreviousGoEnv
  }

  Copy-Item (Join-Path $Root "README.md") (Join-Path $Dir "README.md")
}

Write-Host "Built G0LiteLLaMa release artifacts in $OutDir"
