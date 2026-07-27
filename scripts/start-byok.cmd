@echo off
setlocal
cd /d D:\Devin-byok
echo [1/3] check binary...
if not exist devin-byok.exe (
  echo building devin-byok.exe ...
  go build -o devin-byok.exe .\cmd\devin-byok
  if errorlevel 1 exit /b 1
)
echo [2/3] ensure LS wrapper installed...
if not exist "%APPDATA%\devin-byok\ls-wrapper-install.json" (
  call devin-byok.exe install
  if errorlevel 1 exit /b 1
)
echo [3/3] start local-api on 127.0.0.1:8787 ...
start "devin-byok-serve" /MIN "%CD%\devin-byok.exe" serve
timeout /t 2 >nul
curl -s http://127.0.0.1:8787/healthz
echo.
echo OK. Fully restart Devin, pick BYOK model, send message.
echo Keep minimized devin-byok-serve running.
pause
