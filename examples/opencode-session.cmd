@echo off
setlocal

set "LISTEN=127.0.0.1:18777"
set "UPSTREAM=https://opencode.ai/zen/go/v1"
set "LOG_LEVEL=info"

opencode-session.exe --listen "%LISTEN%" --upstream "%UPSTREAM%" --log-level "%LOG_LEVEL%"
