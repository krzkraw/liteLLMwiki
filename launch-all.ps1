param(
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$SidecarArgs = @()
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$RepoRoot = $PSScriptRoot
$WebUiScript = Join-Path $RepoRoot "launch-webui.ps1"
$SidecarScript = Join-Path $RepoRoot "launch-sidecar.ps1"

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
    $EscapedCommand = ConvertTo-AppleScriptString $Command
    & osascript `
      -e 'tell application "Terminal"' `
      -e 'activate' `
      -e "do script `"$EscapedCommand`"" `
      -e 'end tell' | Out-Null
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
      "new-window",
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

$WebUiArgs = @(
  "-NoExit",
  "-NoProfile",
  "-ExecutionPolicy",
  "Bypass",
  "-File",
  $WebUiScript,
  "-Inline"
)

$SidecarProcessArgs = @(
  "-NoExit",
  "-NoProfile",
  "-ExecutionPolicy",
  "Bypass",
  "-File",
  $SidecarScript,
  "-Inline",
  "-Tui"
) + $SidecarArgs

Start-LiteRTTerminal `
  -Title "LiteRT Web UI" `
  -ProcessArgs $WebUiArgs `
  -WorkingDirectory $RepoRoot `
  -EnvironmentNames @("WEBUI_HOST", "WEBUI_PORT")

Start-LiteRTTerminal `
  -Title "LiteRT Sidecar TUI" `
  -ProcessArgs $SidecarProcessArgs `
  -WorkingDirectory $RepoRoot `
  -EnvironmentNames @(
    "SIDECAR_BIN",
    "SIDECAR_ADDR",
    "SIDECAR_UPSTREAM",
    "LITERT_LM_BIN",
    "SIDECAR_RUNTIME_HOST",
    "SIDECAR_RUNTIME_PORT",
    "MODEL_FILE",
    "MODEL_ID",
    "SIDECAR_LAUNCH_RUNTIME",
    "SIDECAR_IMPORT_MODEL",
    "SIDECAR_RUNTIME_VERBOSE",
    "LLAMA_RUNTIME",
    "LLAMA_SERVER_BIN"
  )

Write-Host "Opened LiteRT Web UI in a separate terminal."
Write-Host "Opened LiteRT Sidecar TUI in a separate terminal."
