param(
  [string]$ConfigPath = $env:RUNTIME_BACKEND_CONFIG,
  [string]$LitertModel = $env:LITERT_TEST_MODEL,
  [string]$LlamaModel = $env:LLAMA_TEST_MODEL,
  [string]$LitertBin = $env:LITERT_LM_BIN,
  [string]$LlamaBin = $env:LLAMA_SERVER_BIN,
  [string]$LitertModelId = $(if ($env:LITERT_TEST_MODEL_ID) { $env:LITERT_TEST_MODEL_ID } else { "gemma4-e2b" }),
  [string]$LlamaPrompt = $(if ($env:LLAMA_TEST_PROMPT) { $env:LLAMA_TEST_PROMPT } else { "Say ok." })
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$RepoRoot = $PSScriptRoot
$LiteRtRuntimeRoot = Join-Path $RepoRoot "native\litert-runtimes"
$LlamaRuntimeRoot = Join-Path $RepoRoot "native\llama-runtimes"
if ([string]::IsNullOrWhiteSpace($ConfigPath)) {
  $ConfigPath = Join-Path $RepoRoot "native\runtime-config\backends.json"
}
Set-Location $RepoRoot

function Quote-PowerShell {
  param([string]$Value)
  return "'" + ($Value -replace "'", "''") + "'"
}

function Format-Command {
  param([string[]]$Parts)

  return (($Parts | ForEach-Object { Quote-PowerShell $_ }) -join " ")
}

function First-ExistingOrDefault {
  param(
    [string]$Fallback,
    [string[]]$Candidates
  )

  foreach ($Candidate in $Candidates) {
    if ((Test-Path $Candidate) -and ((Get-Item $Candidate).Length -gt 0)) {
      return $Candidate
    }
  }
  return $Fallback
}

function Find-LiteRtLmInDir {
  param([string]$Directory)

  foreach ($Name in @("litert-lm.exe", "litert-lm")) {
    $Match = Get-ChildItem -Path $Directory -Filter $Name -File -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($null -ne $Match) {
      return $Match.FullName
    }
  }
  return ""
}

function Find-LiteRtLm {
  if (-not [string]::IsNullOrWhiteSpace($LitertBin)) {
    return $LitertBin
  }

  $SelectedFile = Join-Path $LiteRtRuntimeRoot ".selected"
  if (Test-Path $SelectedFile) {
    $Selected = (Get-Content $SelectedFile -Raw).Trim()
    if ($Selected) {
      $SelectedDir = Join-Path $LiteRtRuntimeRoot $Selected
      if (Test-Path $SelectedDir) {
        $Found = Find-LiteRtLmInDir -Directory $SelectedDir
        if (-not [string]::IsNullOrWhiteSpace($Found)) {
          return $Found
        }
      }
    }
  }

  if (Test-Path $LiteRtRuntimeRoot) {
    $Found = Find-LiteRtLmInDir -Directory $LiteRtRuntimeRoot
    if (-not [string]::IsNullOrWhiteSpace($Found)) {
      return $Found
    }
  }

  $Command = Get-Command "litert-lm" -ErrorAction SilentlyContinue
  if ($null -ne $Command) {
    return $Command.Source
  }
  return "litert-lm"
}

function Find-LlamaServerInDir {
  param([string]$Directory)

  foreach ($Name in @("llama-server.exe", "llama-server")) {
    $Match = Get-ChildItem -Path $Directory -Filter $Name -File -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($null -ne $Match) {
      return $Match.FullName
    }
  }
  return ""
}

function Get-LlamaRuntimeType {
  param([string]$Name)

  $Lower = $Name.ToLowerInvariant()
  if (($Lower -like "*cuda-13*") -or ($Lower -like "*cuda13*")) {
    return "cuda13"
  }
  if (($Lower -like "*cuda-12*") -or ($Lower -like "*cuda12*")) {
    return "cuda12"
  }
  if ($Lower -like "*macos*") {
    return "metal"
  }
  if ($Lower -like "*openvino*") {
    return "openvino"
  }
  if ($Lower -like "*sycl*") {
    return "sycl"
  }
  if (($Lower -like "*vulkan*") -or ($Lower -like "*hip*") -or ($Lower -like "*radeon*") -or ($Lower -like "*opencl*")) {
    return "gpu"
  }
  return "cpu"
}

function Test-LlamaUsesGpuBackend {
  param([string]$Backend)

  return @("gpu", "cuda", "cuda12", "cuda13", "metal", "vulkan", "openvino", "sycl", "npu") -contains $Backend
}

function Test-LiteRtErrorOutput {
  param([string]$OutputText)

  return $OutputText -match "(?m)(^An error occurred$|Traceback \(most recent call last\):|RuntimeError:|INVALID_ARGUMENT:|INTERNAL:|Failed to invoke|Failed to allocate|Validation error:)"
}

function Test-LiteRtResponse {
  param([string]$OutputText)

  if (Test-LiteRtErrorOutput -OutputText $OutputText) {
    return $false
  }
  return -not [string]::IsNullOrWhiteSpace($OutputText)
}

function Test-LlamaCompletionResponse {
  param([string]$ResponseText)

  try {
    $Data = $ResponseText | ConvertFrom-Json
    $Content = $Data.choices[0].message.content
    return ($Content -is [string]) -and (-not [string]::IsNullOrWhiteSpace($Content))
  } catch {
    return $false
  }
}

function Get-LlamaSpecs {
  $Specs = [System.Collections.Generic.List[object]]::new()
  $Seen = [System.Collections.Generic.HashSet[string]]::new()

  if (Test-Path $LlamaRuntimeRoot) {
    foreach ($Directory in Get-ChildItem -Path $LlamaRuntimeRoot -Directory -ErrorAction SilentlyContinue) {
      if ($Directory.Name.StartsWith(".")) {
        continue
      }
      $Executable = Find-LlamaServerInDir -Directory $Directory.FullName
      if ([string]::IsNullOrWhiteSpace($Executable)) {
        continue
      }
      $Backend = Get-LlamaRuntimeType -Name $Directory.Name
      if ($Seen.Add($Backend)) {
        $Specs.Add([pscustomobject]@{
          Backend = $Backend
          Executable = $Executable
          Runtime = $Directory.Name
        }) | Out-Null
      }
    }
  }

  if ($Specs.Count -gt 0) {
    return $Specs
  }

  $ExecutableFallback = $LlamaBin
  if ([string]::IsNullOrWhiteSpace($ExecutableFallback)) {
    $Command = Get-Command "llama-server" -ErrorAction SilentlyContinue
    if ($null -ne $Command) {
      $ExecutableFallback = $Command.Source
    } else {
      $ExecutableFallback = "llama-server"
    }
  }
  foreach ($Backend in @("cpu", "gpu", "metal", "openvino", "cuda13", "cuda12", "sycl")) {
    $Specs.Add([pscustomobject]@{
      Backend = $Backend
      Executable = $ExecutableFallback
      Runtime = "candidate"
    }) | Out-Null
  }
  return $Specs
}

function ConvertTo-Hashtable {
  param([object]$Value)

  if ($null -eq $Value) {
    return $null
  }
  if ($Value -is [System.Collections.IDictionary]) {
    $Hash = @{}
    foreach ($Key in $Value.Keys) {
      $Hash[$Key] = ConvertTo-Hashtable $Value[$Key]
    }
    return $Hash
  }
  if (($Value -is [System.Collections.IEnumerable]) -and ($Value -isnot [string])) {
    $Items = @()
    foreach ($Item in $Value) {
      $Items += ConvertTo-Hashtable $Item
    }
    return $Items
  }
  if ($Value -is [pscustomobject]) {
    $Hash = @{}
    foreach ($Property in $Value.PSObject.Properties) {
      $Hash[$Property.Name] = ConvertTo-Hashtable $Property.Value
    }
    return $Hash
  }
  return $Value
}

function Set-BackendResult {
  param(
    [string]$RuntimeName,
    [string]$Backend,
    [bool]$Working,
    [string]$CommandText,
    [string]$ModelPath,
    [string]$OutputText
  )

  $Config = @{ version = 1; runtimes = @{} }
  if (Test-Path $ConfigPath) {
    $Raw = (Get-Content $ConfigPath -Raw).Trim()
    if (-not [string]::IsNullOrWhiteSpace($Raw)) {
      $Config = ConvertTo-Hashtable ($Raw | ConvertFrom-Json)
    }
  }
  if (-not ($Config.ContainsKey("runtimes")) -or ($null -eq $Config["runtimes"])) {
    $Config["runtimes"] = @{}
  }
  if (-not ($Config["runtimes"].ContainsKey($RuntimeName))) {
    $Config["runtimes"][$RuntimeName] = @{}
  }
  if ($OutputText.Length -gt 4000) {
    $OutputText = $OutputText.Substring($OutputText.Length - 4000)
  }

  $UpdatedAt = [DateTimeOffset]::UtcNow.ToString("o")
  $Config["version"] = 1
  $Config["updatedAt"] = $UpdatedAt
  $Config["runtimes"][$RuntimeName][$Backend] = @{
    working = $Working
    command = $CommandText
    model = $ModelPath
    testedAt = $UpdatedAt
    output = $OutputText
  }

  $Directory = Split-Path -Parent $ConfigPath
  New-Item -ItemType Directory -Force -Path $Directory | Out-Null
  $Config | ConvertTo-Json -Depth 8 | Set-Content -Path $ConfigPath -Encoding utf8
}

function Invoke-LiteRtBackendProbe {
  param(
    [string]$Executable,
    [string]$ModelPath,
    [string]$ModelId,
    [string]$Backend,
    [string]$Prompt = $(if ($env:LITERT_TEST_PROMPT) { $env:LITERT_TEST_PROMPT } else { "Say ok." })
  )

  $MaxTokens = if ($env:LITERT_TEST_MAX_NUM_TOKENS) { $env:LITERT_TEST_MAX_NUM_TOKENS } else { "4096" }
  Write-Output ("Runtime command: " + (Format-Command @($Executable, "run", $ModelId, "--backend=$Backend", "--max-num-tokens=$MaxTokens", "--prompt=$Prompt")))
  $ListOutput = & $Executable list 2>$null
  $HasModel = $false
  if ($LASTEXITCODE -eq 0) {
    foreach ($Line in $ListOutput) {
      $Fields = $Line -split "\s+"
      if (($Fields.Count -gt 0) -and ($Fields[0] -eq $ModelId)) {
        $HasModel = $true
      }
    }
  }
  if (-not $HasModel) {
    & $Executable import $ModelPath $ModelId
    if ($LASTEXITCODE -ne 0) {
      throw "litert-lm import failed with exit code $LASTEXITCODE"
    }
  }
  $RunOutput = & $Executable run $ModelId "--backend=$Backend" "--max-num-tokens=$MaxTokens" "--prompt=$Prompt" 2>&1 | Out-String
  $RunExitCode = $LASTEXITCODE
  if (-not [string]::IsNullOrWhiteSpace($RunOutput)) {
    Write-Output $RunOutput.TrimEnd()
  }
  if ($RunExitCode -ne 0) {
    throw "litert-lm run failed with exit code $RunExitCode"
  }
  if (-not (Test-LiteRtResponse -OutputText $RunOutput)) {
    throw "litert-lm run did not return a usable model response."
  }
}

function Invoke-LlamaBackendProbe {
  param(
    [string]$Executable,
    [string]$ModelPath,
    [string]$Backend,
    [string]$Prompt = $LlamaPrompt
  )

  $Port = if ($env:LLAMA_TEST_PORT) { [int]$env:LLAMA_TEST_PORT } else { Get-Random -Minimum 28000 -Maximum 38000 }
  $MaxTokens = if ($env:LLAMA_TEST_MAX_TOKENS) { [int]$env:LLAMA_TEST_MAX_TOKENS } else { 128 }
  $TimeoutSeconds = if ($env:LLAMA_TEST_TIMEOUT_SECONDS) { [int]$env:LLAMA_TEST_TIMEOUT_SECONDS } else { 180 }
  $RequestTimeoutSeconds = if ($env:LLAMA_TEST_REQUEST_TIMEOUT_SECONDS) { [int]$env:LLAMA_TEST_REQUEST_TIMEOUT_SECONDS } else { 120 }
  $StdoutLog = Join-Path ([System.IO.Path]::GetTempPath()) "llama-backend-$Backend-$([Guid]::NewGuid().ToString('N')).stdout.log"
  $StderrLog = Join-Path ([System.IO.Path]::GetTempPath()) "llama-backend-$Backend-$([Guid]::NewGuid().ToString('N')).stderr.log"
  $Arguments = @("-m", $ModelPath, "--alias", "configure-test", "--host", "127.0.0.1", "--port", [string]$Port, "--reasoning", "off")
  if (Test-LlamaUsesGpuBackend -Backend $Backend) {
    $Arguments += @("--n-gpu-layers", "999")
  }
  Write-Output ("Runtime command: " + (Format-Command (@($Executable) + $Arguments)))

  $Process = Start-Process -FilePath $Executable -ArgumentList $Arguments -PassThru -NoNewWindow -RedirectStandardOutput $StdoutLog -RedirectStandardError $StderrLog
  try {
    $Deadline = [DateTimeOffset]::UtcNow.AddSeconds($TimeoutSeconds)
    $LastResponse = ""
    $RequestBody = @{
      model = "configure-test"
      messages = @(@{
        role = "user"
        content = $Prompt
      })
      max_tokens = $MaxTokens
      temperature = 0
      stream = $false
    } | ConvertTo-Json -Depth 8 -Compress

    while ([DateTimeOffset]::UtcNow -lt $Deadline) {
      if ($Process.HasExited) {
        $Output = ""
        if (Test-Path $StdoutLog) {
          $Output += Get-Content $StdoutLog -Raw
        }
        if (Test-Path $StderrLog) {
          $Output += Get-Content $StderrLog -Raw
        }
        throw "llama-server exited before returning a chat completion. $Output"
      }

      try {
        $Response = Invoke-WebRequest `
          -UseBasicParsing `
          -TimeoutSec $RequestTimeoutSeconds `
          -Method Post `
          -ContentType "application/json" `
          -Body $RequestBody `
          -Uri "http://127.0.0.1:$Port/v1/chat/completions"
        $LastResponse = $Response.Content
        if (Test-LlamaCompletionResponse -ResponseText $LastResponse) {
          Write-Output "Completion response: $LastResponse"
          return
        }
      } catch {
        $LastResponse = ($_ | Out-String)
      }
      Start-Sleep -Seconds 1
    }
    $TimedOutOutput = ""
    if (Test-Path $StdoutLog) {
      $TimedOutOutput += Get-Content $StdoutLog -Raw
    }
    if (Test-Path $StderrLog) {
      $TimedOutOutput += Get-Content $StderrLog -Raw
    }
    throw "llama-server did not return a chat completion before timeout. Last response: $LastResponse $TimedOutOutput"
  } finally {
    if (-not $Process.HasExited) {
      Stop-Process -Id $Process.Id -Force -ErrorAction SilentlyContinue
      $Process.WaitForExit()
    }
    Remove-Item -Force -ErrorAction SilentlyContinue $StdoutLog, $StderrLog
  }
}

function Read-TestCommand {
  param([string]$DefaultCommand)

  Write-Host $DefaultCommand
  $Override = Read-Host "Edit command and press Enter, or press Enter to run default"
  if ([string]::IsNullOrWhiteSpace($Override)) {
    return $DefaultCommand
  }
  return $Override
}

function Invoke-BackendTest {
  param(
    [string]$DisplayRuntime,
    [string]$RuntimeName,
    [string]$Backend,
    [string]$ModelPath,
    [string]$DefaultCommand
  )

  Write-Host ""
  Write-Host "we are testing $DisplayRuntime on $Backend backend, here is command I am going to use"
  $CommandText = Read-TestCommand -DefaultCommand $DefaultCommand

  while ($true) {
    $OutputText = ""
    $ExitCode = 0
    try {
      $global:LASTEXITCODE = 0
      $OutputText = Invoke-Expression $CommandText 2>&1 | Out-String
      if ($LASTEXITCODE -ne 0) {
        $ExitCode = $LASTEXITCODE
      }
    } catch {
      $OutputText = ($_ | Out-String)
      $ExitCode = 1
    }

    if ($ExitCode -eq 0) {
      Write-Host "Backend $RuntimeName/$Backend worked."
      Set-BackendResult -RuntimeName $RuntimeName -Backend $Backend -Working $true -CommandText $CommandText -ModelPath $ModelPath -OutputText $OutputText
      return
    }

    Write-Host "Backend $RuntimeName/$Backend failed with exit code $ExitCode."
    Write-Host "Command output:"
    Write-Host $OutputText

    while ($true) {
      $Choice = Read-Host "Choose [R] retry with edited command, [N] mark runtime backend combo as not working"
      if ($Choice -match "^(r|retry)$") {
        Write-Host "Current command:"
        Write-Host $CommandText
        $CommandText = Read-TestCommand -DefaultCommand $CommandText
        break
      }
      if ($Choice -match "^(n|no|mark)$") {
        Set-BackendResult -RuntimeName $RuntimeName -Backend $Backend -Working $false -CommandText $CommandText -ModelPath $ModelPath -OutputText $OutputText
        Write-Host "Backend $RuntimeName/$Backend marked not working."
        return
      }
      Write-Host "Choose R or N."
    }
  }
}

if ([string]::IsNullOrWhiteSpace($LitertModel)) {
  $LitertModel = First-ExistingOrDefault `
    -Fallback (Join-Path $RepoRoot "models\litert\main\gemma-4-E2B-it.litertlm") `
    -Candidates @((Join-Path $RepoRoot "models\litert\main\gemma-4-E2B-it.litertlm"))
}
if ([string]::IsNullOrWhiteSpace($LlamaModel)) {
  $LlamaModel = First-ExistingOrDefault `
    -Fallback (Join-Path $RepoRoot "models\llamacpp\main\Qwen3.5-0.8B-UD-Q8_K_XL.gguf") `
    -Candidates @(
      (Join-Path $RepoRoot "models\llamacpp\main\Qwen3.5-0.8B-UD-Q8_K_XL.gguf"),
      (Join-Path $RepoRoot "models\llamacpp\main\Qwen3.5-2B-IQ4_NL.gguf"),
      (Join-Path $RepoRoot "models\llamacpp\main\gemma-4-E2B-it-qat-UD-Q4_K_XL.gguf")
    )
}

$LiteRtExecutable = Find-LiteRtLm
Write-Host "Backend test results will be written to $ConfigPath"
Write-Host "LiteRT test model: $LitertModel"
Write-Host "llama.cpp test model: $LlamaModel"

foreach ($Backend in @("cpu", "gpu", "npu")) {
  $CommandText = "Invoke-LiteRtBackendProbe -Executable $(Quote-PowerShell $LiteRtExecutable) -ModelPath $(Quote-PowerShell $LitertModel) -ModelId $(Quote-PowerShell $LitertModelId) -Backend $(Quote-PowerShell $Backend)"
  Invoke-BackendTest -DisplayRuntime "liteRT" -RuntimeName "litert" -Backend $Backend -ModelPath $LitertModel -DefaultCommand $CommandText
}

foreach ($Spec in Get-LlamaSpecs) {
  $CommandText = "Invoke-LlamaBackendProbe -Executable $(Quote-PowerShell $Spec.Executable) -ModelPath $(Quote-PowerShell $LlamaModel) -Backend $(Quote-PowerShell $Spec.Backend)"
  Invoke-BackendTest -DisplayRuntime "llama" -RuntimeName "llamacpp" -Backend $Spec.Backend -ModelPath $LlamaModel -DefaultCommand $CommandText
}

Write-Host ""
Write-Host "Configuration complete. Results are in $ConfigPath"
