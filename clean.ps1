$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$RepoRoot = $PSScriptRoot

& git -C $RepoRoot rev-parse --is-inside-work-tree | Out-Null
if ($LASTEXITCODE -ne 0) {
  throw "clean.ps1 must be run from inside the liteLLMwiki Git checkout."
}

Write-Host "Cleaning ignored and untracked files from $RepoRoot"
Write-Host "Preserving models/"
Write-Host "Running: git clean -xdf -e models/"

& git -C $RepoRoot clean -xdf -e "models/"
