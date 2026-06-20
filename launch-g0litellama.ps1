param(
  [switch]$Inline,
  [switch]$Headless,
  [switch]$Tui,
  [string]$G0LiteLLaMaBin = "",
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$ExtraArgs = @()
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$RepoRoot = $PSScriptRoot
$LiteRtRuntimeRoot = Join-Path $RepoRoot "G0LiteLLaMa\litert-runtimes"
$LiteRtSelectedFile = Join-Path $LiteRtRuntimeRoot ".selected"
$LlamaRuntimeRoot = Join-Path $RepoRoot "G0LiteLLaMa\llama-runtimes"
$LlamaSelectedFile = Join-Path $LlamaRuntimeRoot ".selected"
$G0LiteLLaMaArgs = @()

function Get-PowerShellExe {
  if (Get-Command pwsh -ErrorAction SilentlyContinue) {
    return (Get-Command pwsh).Source
  }

  return (Get-Command powershell).Source
}

function ConvertTo-PosixShellArgument {
  param([AllowNull()][string]$Value)

  if ($null -eq $Value) {
    return "''"
  }

  $SingleQuote = [string][char]39
  $EscapedValue = $Value.Replace($SingleQuote, "$SingleQuote`"$SingleQuote`"$SingleQuote")
  return "$SingleQuote$EscapedValue$SingleQuote"
}

function ConvertTo-AppleScriptString {
  param([string]$Value)

  return $Value.Replace('\', '\\').Replace('"', '\"')
}

function Get-LiteRTMacTerminalApp {
  $Candidate = if ($env:LITERT_TERMINAL_APP) {
    $env:LITERT_TERMINAL_APP
  } elseif ($env:TERM_PROGRAM) {
    $env:TERM_PROGRAM
  } else {
    ""
  }

  switch ($Candidate) {
    "Ghostty" { return "Ghostty" }
    "ghostty" { return "Ghostty" }
    "Ghostty.app" { return "Ghostty" }
    "Apple_Terminal" { return "Terminal" }
    "Terminal" { return "Terminal" }
    "Terminal.app" { return "Terminal" }
    "" { return "Terminal" }
    default { return $Candidate }
  }
}

function Start-LiteRTGhostty {
  param(
    [string]$Command,
    [string]$WorkingDirectory
  )

  $EscapedCommand = ConvertTo-AppleScriptString $Command
  $EscapedWorkingDirectory = ConvertTo-AppleScriptString $WorkingDirectory
  & osascript `
    -e 'tell application "Ghostty"' `
    -e 'activate' `
    -e 'set cfg to new surface configuration' `
    -e "set initial working directory of cfg to `"$EscapedWorkingDirectory`"" `
    -e "set initial input of cfg to `"$EscapedCommand`" & linefeed" `
    -e 'set win to new window with configuration cfg' `
    -e 'end tell' | Out-Null
}

function Start-LiteRTAppleTerminal {
  param([string]$Command)

  $EscapedCommand = ConvertTo-AppleScriptString $Command
  & osascript `
    -e 'tell application "Terminal"' `
    -e 'activate' `
    -e "do script `"$EscapedCommand`"" `
    -e 'end tell' | Out-Null
}

function ConvertTo-WindowsCommandArgument {
  param([AllowNull()][string]$Value)

  if ($null -eq $Value) {
    return '""'
  }

  return '"' + $Value.Replace('"', '\"') + '"'
}

function Join-WindowsCommandArguments {
  param([string[]]$Arguments)

  $QuotedArguments = @()
  foreach ($Argument in $Arguments) {
    $QuotedArguments += ConvertTo-WindowsCommandArgument $Argument
  }

  return ($QuotedArguments -join " ")
}

function Get-LiteRTPlatform {
  if ([System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::OSX)) {
    return "macos"
  }
  if ([System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Linux)) {
    return "linux"
  }

  return "windows"
}

function New-TerminalShellCommand {
  param(
    [string]$PowerShellExe,
    [string[]]$ProcessArgs,
    [string]$WorkingDirectory,
    [string[]]$EnvironmentNames = @()
  )

  $EnvironmentAssignments = @()
  foreach ($Name in $EnvironmentNames) {
    $Value = [Environment]::GetEnvironmentVariable($Name)
    if (-not [string]::IsNullOrWhiteSpace($Value)) {
      $EnvironmentAssignments += "$Name=$(ConvertTo-PosixShellArgument $Value)"
    }
  }

  $CommandParts = @(
    "cd",
    (ConvertTo-PosixShellArgument $WorkingDirectory),
    "&&"
  ) + $EnvironmentAssignments + @(
    (ConvertTo-PosixShellArgument $PowerShellExe)
  )

  foreach ($Argument in $ProcessArgs) {
    $CommandParts += ConvertTo-PosixShellArgument $Argument
  }

  return ($CommandParts -join " ")
}

function Start-LiteRTTerminal {
  param(
    [string]$Title,
    [string[]]$ProcessArgs,
    [string]$WorkingDirectory,
    [string[]]$EnvironmentNames = @()
  )

  $PowerShellExe = Get-PowerShellExe
  $PlatformName = Get-LiteRTPlatform

  if ($PlatformName -eq "macos") {
    $Command = New-TerminalShellCommand `
      -PowerShellExe $PowerShellExe `
      -ProcessArgs $ProcessArgs `
      -WorkingDirectory $WorkingDirectory `
      -EnvironmentNames $EnvironmentNames

    if ((Get-LiteRTMacTerminalApp) -eq "Ghostty") {
      Start-LiteRTGhostty -Command $Command -WorkingDirectory $WorkingDirectory
    } else {
      Start-LiteRTAppleTerminal -Command $Command
    }
    return
  }

  if ($PlatformName -eq "linux") {
    $Command = New-TerminalShellCommand `
      -PowerShellExe $PowerShellExe `
      -ProcessArgs $ProcessArgs `
      -WorkingDirectory $WorkingDirectory `
      -EnvironmentNames $EnvironmentNames

    if (Get-Command gnome-terminal -ErrorAction SilentlyContinue) {
      Start-Process -FilePath "gnome-terminal" -ArgumentList @("--title=$Title", "--", "bash", "-lc", "$Command; exec bash") -WorkingDirectory $WorkingDirectory | Out-Null
      return
    }
    if (Get-Command konsole -ErrorAction SilentlyContinue) {
      Start-Process -FilePath "konsole" -ArgumentList @("-p", "tabtitle=$Title", "-e", "bash", "-lc", "$Command; exec bash") -WorkingDirectory $WorkingDirectory | Out-Null
      return
    }
    if (Get-Command xterm -ErrorAction SilentlyContinue) {
      Start-Process -FilePath "xterm" -ArgumentList @("-T", $Title, "-e", "bash", "-lc", "$Command; exec bash") -WorkingDirectory $WorkingDirectory | Out-Null
      return
    }

    throw "No supported terminal launcher found. Run this command manually: $Command"
  }

  if (Get-Command wt.exe -ErrorAction SilentlyContinue) {
    $WindowsTerminalArgs = @(
      "--window",
      "new",
      "new-tab",
      "--title",
      $Title,
      "-d",
      $WorkingDirectory,
      $PowerShellExe
    ) + $ProcessArgs
    Start-Process -FilePath "wt.exe" -ArgumentList (Join-WindowsCommandArguments $WindowsTerminalArgs) -WorkingDirectory $WorkingDirectory | Out-Null
    return
  }

  Start-Process -FilePath $PowerShellExe -ArgumentList $ProcessArgs -WorkingDirectory $WorkingDirectory | Out-Null
}

if (-not $Inline) {
  if ($Headless -and $Tui) {
    throw "Use either -Headless or -Tui, not both."
  }

  $ProcessArgs = @(
    "-NoExit",
    "-NoProfile",
    "-ExecutionPolicy",
    "Bypass",
    "-File",
    $PSCommandPath,
    "-Inline"
  )
  if ($Headless) {
    $ProcessArgs += "-Headless"
  } else {
    $ProcessArgs += "-Tui"
  }
  if (-not [string]::IsNullOrWhiteSpace($G0LiteLLaMaBin)) {
    $ProcessArgs += @("-G0LiteLLaMaBin", $G0LiteLLaMaBin)
  }
  $ProcessArgs += $ExtraArgs

  Start-LiteRTTerminal `
    -Title "G0LiteLLaMa TUI" `
    -ProcessArgs $ProcessArgs `
    -WorkingDirectory $RepoRoot `
    -EnvironmentNames @(
      "G0LITELLAMA_BIN",
      "G0LITELLAMA_ADDR",
      "G0LITELLAMA_UPSTREAM",
      "LITERT_LM_BIN",
      "G0LITELLAMA_RUNTIME_HOST",
      "G0LITELLAMA_RUNTIME_PORT",
      "MODEL_FILE",
      "MODEL_ID",
      "G0LITELLAMA_LAUNCH_RUNTIME",
      "G0LITELLAMA_IMPORT_MODEL",
      "G0LITELLAMA_RUNTIME_VERBOSE",
      "G0LITELLAMA_HEADLESS",
      "LITERT_RUNTIME",
      "LLAMA_RUNTIME",
      "LLAMA_SERVER_BIN"
    )
  Write-Host "Opened G0LiteLLaMa TUI in a separate terminal."
  exit 0
}

function Get-EnvValue {
  param([string]$Name)
  return [Environment]::GetEnvironmentVariable($Name)
}

function Add-ValueFlag {
  param(
    [string]$EnvName,
    [string]$FlagName
  )

  $Value = Get-EnvValue $EnvName
  if (-not [string]::IsNullOrWhiteSpace($Value)) {
    $script:G0LiteLLaMaArgs += @($FlagName, $Value)
  }
}

function Add-BoolFlag {
  param(
    [string]$EnvName,
    [string]$FlagName
  )

  $Value = Get-EnvValue $EnvName
  if (-not [string]::IsNullOrWhiteSpace($Value)) {
    $script:G0LiteLLaMaArgs += "$FlagName=$Value"
  }
}

function Get-DefaultG0LiteLLaMaBin {
  $Architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
  $ArchSuffix = switch ($Architecture.ToString()) {
    "Arm64" { "arm64" }
    "X64" { "amd64" }
    default { "" }
  }

  if ($ArchSuffix -eq "") {
    return ""
  }

  return Join-Path $RepoRoot "G0LiteLLaMa\dist\g0litellama-windows-$ArchSuffix\g0litellama.exe"
}

function Get-LiteRtExecutableNames {
  if ([System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)) {
    return @("litert-lm.exe")
  }

  return @("litert-lm")
}

function Find-LiteRtLmBin {
  if ($env:LITERT_LM_BIN -and (Test-Path $env:LITERT_LM_BIN)) {
    return $env:LITERT_LM_BIN
  }

  if ($env:LITERT_RUNTIME) {
    $RuntimeDir = Join-Path $LiteRtRuntimeRoot $env:LITERT_RUNTIME
    if (Test-Path $RuntimeDir) {
      foreach ($Name in Get-LiteRtExecutableNames) {
        $Match = Get-ChildItem -Path $RuntimeDir -Filter $Name -File -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($null -ne $Match) {
          return $Match.FullName
        }
      }
    }
  }

  if (Test-Path $LiteRtSelectedFile) {
    $RuntimeName = (Get-Content $LiteRtSelectedFile -Raw).Trim()
    if ($RuntimeName) {
      $RuntimeDir = Join-Path $LiteRtRuntimeRoot $RuntimeName
      if (Test-Path $RuntimeDir) {
        foreach ($Name in Get-LiteRtExecutableNames) {
          $Match = Get-ChildItem -Path $RuntimeDir -Filter $Name -File -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
          if ($null -ne $Match) {
            return $Match.FullName
          }
        }
      }
    }
  }

  if (Test-Path $LiteRtRuntimeRoot) {
    foreach ($Name in Get-LiteRtExecutableNames) {
      $Match = Get-ChildItem -Path $LiteRtRuntimeRoot -Filter $Name -File -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
      if ($null -ne $Match) {
        return $Match.FullName
      }
    }
  }

  return ""
}

function Add-LiteRtRuntimeToPath {
  $LiteRtLmBin = Find-LiteRtLmBin
  if (-not [string]::IsNullOrWhiteSpace($LiteRtLmBin)) {
    $env:LITERT_LM_BIN = $LiteRtLmBin
    $Directory = Split-Path -Parent $LiteRtLmBin
    $env:PATH = "$Directory$([IO.Path]::PathSeparator)$env:PATH"
  }
}

function Get-LlamaExecutableNames {
  if ([System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)) {
    return @("llama-server.exe")
  }

  return @("llama-server")
}

function Find-LlamaServerBin {
  if ($env:LLAMA_SERVER_BIN -and (Test-Path $env:LLAMA_SERVER_BIN)) {
    return $env:LLAMA_SERVER_BIN
  }

  if ($env:LLAMA_RUNTIME) {
    $RuntimeDir = Join-Path $LlamaRuntimeRoot $env:LLAMA_RUNTIME
    if (Test-Path $RuntimeDir) {
      foreach ($Name in Get-LlamaExecutableNames) {
        $Match = Get-ChildItem -Path $RuntimeDir -Filter $Name -File -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($null -ne $Match) {
          return $Match.FullName
        }
      }
    }
  }

  if (Test-Path $LlamaSelectedFile) {
    $RuntimeName = (Get-Content $LlamaSelectedFile -Raw).Trim()
    if ($RuntimeName) {
      $RuntimeDir = Join-Path $LlamaRuntimeRoot $RuntimeName
      if (Test-Path $RuntimeDir) {
        foreach ($Name in Get-LlamaExecutableNames) {
          $Match = Get-ChildItem -Path $RuntimeDir -Filter $Name -File -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
          if ($null -ne $Match) {
            return $Match.FullName
          }
        }
      }
    }
  }

  if (Test-Path $LlamaRuntimeRoot) {
    foreach ($Name in Get-LlamaExecutableNames) {
      $Match = Get-ChildItem -Path $LlamaRuntimeRoot -Filter $Name -File -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
      if ($null -ne $Match) {
        return $Match.FullName
      }
    }
  }

  return ""
}

function Add-LlamaRuntimeToPath {
  $LlamaServerBin = Find-LlamaServerBin
  if (-not [string]::IsNullOrWhiteSpace($LlamaServerBin)) {
    $Directory = Split-Path -Parent $LlamaServerBin
    $env:PATH = "$Directory$([IO.Path]::PathSeparator)$env:PATH"
  }
}

if ($G0LiteLLaMaBin -eq "") {
  $G0LiteLLaMaBin = if ($env:G0LITELLAMA_BIN) { $env:G0LITELLAMA_BIN } else { Get-DefaultG0LiteLLaMaBin }
}

if ((-not $Tui) -and ($Headless -or ($env:G0LITELLAMA_HEADLESS -match "^(1|true|yes)$"))) {
  $G0LiteLLaMaArgs += "--headless"
}

Add-LlamaRuntimeToPath
Add-LiteRtRuntimeToPath

Add-ValueFlag "G0LITELLAMA_ADDR" "-addr"
Add-ValueFlag "G0LITELLAMA_UPSTREAM" "-upstream"
Add-ValueFlag "LITERT_LM_BIN" "-runtime-exe"
Add-ValueFlag "G0LITELLAMA_RUNTIME_HOST" "-runtime-host"
Add-ValueFlag "G0LITELLAMA_RUNTIME_PORT" "-runtime-port"
Add-ValueFlag "MODEL_FILE" "-model-file"
Add-ValueFlag "MODEL_ID" "-model-id"
Add-BoolFlag "G0LITELLAMA_LAUNCH_RUNTIME" "-launch-runtime"
Add-BoolFlag "G0LITELLAMA_IMPORT_MODEL" "-import-model"
Add-BoolFlag "G0LITELLAMA_RUNTIME_VERBOSE" "-runtime-verbose"

if (($G0LiteLLaMaBin -ne "") -and (Test-Path $G0LiteLLaMaBin)) {
  & $G0LiteLLaMaBin @G0LiteLLaMaArgs @ExtraArgs
  exit $LASTEXITCODE
}

Push-Location (Join-Path $RepoRoot "G0LiteLLaMa")
try {
  & go run .\cmd\g0litellama @G0LiteLLaMaArgs @ExtraArgs
  exit $LASTEXITCODE
} finally {
  Pop-Location
}
