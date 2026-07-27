@echo off
echo Stopping devin-byok serve ...
taskkill /FI "IMAGENAME eq devin-byok.exe" /F >nul 2>&1
echo done
