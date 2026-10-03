$ErrorActionPreference = "Stop"

Set-Location (Split-Path -Parent $PSScriptRoot)

go test ./...
go vet ./...
go build -trimpath -o opencode-session.exe .

Write-Host "All checks passed. Built .\opencode-session.exe"
