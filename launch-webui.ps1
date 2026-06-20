param(
  [switch]$Inline,
  [string]$WebHost = "",
  [int]$Port = 0,
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$ExtraArgs = @()
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
$RepoRoot = $PSScriptRoot

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
  if ($WebHost -eq "") {
    $WebHost = if ($env:WEBUI_HOST) { $env:WEBUI_HOST } else { "127.0.0.1" }
  }

  if ($Port -eq 0) {
    $Port = if ($env:WEBUI_PORT) { [int]$env:WEBUI_PORT } else { 5173 }
  }

  $ProcessArgs = @(
    "-NoExit",
    "-NoProfile",
    "-ExecutionPolicy",
    "Bypass",
    "-File",
    $PSCommandPath,
    "-Inline",
    "-WebHost",
    $WebHost,
    "-Port",
    [string]$Port
  )
  $ProcessArgs += $ExtraArgs

  Start-LiteRTTerminal `
    -Title "LiteRT Web UI" `
    -ProcessArgs $ProcessArgs `
    -WorkingDirectory $RepoRoot `
    -EnvironmentNames @("WEBUI_HOST", "WEBUI_PORT")
  Write-Host "Opened LiteRT Web UI in a separate terminal."
  exit 0
}

if ($WebHost -eq "") {
  $WebHost = if ($env:WEBUI_HOST) { $env:WEBUI_HOST } else { "127.0.0.1" }
}

if ($Port -eq 0) {
  $Port = if ($env:WEBUI_PORT) { [int]$env:WEBUI_PORT } else { 5173 }
}

Set-Location $RepoRoot
$BunArgs = @("run", "dev", "--host", $WebHost, "--port", [string]$Port) + $ExtraArgs
& bun @BunArgs
exit $LASTEXITCODE
