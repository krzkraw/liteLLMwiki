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

function Write-GreenCheck {
  param([string]$Message)

  Write-Host "  ✓ $Message" -ForegroundColor Green
}

function Write-TaskPending {
  param([string]$Message)

  Write-Host "  [ ] $Message"
}

function Write-TaskStatus {
  param(
    [string]$Label,
    [scriptblock]$Check,
    [string]$DoneText = "done",
    [string]$PendingText = "pending"
  )

  if (& $Check) {
    Write-GreenCheck "$Label - $DoneText"
  } else {
    Write-TaskPending "$Label - $PendingText"
  }
}

function Write-BoxLine {
  param([string]$Text = "")

  Write-Host "| $Text"
}

function Write-TaskBox {
  param(
    [string]$Label,
    [string]$Description,
    [string]$CommandText,
    [string]$ExpectedResult
  )

  Write-Host ""
  Write-Host "+------------------------------------------------------------+"
  Write-Host "| Task: $Label"
  Write-Host "| Description: $Description"
  Write-Host "| Command or URL I would use:"
  foreach ($Line in ($CommandText -split "`r?`n")) {
    Write-BoxLine "  $Line"
  }
  Write-Host "| Expected result: $ExpectedResult"
  Write-BoxLine "Do you want me to do it?"
  Write-Host "| Choices:"
  Write-BoxLine "  [Y] Yes - run it now"
  Write-BoxLine "  [N] No - stop this installer"
  Write-BoxLine "  [M] Manual & wait - I will do it and press Enter"
  Write-Host "+------------------------------------------------------------+"
}

function Read-TaskChoice {
  param(
    [string]$Label,
    [string]$Description,
    [string]$CommandText,
    [string]$ExpectedResult
  )

  Write-TaskBox -Label $Label -Description $Description -CommandText $CommandText -ExpectedResult $ExpectedResult
  while ($true) {
    $Answer = Read-Host "Choice [Y/N/M]"
    if ($Answer -match "^(y|yes)$") {
      return "yes"
    }
    if ($Answer -match "^(n|no)$") {
      return "no"
    }
    if ($Answer -match "^(m|manual)$") {
      return "manual"
    }
    Write-Host "Choose Y, N, or M."
  }
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
  Write-GreenCheck "$Label - done"
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

  $SharePath = $RelativePath -replace "^models[\\/]", ""
  return "$script:ModelsNextcloudBase/public.php/webdav/$SharePath"
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
    Write-GreenCheck "llama.cpp runtime - already done"
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
    } -ExpectedResult "llama-server is available on PATH or under native/llama-runtimes" `
      -Description "Install llama.cpp so the sidecar can start a llama-server runtime."
    return
  }

  $SelectedKeys = [System.Collections.Generic.List[string]]::new()
  $RuntimeCommandText = "Source: $LlamaReleaseBase`nDestination: native/llama-runtimes`nThe installer will show platform runtime choices and download selected archives."
  $RuntimeChoice = Read-TaskChoice `
    -Label "llama.cpp runtime" `
    -Description "Download one or more local llama.cpp runtime folders, or install llama-server manually." `
    -CommandText $RuntimeCommandText `
    -ExpectedResult "llama-server is available on PATH or under native/llama-runtimes"
  if ($RuntimeChoice -eq "no") {
    throw "Installer stopped before llama.cpp runtime."
  }
  if ($RuntimeChoice -eq "manual") {
    Invoke-WaitForUserAction -Label "llama-server" -Check {
      -not [string]::IsNullOrWhiteSpace((Find-InstalledLlamaServer))
    } -ExpectedResult "llama-server is available on PATH or under native/llama-runtimes"
    Add-Summary "OK: llama-server"
    return
  }

  while ($true) {
    Write-Host ""
    Write-Host "llama.cpp runtime needs to be installed downloaded. Select one or more runtimes:"
    for ($RuntimeIndex = 0; $RuntimeIndex -lt $Definitions.Count; $RuntimeIndex += 1) {
      $Definition = $Definitions[$RuntimeIndex]
      $Checked = "[ ]"
      if ($SelectedKeys.Contains($Definition.Key)) {
        $Checked = "[x]"
      }
      Write-Host ("  {0}) {1} {2}: {3} -> native/llama-runtimes/{4}" -f ($RuntimeIndex + 1), $Checked, $Definition.Key, $Definition.Label, $Definition.Folder)
      Write-Host "      $($Definition.Url)"
      if ($Definition.ExtraUrl) {
        Write-Host "      CUDA DLLs: $($Definition.ExtraUrl)"
      }
    }
    Write-Host "  a: toggle all"
    Write-Host "  c: continue"
    Write-Host "  s: skip, I will install llama-server myself and press Enter"

    $SelectionText = Read-Host "Toggle numbers, a: toggle all, c: continue, s: skip"
    if ([string]::IsNullOrWhiteSpace($SelectionText)) {
      $SelectionText = "c"
    }

    if ($SelectionText -eq "a") {
      if ($SelectedKeys.Count -eq $Definitions.Count) {
        $SelectedKeys.Clear()
      } else {
        $SelectedKeys.Clear()
        foreach ($Definition in $Definitions) {
          [void]$SelectedKeys.Add($Definition.Key)
        }
      }
      continue
    }

    if ($SelectionText -eq "c") {
      if ($SelectedKeys.Count -eq 0) {
        Write-Host "Select at least one runtime, or use s to skip."
        continue
      }

      $PrimaryKey = $SelectedKeys[0]
      foreach ($SelectedKey in @($SelectedKeys)) {
        $Selected = $Definitions | Where-Object { $_.Key -eq $SelectedKey } | Select-Object -First 1
        if ($null -ne $Selected) {
          Install-LlamaRuntime -Definition $Selected
        }
      }
      $Primary = $Definitions | Where-Object { $_.Key -eq $PrimaryKey } | Select-Object -First 1
      if ($null -ne $Primary) {
        Set-Content -Path $LlamaSelectedFile -Value $Primary.Folder
      }
      return
    }

    if ($SelectionText -eq "s") {
      Invoke-WaitForUserAction -Label "llama-server" -Check {
        -not [string]::IsNullOrWhiteSpace((Find-InstalledLlamaServer))
      } -ExpectedResult "llama-server is available on PATH or under native/llama-runtimes"
      Add-Summary "OK: llama-server"
      return
    }

    $Tokens = $SelectionText -split "[,\s]+" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
    foreach ($Token in $Tokens) {
      [int]$ParsedIndex = 0
      if (([int]::TryParse($Token, [ref]$ParsedIndex)) -and ($ParsedIndex -ge 1) -and ($ParsedIndex -le $Definitions.Count)) {
        $SelectedKey = $Definitions[$ParsedIndex - 1].Key
        if ($SelectedKeys.Contains($SelectedKey)) {
          [void]$SelectedKeys.Remove($SelectedKey)
        } else {
          [void]$SelectedKeys.Add($SelectedKey)
        }
      } else {
        Write-Host "Unknown llama.cpp runtime selection: $Token"
      }
    }
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
    [scriptblock]$Check,
    [string]$ExpectedResult
  )

  Write-Host "I will wait."
  Write-Host "Expected result: $ExpectedResult"
  Write-Host "Press Enter after the expected result is true: $Label"
  while ($true) {
    [void](Read-Host)
    if (& $Check) {
      Write-GreenCheck "$Label - done"
      return
    }
    Write-Host "Still not detected. Expected result: $ExpectedResult"
    Write-Host "Press Enter after the expected result is true, or Ctrl-C to stop."
  }
}

function Invoke-ConfirmOrWait {
  param(
    [string]$Label,
    [string]$CommandText,
    [scriptblock]$Check,
    [scriptblock]$Action,
    [string]$ExpectedResult,
    [string]$Description = "Install or download the missing requirement."
  )

  if (& $Check) {
    Write-GreenCheck "$Label - already done"
    Add-Summary "OK: $Label"
    return
  }

  while (-not (& $Check)) {
    $Choice = Read-TaskChoice -Label $Label -Description $Description -CommandText $CommandText -ExpectedResult $ExpectedResult
    if ($Choice -eq "yes") {
      try {
        & $Action
      } catch {
        Write-Host "The task failed. Here is the command or URL again:"
        Write-Host $CommandText
        Invoke-WaitForUserAction -Label $Label -Check $Check -ExpectedResult $ExpectedResult
      }
    } elseif ($Choice -eq "manual") {
      Invoke-WaitForUserAction -Label $Label -Check $Check -ExpectedResult $ExpectedResult
    } else {
      throw "Installer stopped before $Label."
    }
  }

  Write-GreenCheck "$Label - done"
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
  } -ExpectedResult "command '$CommandName' is available on PATH" `
    -Description "Install $Label so the installer can run repository setup commands."
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
  $ExpectedResult = "file exists and is non-empty at $RelativePath"

  if ((Test-Path $Target) -and ((Get-Item $Target).Length -gt 0)) {
    Write-GreenCheck "$Label at $RelativePath - already done"
    Add-Summary "OK: $Label at $RelativePath"
    return
  }

  while ((-not (Test-Path $Target)) -or ((Get-Item $Target -ErrorAction SilentlyContinue).Length -eq 0)) {
    $Choice = Read-TaskChoice `
      -Label $Label `
      -Description "Download the model file or place it manually in the expected local path." `
      -CommandText $CommandText `
      -ExpectedResult $ExpectedResult
    if ($Choice -eq "yes") {
      if ($NeedsToken) {
        Prompt-HfTokenIfNeeded
      }
      try {
        Download-Model -Url $DownloadUrl -Target $Target -AuthKind $DownloadAuthKind -AuthToken $DownloadAuthToken
      } catch {
        Write-Host "The task failed. Here is the command or URL again:"
        Write-Host $CommandText
        Invoke-WaitForUserAction -Label $Label -Check {
          (Test-Path $Target) -and ((Get-Item $Target -ErrorAction SilentlyContinue).Length -gt 0)
        } -ExpectedResult $ExpectedResult
      }
    } elseif ($Choice -eq "manual") {
      Invoke-WaitForUserAction -Label $Label -Check {
        (Test-Path $Target) -and ((Get-Item $Target -ErrorAction SilentlyContinue).Length -gt 0)
      } -ExpectedResult $ExpectedResult
    } else {
      throw "Installer stopped before $Label."
    }
  }

  Write-GreenCheck "$Label at $RelativePath - done"
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
  } -ExpectedResult "node_modules and public/vendor/litert-lm/core/wasm exist" `
    -Description "Install Node packages and regenerate the LiteRT-LM WASM vendor files."
}

function Print-InstallTasks {
  Write-Host ""
  Write-Host "Install tasks"
  Write-Host "-------------"
  Write-TaskStatus "git" { Test-Command "git" } "available" "needs install"
  Write-TaskStatus "node" { Test-Command "node" } "available" "needs install"
  Write-TaskStatus "npm" { Test-Command "npm" } "available" "needs install"
  Write-TaskStatus "go" { Test-Command "go" } "available" "needs install"
  Write-TaskStatus "curl" { Test-Command "curl" } "available" "needs install"
  Write-TaskStatus "uv" { Test-Command "uv" } "available" "needs install"
  Write-TaskStatus "litert-lm" { Test-Command "litert-lm" } "available" "needs install"
  Write-TaskStatus "llama.cpp runtime" { -not [string]::IsNullOrWhiteSpace((Find-InstalledLlamaServer)) } "available" "needs selection or manual install"
  Write-TaskStatus "npm dependencies" {
    (Test-Path (Join-Path $RepoRoot "node_modules")) -and
      (Test-Path (Join-Path $RepoRoot "public\vendor\litert-lm\core\wasm"))
  } "already installed" "needs npm install"
  Write-TaskStatus "Gemma 4 E2B web model" {
    $Path = Join-Path $RepoRoot "models\litert\gemma-4-E2B-it-web.litertlm"
    (Test-Path $Path) -and ((Get-Item $Path).Length -gt 0)
  } "downloaded" "needs download"
  Write-TaskStatus "Gemma 4 E2B native LiteRT model" {
    $Path = Join-Path $RepoRoot "models\litert\gemma-4-E2B-it.litertlm"
    (Test-Path $Path) -and ((Get-Item $Path).Length -gt 0)
  } "downloaded" "needs download"
  Write-TaskStatus "Gemma 4 E2B llama.cpp GGUF model" {
    $Path = Join-Path $RepoRoot "models\llamacpp\gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf"
    (Test-Path $Path) -and ((Get-Item $Path).Length -gt 0)
  } "downloaded" "needs download"
  Write-TaskStatus "Qwen3 embedding GGUF model" {
    $Path = Join-Path $RepoRoot "models\llamacpp\Qwen3-Embedding-0.6B-Q8_0.gguf"
    (Test-Path $Path) -and ((Get-Item $Path).Length -gt 0)
  } "downloaded" "needs download"
  Write-TaskStatus "EmbeddingGemma LiteRT embedding model" {
    $Path = Join-Path $RepoRoot "models\litert\embeddinggemma-300M_seq2048_mixed-precision.tflite"
    (Test-Path $Path) -and ((Get-Item $Path).Length -gt 0)
  } "downloaded" "needs download"
  Write-TaskPending "npm test - will run"
  Write-TaskPending "web production build - will run"
  Write-TaskPending "sidecar artifacts build - will run"
  Write-TaskPending "smoke UI - will run"
  Write-TaskPending "smoke executable sidecar - will run"
  Write-TaskPending "smoke web model - will run when the web model is present"
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

  Print-InstallTasks

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
