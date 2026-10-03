$ErrorActionPreference = "Stop"

$Binary = Join-Path $env:LOCALAPPDATA "opencode-session\opencode-session.exe"
$Arguments = "--listen 127.0.0.1:18777 --upstream https://opencode.ai/zen/go/v1 --log-level info"
$Action = New-ScheduledTaskAction -Execute $Binary -Argument $Arguments
$Trigger = New-ScheduledTaskTrigger -AtLogOn
$Settings = New-ScheduledTaskSettingsSet -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)

Register-ScheduledTask `
  -TaskName "opencode-session" `
  -Action $Action `
  -Trigger $Trigger `
  -Settings $Settings `
  -Description "Local OpenCode Go session header proxy for Codex++"
