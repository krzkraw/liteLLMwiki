param()

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$RepoRoot = $PSScriptRoot
$SmokePort = if ($env:INSTALL_SMOKE_PORT) { [int]$env:INSTALL_SMOKE_PORT } else { 5177 }
$SmokeUrl = "http://127.0.0.1:$SmokePort/"
$Summary = [System.Collections.Generic.List[string]]::new()
$DevServerProcess = $null

Set-Location $RepoRoot

function Add-Summary {
  param([string]$Message)
  $script:Summary.Add($Message) | Out-Null
}

function Test-Command {
  param([string]$Name)
  return $null -ne (Get-Command $Name -ErrorAction SilentlyContinue)
}

function Invoke-RunLogged {
  param(
    [string]$Label,
    [scriptblock]$Action
  )

  Write-Host ""
  Write-Host "==> $Label"
  & $Action
  if ($LASTEXITCODE -ne 0) {
    throw "$Label failed with exit code $LASTEXITCODE"
  }
  Add-Summary "PASS: $Label"
}

function Get-PackageInstallCommand {
  param(
    [string]$WingetId,
    [string]$ChocoPackage,
    [string]$ManualUrl
  )

  if (Test-Command "winget") {
    return "winget install --id $WingetId -e"
  }

  if (Test-Command "choco") {
    return "choco install $ChocoPackage -y"
  }

  return $ManualUrl
}

function Invoke-WaitForUserAction {
  param(
    [string]$Label,
    [scriptblock]$Check
  )

  Write-Host "I will wait. Press Enter after you have done it: $Label"
  while ($true) {
    [void](Read-Host)
    if (& $Check) {
      return
    }
    Write-Host "Still not detected. Press Enter after completing it, or Ctrl-C to stop."
  }
}

function Invoke-ConfirmOrWait {
  param(
    [string]$Label,
    [string]$CommandText,
    [scriptblock]$Check,
    [scriptblock]$Action
  )

  if (& $Check) {
    Add-Summary "OK: $Label"
    return
  }

  Write-Host ""
  Write-Host "$Label needs to be installed downloaded, here is the command or URL I would use:"
  Write-Host $CommandText

  while (-not (& $Check)) {
    $Answer = Read-Host "Do you want me to do it? [y/N]"
    if ($Answer -match "^(y|yes)$") {
      try {
        & $Action
      } catch {
        Write-Host "The task failed. Here is the command or URL again:"
        Write-Host $CommandText
        Invoke-WaitForUserAction -Label $Label -Check $Check
      }
    } else {
      Invoke-WaitForUserAction -Label $Label -Check $Check
    }
  }

  Add-Summary "OK: $Label"
}

function Ensure-Dependency {
  param(
    [string]$Label,
    [string]$CommandName,
    [string]$CommandText
  )

  Invoke-ConfirmOrWait -Label $Label -CommandText $CommandText -Check {
    Test-Command $CommandName
  } -Action {
    Invoke-Expression $CommandText
  }
}

function Prompt-HfTokenIfNeeded {
  if ($env:HF_TOKEN -or $env:HUGGING_FACE_HUB_TOKEN) {
    return
  }

  $Answer = Read-Host "This Hugging Face download may need a token. Paste one now? [y/N]"
  if ($Answer -match "^(y|yes)$") {
    $SecureToken = Read-Host "HF token" -AsSecureString
    $Bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($SecureToken)
    try {
      $Token = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($Bstr)
      $env:HF_TOKEN = $Token
      $env:HUGGING_FACE_HUB_TOKEN = $Token
    } finally {
      [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($Bstr)
    }
  }
}

function Download-Model {
  param(
    [string]$Url,
    [string]$Target
  )

  $Directory = Split-Path -Parent $Target
  $Partial = "$Target.partial"
  New-Item -ItemType Directory -Force -Path $Directory | Out-Null
  Remove-Item -Force -ErrorAction SilentlyContinue $Partial

  $Headers = @{}
  if ($env:HF_TOKEN) {
    $Headers["Authorization"] = "Bearer $($env:HF_TOKEN)"
  } elseif ($env:HUGGING_FACE_HUB_TOKEN) {
    $Headers["Authorization"] = "Bearer $($env:HUGGING_FACE_HUB_TOKEN)"
  }

  Invoke-WebRequest -Uri $Url -OutFile $Partial -Headers $Headers
  Move-Item -Force $Partial $Target
}

function Ensure-Model {
  param(
    [string]$Label,
    [string]$RelativePath,
    [string]$Url,
    [bool]$MayNeedToken
  )

  $Target = Join-Path $RepoRoot $RelativePath
  $CommandText = "URL: $Url`nPath: $RelativePath`nCommand: Invoke-WebRequest -Uri '$Url' -OutFile '$RelativePath'"

  if ((Test-Path $Target) -and ((Get-Item $Target).Length -gt 0)) {
    Add-Summary "OK: $Label at $RelativePath"
    return
  }

  Write-Host ""
  Write-Host "$Label needs to be installed downloaded, here is the command or URL I would use:"
  Write-Host $CommandText

  while ((-not (Test-Path $Target)) -or ((Get-Item $Target -ErrorAction SilentlyContinue).Length -eq 0)) {
    $Answer = Read-Host "Do you want me to do it? [y/N]"
    if ($Answer -match "^(y|yes)$") {
      if ($MayNeedToken) {
        Prompt-HfTokenIfNeeded
      }
      try {
        Download-Model -Url $Url -Target $Target
      } catch {
        Write-Host "The task failed. Here is the command or URL again:"
        Write-Host $CommandText
        Write-Host "I will wait. Press Enter after you have put the file at $RelativePath"
        [void](Read-Host)
      }
    } else {
      Write-Host "I will wait. Open the URL in a browser if needed and put the file at:"
      Write-Host $RelativePath
      Write-Host "Press Enter after the file is there."
      [void](Read-Host)
    }
  }

  Add-Summary "OK: $Label at $RelativePath"
}

function Ensure-NpmDependencies {
  Invoke-ConfirmOrWait -Label "npm dependencies" -CommandText "npm install" -Check {
    (Test-Path (Join-Path $RepoRoot "node_modules")) -and
      (Test-Path (Join-Path $RepoRoot "public\vendor\litert-lm\core\wasm"))
  } -Action {
    & npm install
    if ($LASTEXITCODE -ne 0) {
      throw "npm install failed"
    }
  }
}

function Wait-ForUrl {
  param([string]$Url)

  for ($Index = 0; $Index -lt 80; $Index += 1) {
    try {
      Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 2 | Out-Null
      return $true
    } catch {
      Start-Sleep -Milliseconds 250
    }
  }

  return $false
}

function Stop-DevServer {
  if (($null -ne $script:DevServerProcess) -and (-not $script:DevServerProcess.HasExited)) {
    Stop-Process -Id $script:DevServerProcess.Id -ErrorAction SilentlyContinue
    $script:DevServerProcess.WaitForExit()
  }
}

function Run-SmokeTests {
  Write-Host ""
  Write-Host "==> Starting temporary web UI for smoke tests at $SmokeUrl"

  $LogPath = Join-Path ([System.IO.Path]::GetTempPath()) "litert-wiki-install-vite.log"
  $script:DevServerProcess = Start-Process -FilePath "npm" -ArgumentList @(
    "run",
    "dev",
    "--",
    "--host",
    "127.0.0.1",
    "--port",
    [string]$SmokePort,
    "--strictPort"
  ) -WorkingDirectory $RepoRoot -NoNewWindow -RedirectStandardOutput $LogPath -RedirectStandardError $LogPath -PassThru

  if (-not (Wait-ForUrl $SmokeUrl)) {
    Get-Content $LogPath
    throw "Temporary web UI did not become ready."
  }

  try {
    Invoke-RunLogged "smoke UI" {
      $env:SMOKE_URL = $SmokeUrl
      & npm run smoke
    }
    Invoke-RunLogged "smoke executable sidecar" {
      $env:SMOKE_URL = $SmokeUrl
      & npm run smoke:executable
    }

    $WebModel = Join-Path $RepoRoot "models\litert\gemma-4-E2B-it-web.litertlm"
    if ((Test-Path $WebModel) -and ((Get-Item $WebModel).Length -gt 0)) {
      Invoke-RunLogged "smoke web model" {
        $env:SMOKE_URL = $SmokeUrl
        & npm run smoke:model
      }
    } else {
      Add-Summary "SKIP: smoke web model, models/litert/gemma-4-E2B-it-web.litertlm missing"
    }
  } finally {
    Stop-DevServer
  }
}

function Print-Summary {
  Write-Host ""
  Write-Host "Summary"
  Write-Host "-------"
  foreach ($Item in $Summary) {
    Write-Host $Item
  }
  Write-Host ""
  Write-Host "Next command:"
  Write-Host ".\launch-all.ps1"
}

try {
  $NodeCommand = Get-PackageInstallCommand "OpenJS.NodeJS.LTS" "nodejs-lts" "Install Node.js from https://nodejs.org/"
  $GitCommand = Get-PackageInstallCommand "Git.Git" "git" "Install Git from https://git-scm.com/download/win"
  $GoCommand = Get-PackageInstallCommand "GoLang.Go" "golang" "Install Go from https://go.dev/dl/"
  $CurlCommand = Get-PackageInstallCommand "cURL.cURL" "curl" "Install curl with winget, choco, or from https://curl.se/windows/"
  $UvCommand = Get-PackageInstallCommand "astral-sh.uv" "uv" "Install uv from https://docs.astral.sh/uv/getting-started/installation/"

  Ensure-Dependency "git" "git" $GitCommand
  Ensure-Dependency "node" "node" $NodeCommand
  Ensure-Dependency "npm" "npm" $NodeCommand
  Ensure-Dependency "go" "go" $GoCommand
  Ensure-Dependency "curl" "curl" $CurlCommand
  Ensure-Dependency "uv" "uv" $UvCommand
  Ensure-Dependency "litert-lm" "litert-lm" "uv tool install litert-lm"
  Invoke-ConfirmOrWait -Label "llama-server" -CommandText "https://github.com/ggml-org/llama.cpp/releases" -Check {
    Test-Command "llama-server"
  } -Action {
    Start-Process "https://github.com/ggml-org/llama.cpp/releases"
    throw "Manual install required"
  }

  Ensure-NpmDependencies

  Ensure-Model "Gemma 4 E2B web model" "models/litert/gemma-4-E2B-it-web.litertlm" "https://huggingface.co/litert-community/gemma-4-E2B-it-litert-lm/resolve/main/gemma-4-E2B-it-web.litertlm" $true
  Ensure-Model "Gemma 4 E2B native LiteRT model" "models/litert/gemma-4-E2B-it.litertlm" "https://huggingface.co/litert-community/gemma-4-E2B-it-litert-lm/resolve/main/gemma-4-E2B-it.litertlm" $true
  Ensure-Model "Gemma 4 E2B llama.cpp GGUF model" "models/llamacpp/gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf" "https://huggingface.co/unsloth/gemma-4-E2B-it-qat-GGUF/resolve/main/gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf" $false
  Ensure-Model "Qwen3 embedding GGUF model" "models/llamacpp/Qwen3-Embedding-0.6B-Q8_0.gguf" "https://huggingface.co/Qwen/Qwen3-Embedding-0.6B-GGUF/resolve/main/Qwen3-Embedding-0.6B-Q8_0.gguf" $false
  Ensure-Model "EmbeddingGemma LiteRT embedding model" "models/litert/embeddinggemma-300M_seq2048_mixed-precision.tflite" "https://huggingface.co/litert-community/embeddinggemma-300m/resolve/main/embeddinggemma-300M_seq2048_mixed-precision.tflite" $true

  Invoke-RunLogged "npm test" { & npm test }
  Invoke-RunLogged "web production build" { & npm run build }
  Invoke-RunLogged "sidecar artifacts build" { & npm run build:sidecar }
  Run-SmokeTests

  Print-Summary
} finally {
  Stop-DevServer
}
