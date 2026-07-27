$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root
$rsrc = Join-Path (go env GOPATH) "bin\rsrc.exe"
if (-not (Test-Path $rsrc)) { go install github.com/akavel/rsrc@latest; $rsrc = Join-Path (go env GOPATH) "bin\rsrc.exe" }
& $rsrc -ico ".\internal\desktop\icon.ico" -arch amd64 -o ".\cmd\devin-byok-gui\rsrc_windows_amd64.syso"
Write-Host "wrote cmd\devin-byok-gui\rsrc_windows_amd64.syso"
