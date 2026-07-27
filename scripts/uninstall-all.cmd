@echo off
cd /d D:\Devin-byok
call scripts\stop-byok.cmd
devin-byok.exe uninstall
echo Uninstall complete. Fully restart Devin.
pause
