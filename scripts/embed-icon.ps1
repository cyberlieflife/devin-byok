param(
  [string]$Arch = "amd64"
)
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root
if (-not $Arch) { $Arch = "amd64" }
$Arch = $Arch.ToLower()
if ($Arch -notin @("amd64", "arm64")) { throw "unsupported arch: $Arch" }
$rsrc = Join-Path (go env GOPATH) "bin\rsrc.exe"
if (-not (Test-Path $rsrc)) { go install github.com/akavel/rsrc@latest; $rsrc = Join-Path (go env GOPATH) "bin\rsrc.exe" }
& $rsrc -ico ".\internal\desktop\icon.ico" -arch $Arch -o ".\cmd\devin-byok-gui\rsrc_windows_$Arch.syso"
if ($LASTEXITCODE -ne 0) { throw "rsrc icon embed failed for arch $Arch" }
Write-Host "wrote cmd\devin-byok-gui\rsrc_windows_$Arch.syso"
