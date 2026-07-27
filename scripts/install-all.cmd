@echo off
cd /d D:\Devin-byok
echo Building...
go build -o devin-byok.exe .\cmd\devin-byok || exit /b 1
go build -o devin-byok-ls-wrapper.exe .\cmd\ls-wrapper || exit /b 1
echo Installing LS wrapper...
devin-byok.exe install || exit /b 1
echo.
echo Install complete. Run scripts\start-byok.cmd next.
pause
