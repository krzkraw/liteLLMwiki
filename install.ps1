param(
  [string]$modelsNextcloud = $env:MODELS_NEXTCLOUD
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$RepoRoot = $PSScriptRoot
$SmokePort = if ($env:INSTALL_SMOKE_PORT) { [int]$env:INSTALL_SMOKE_PORT } else { 5177 }
$SmokeUrl = "http://127.0.0.1:$SmokePort/"
$Summary = [System.Collections.Generic.List[string]]::new()
$DevServerProcess = $null
$ModelsNextcloudBase = $null
$ModelsNextcloudToken = $null
$LlamaRuntimeRoot = Join-Path $RepoRoot "native\llama-runtimes"
$LlamaSelectedFile = Join-Path $LlamaRuntimeRoot ".selected"
$LlamaReleaseBase = "https://github.com/ggml-org/llama.cpp/releases/download/b9724"

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

function Initialize-ModelsNextcloud {
  param([string]$Share)

  if ([string]::IsNullOrWhiteSpace($Share)) {
    return
  }

  $Trimmed = $Share.TrimEnd("/")
  if ($Trimmed -match "^(https?://[^/]+)/s/([^/?#]+)") {
    $script:ModelsNextcloudBase = $Matches[1]
    $script:ModelsNextcloudToken = $Matches[2]
    return
  }

  throw "modelsNextcloud must be a Nextcloud public share URL like https://nextcloud.example/s/share-token"
}

function Get-NextcloudModelUrl {
  param([string]$RelativePath)

  return "$script:ModelsNextcloudBase/public.php/webdav/$RelativePath"
}

function Get-BasicAuthorizationHeader {
  param([string]$Token)

  $Bytes = [Text.Encoding]::ASCII.GetBytes("${Token}:")
  return "Basic $([Convert]::ToBase64String($Bytes))"
}

function New-LlamaRuntimeDefinition {
  param(
    [string]$Key,
    [string]$Folder,
    [string]$Label,
    [string]$Url,
    [string]$Sha256,
    [string]$ExtraUrl = "",
    [string]$ExtraSha256 = ""
  )

  [pscustomobject]@{
    Key = $Key
    Folder = $Folder
    Label = $Label
    Url = $Url
    Sha256 = $Sha256
    ExtraUrl = $ExtraUrl
    ExtraSha256 = $ExtraSha256
  }
}

function Get-LlamaRuntimeDefinitions {
  $Architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
  $RunningOnWindows = [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform(
    [System.Runtime.InteropServices.OSPlatform]::Windows
  )
  $RunningOnMacOs = [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform(
    [System.Runtime.InteropServices.OSPlatform]::OSX
  )

  $Definitions = @()

  if ($RunningOnMacOs -and ($Architecture.ToString() -eq "Arm64")) {
    $Definitions += New-LlamaRuntimeDefinition "macos-arm64" "llama-macos-arm64" "macOS Apple Silicon" "$LlamaReleaseBase/llama-b9724-bin-macos-arm64.tar.gz" "sha256:b4582c69bc58e6b84d16105011d9431eeec9a0d1745d9ca8e48472a285db6b7f"
  }
  if ($RunningOnMacOs -and ($Architecture.ToString() -eq "X64")) {
    $Definitions += New-LlamaRuntimeDefinition "macos-x64" "llama-macos-x64" "macOS Intel" "$LlamaReleaseBase/llama-b9724-bin-macos-x64.tar.gz" "sha256:4fd4228bd23dbc6ae53805a89b1811861c1b9da5d2ff07bfd9a08fb5f0c87f6e"
  }
  if ($RunningOnWindows -and ($Architecture.ToString() -eq "X64")) {
    $Definitions += New-LlamaRuntimeDefinition "win-cpu-x64" "llama-win-cpu-x64" "Windows x64 CPU" "$LlamaReleaseBase/llama-b9724-bin-win-cpu-x64.zip" "sha256:e06bafb4e1aaf3745be816d5d072cd965e52ef49ef8e9e93f031e196703780bf"
    $Definitions += New-LlamaRuntimeDefinition "win-cuda13-x64" "llama-win-cuda-13.3-x64" "Windows x64 CUDA 13.3" "$LlamaReleaseBase/llama-b9724-bin-win-cuda-13.3-x64.zip" "sha256:c16700717a20daebc12a2de2bf1ac711ba43f9565dac9d6fbcdf04099dde975a" "$LlamaReleaseBase/cudart-llama-bin-win-cuda-13.3-x64.zip" "sha256:1462a050eb4c684921ba51dcc4cc488a036674c3e73e9945ee705b854808d03e"
    $Definitions += New-LlamaRuntimeDefinition "win-cuda12-x64" "llama-win-cuda-12.4-x64" "Windows x64 CUDA 12.4" "$LlamaReleaseBase/llama-b9724-bin-win-cuda-12.4-x64.zip" "sha256:913d47f80a3cad43fe95eda2ed0cf0dbd5fe01d758f66c097fa0a6138021729d" "$LlamaReleaseBase/cudart-llama-bin-win-cuda-12.4-x64.zip" "sha256:8c79a9b226de4b3cacfd1f83d24f962d0773be79f1e7b75c6af4ded7e32ae1d6"
    $Definitions += New-LlamaRuntimeDefinition "win-vulkan-x64" "llama-win-vulkan-x64" "Windows x64 Vulkan" "$LlamaReleaseBase/llama-b9724-bin-win-vulkan-x64.zip" "sha256:3e245e75f38477f9c99858cf149a3831988701090d156512eb143f2312b76b44"
    $Definitions += New-LlamaRuntimeDefinition "win-openvino-x64" "llama-win-openvino-2026.2-x64" "Windows x64 OpenVINO" "$LlamaReleaseBase/llama-b9724-bin-win-openvino-2026.2-x64.zip" "sha256:da36f6380bbeffddd4db58bfbc09077982c465d92123e943e6af679e8ed5d0ec"
    $Definitions += New-LlamaRuntimeDefinition "win-sycl-x64" "llama-win-sycl-x64" "Windows x64 SYCL" "$LlamaReleaseBase/llama-b9724-bin-win-sycl-x64.zip" "sha256:f660e83887af4a1c62742010a8064ab26aa9befacecaa5c86c6061ae68a3c04f"
    $Definitions += New-LlamaRuntimeDefinition "win-hip-x64" "llama-win-hip-radeon-x64" "Windows x64 HIP Radeon" "$LlamaReleaseBase/llama-b9724-bin-win-hip-radeon-x64.zip" "sha256:2b861729d7b1620a7ee09ebc8681f2534be9da307f93fd68afb6410f160a016b"
  }
  if ($RunningOnWindows -and ($Architecture.ToString() -eq "Arm64")) {
    $Definitions += New-LlamaRuntimeDefinition "win-cpu-arm64" "llama-win-cpu-arm64" "Windows arm64 CPU" "$LlamaReleaseBase/llama-b9724-bin-win-cpu-arm64.zip" "sha256:092191286aa8c1d11e909308358e6ac9bd7b5dc83e01d71d96807f6b0cf948bf"
    $Definitions += New-LlamaRuntimeDefinition "win-opencl-adreno-arm64" "llama-win-opencl-adreno-arm64" "Windows arm64 OpenCL Adreno" "$LlamaReleaseBase/llama-b9724-bin-win-opencl-adreno-arm64.zip" "sha256:3e465918a49382fd466003e2d1658b261e87c68b8aa77c087a441ef3b7dee62c"
  }

  return $Definitions
}

function Find-InstalledLlamaServer {
  foreach ($Name in @("llama-server.exe", "llama-server")) {
    $Command = Get-Command $Name -ErrorAction SilentlyContinue
    if ($null -ne $Command) {
      return $Command.Source
    }
  }

  if (Test-Path $LlamaSelectedFile) {
    $RuntimeName = (Get-Content $LlamaSelectedFile -Raw).Trim()
    if ($RuntimeName) {
      $RuntimeDir = Join-Path $LlamaRuntimeRoot $RuntimeName
      if (Test-Path $RuntimeDir) {
        $Match = Get-ChildItem -Path $RuntimeDir -Filter "llama-server*" -File -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($null -ne $Match) {
          return $Match.FullName
        }
      }
    }
  }

  if (Test-Path $LlamaRuntimeRoot) {
    $Match = Get-ChildItem -Path $LlamaRuntimeRoot -Filter "llama-server*" -File -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($null -ne $Match) {
      return $Match.FullName
    }
  }

  return ""
}

function Assert-ArchiveSha256 {
  param(
    [string]$Path,
    [string]$ExpectedSha256
  )

  $Expected = $ExpectedSha256.Replace("sha256:", "")
  $Actual = (Get-FileHash -Algorithm SHA256 -Path $Path).Hash.ToLowerInvariant()
  if ($Actual -ne $Expected) {
    throw "SHA256 mismatch for $Path. Expected $Expected, got $Actual"
  }
}

function Expand-LlamaArchive {
  param(
    [string]$Archive,
    [string]$TargetDir
  )

  if ($Archive.EndsWith(".zip")) {
    Expand-Archive -Path $Archive -DestinationPath $TargetDir -Force
    return
  }

  if ($Archive.EndsWith(".tar.gz")) {
    & tar -xzf $Archive -C $TargetDir
    if ($LASTEXITCODE -ne 0) {
      throw "tar failed for $Archive"
    }
    return
  }

  throw "Unsupported llama.cpp archive: $Archive"
}

function Install-LlamaAsset {
  param(
    [string]$Url,
    [string]$Sha256,
    [string]$TargetDir
  )

  $ArchiveName = Split-Path -Leaf $Url
  $Archive = Join-Path ([System.IO.Path]::GetTempPath()) "$([Guid]::NewGuid().ToString('N'))-$ArchiveName"
  Invoke-WebRequest -Uri $Url -OutFile $Archive
  Assert-ArchiveSha256 -Path $Archive -ExpectedSha256 $Sha256
  Expand-LlamaArchive -Archive $Archive -TargetDir $TargetDir
  Remove-Item -Force $Archive
}

function Install-LlamaRuntime {
  param([object]$Definition)

  $TargetDir = Join-Path $LlamaRuntimeRoot $Definition.Folder
  $TempDir = "$TargetDir.tmp"

  Write-Host ""
  Write-Host "llama.cpp runtime needs to be installed downloaded, here is the command or URL I would use:"
  Write-Host "Runtime: $($Definition.Label)"
  Write-Host "Folder: native/llama-runtimes/$($Definition.Folder)"
  Write-Host "URL: $($Definition.Url)"
  Write-Host "sha256: $($Definition.Sha256.Replace('sha256:', ''))"
  if ($Definition.ExtraUrl) {
    Write-Host "CUDA DLL URL: $($Definition.ExtraUrl)"
    Write-Host "sha256: $($Definition.ExtraSha256.Replace('sha256:', ''))"
  }

  New-Item -ItemType Directory -Force -Path $LlamaRuntimeRoot | Out-Null
  Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $TempDir
  New-Item -ItemType Directory -Force -Path $TempDir | Out-Null
  Install-LlamaAsset -Url $Definition.Url -Sha256 $Definition.Sha256 -TargetDir $TempDir
  if ($Definition.ExtraUrl) {
    Install-LlamaAsset -Url $Definition.ExtraUrl -Sha256 $Definition.ExtraSha256 -TargetDir $TempDir
  }
  Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $TargetDir
  Move-Item -Force $TempDir $TargetDir
  Set-Content -Path $LlamaSelectedFile -Value $Definition.Folder
  Add-Summary "OK: llama.cpp runtime $($Definition.Folder)"
}

function Ensure-LlamaRuntime {
  $Installed = Find-InstalledLlamaServer
  if (-not [string]::IsNullOrWhiteSpace($Installed)) {
    Add-Summary "OK: llama-server at $Installed"
    return
  }

  $Definitions = @(Get-LlamaRuntimeDefinitions)
  if ($Definitions.Count -eq 0) {
    Invoke-ConfirmOrWait -Label "llama-server" -CommandText "https://github.com/ggml-org/llama.cpp/releases" -Check {
      -not [string]::IsNullOrWhiteSpace((Find-InstalledLlamaServer))
    } -Action {
      Start-Process "https://github.com/ggml-org/llama.cpp/releases"
      throw "Manual install required"
    }
    return
  }

  Write-Host ""
  Write-Host "llama.cpp runtime needs to be installed downloaded. Choose one option, or all:"
  foreach ($Definition in $Definitions) {
    Write-Host "  $($Definition.Key): $($Definition.Label) -> native/llama-runtimes/$($Definition.Folder)"
    Write-Host "      $($Definition.Url)"
    if ($Definition.ExtraUrl) {
      Write-Host "      CUDA DLLs: $($Definition.ExtraUrl)"
    }
  }
  Write-Host "  all: install every option listed above"
  Write-Host "  skip: I will install llama-server myself and press Enter"

  while ($true) {
    $DefaultChoice = $Definitions[0].Key
    $Choice = Read-Host "llama.cpp runtime choice [all/$DefaultChoice/skip]"
    if ([string]::IsNullOrWhiteSpace($Choice)) {
      $Choice = $DefaultChoice
    }
    if ($Choice -eq "all") {
      foreach ($Definition in $Definitions) {
        Install-LlamaRuntime -Definition $Definition
      }
      return
    }
    if ($Choice -eq "skip") {
      Invoke-WaitForUserAction -Label "llama-server" -Check {
        -not [string]::IsNullOrWhiteSpace((Find-InstalledLlamaServer))
      }
      Add-Summary "OK: llama-server"
      return
    }

    $Selected = $Definitions | Where-Object { $_.Key -eq $Choice } | Select-Object -First 1
    if ($null -ne $Selected) {
      Install-LlamaRuntime -Definition $Selected
      return
    }
    Write-Host "Unknown llama.cpp runtime choice: $Choice"
  }
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
    [string]$Target,
    [string]$AuthKind = "huggingface",
    [string]$AuthToken = ""
  )

  $Directory = Split-Path -Parent $Target
  $Partial = "$Target.partial"
  New-Item -ItemType Directory -Force -Path $Directory | Out-Null
  Remove-Item -Force -ErrorAction SilentlyContinue $Partial

  $Headers = @{}
  if ($AuthKind -eq "nextcloud") {
    $Headers["Authorization"] = Get-BasicAuthorizationHeader -Token $AuthToken
  } else {
    if ($env:HF_TOKEN) {
      $Headers["Authorization"] = "Bearer $($env:HF_TOKEN)"
    } elseif ($env:HUGGING_FACE_HUB_TOKEN) {
      $Headers["Authorization"] = "Bearer $($env:HUGGING_FACE_HUB_TOKEN)"
    }
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
  $DownloadUrl = $Url
  $DownloadAuthKind = "huggingface"
  $DownloadAuthToken = ""
  $NeedsToken = $MayNeedToken

  if ($script:ModelsNextcloudBase) {
    $DownloadUrl = Get-NextcloudModelUrl -RelativePath $RelativePath
    $DownloadAuthKind = "nextcloud"
    $DownloadAuthToken = $script:ModelsNextcloudToken
    $NeedsToken = $false
  }

  if ($DownloadAuthKind -eq "nextcloud") {
    $CommandText = "URL: $DownloadUrl`nPath: $RelativePath`nCommand: Invoke-WebRequest -Uri '$DownloadUrl' -Headers @{ Authorization = 'Basic <modelsNextcloud-token>' } -OutFile '$RelativePath'"
  } else {
    $CommandText = "URL: $DownloadUrl`nPath: $RelativePath`nCommand: Invoke-WebRequest -Uri '$DownloadUrl' -OutFile '$RelativePath'"
  }

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
      if ($NeedsToken) {
        Prompt-HfTokenIfNeeded
      }
      try {
        Download-Model -Url $DownloadUrl -Target $Target -AuthKind $DownloadAuthKind -AuthToken $DownloadAuthToken
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

Initialize-ModelsNextcloud -Share $modelsNextcloud

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
  Ensure-LlamaRuntime

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
