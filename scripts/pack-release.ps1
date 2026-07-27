param(
  [string]$Version = ""
)
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
if (-not $Root) { $Root = "D:\Devin-byok" }
Set-Location $Root

if (-not $Version) {
  $vg = Get-Content ".\internal\version\version.go" -Raw
  if ($vg -match 'Version = "([^"]+)"') { $Version = $Matches[1] } else { $Version = "0.0.0" }
}

Write-Host "Building self-contained GUI release $Version ..."
# 先编译内嵌 ls-wrapper + 同步配置模板
New-Item -ItemType Directory -Force -Path .\internal\payload | Out-Null
if (Test-Path .\config.example.yaml) {
  Copy-Item .\config.example.yaml .\internal\payload\config.example.yaml -Force
} elseif (-not (Test-Path .\internal\payload\config.example.yaml)) {
  throw "missing config.example.yaml for embed payload"
}
go build -ldflags "-s -w" -o .\internal\payload\ls-wrapper.exe ./cmd/ls-wrapper
if ($LASTEXITCODE -ne 0) { throw "ls-wrapper build failed" }

$ld = "-X devin-byok/internal/version.Version=$Version -X devin-byok/internal/version.BuildTime=$(Get-Date -Format yyyy-MM-ddTHH:mm:ss)"
# CLI 仅供开发调试，不打进 zip
go build -ldflags $ld -o devin-byok.exe ./cmd/devin-byok
# 嵌入 Windows 应用图标（资源管理器/任务栏）
$rsrc = Join-Path (go env GOPATH) "bin\rsrc.exe"
if (-not (Test-Path $rsrc)) {
  go install github.com/akavel/rsrc@latest
  $rsrc = Join-Path (go env GOPATH) "bin\rsrc.exe"
}
if (Test-Path $rsrc) {
  & $rsrc -ico ".\internal\desktop\icon.ico" -arch amd64 -o ".\cmd\devin-byok-gui\rsrc_windows_amd64.syso"
  if ($LASTEXITCODE -ne 0) { throw "rsrc icon embed failed" }
} else {
  Write-Host "WARN: rsrc not found; GUI may lack Windows file icon"
}
go build -ldflags "-H windowsgui -s -w $ld" -o devin-byok-gui.exe ./cmd/devin-byok-gui

$dist = Join-Path $Root "dist"
$stage = Join-Path $dist ("devin-byok-" + $Version + "-windows-amd64")
if (Test-Path $stage) { Remove-Item -Recurse -Force $stage }
New-Item -ItemType Directory -Force -Path $stage | Out-Null

Copy-Item .\devin-byok-gui.exe $stage\

$start = @"
Devin BYOK v$Version (self-contained GUI)

1. Run devin-byok-gui.exe
2. Configure models/providers in GUI (saved to %USERPROFILE%\.devin-byok\config.yaml)
3. Start service in GUI (auto apply + LS wrapper)
4. Fully quit and reopen Devin, pick a BYOK model

Quit GUI: service stops automatically and Devin settings restore.

Repo: https://github.com/cyberlieflife/devin-byok
License: AGPL-3.0
"@
Set-Content -Encoding UTF8 (Join-Path $stage "START.txt") -Value $start

$zip = Join-Path $dist ("devin-byok-" + $Version + "-windows-amd64.zip")
if (Test-Path $zip) { Remove-Item -Force $zip }
Compress-Archive -Path (Join-Path $stage "*") -DestinationPath $zip -Force
$hash = (Get-FileHash -Algorithm SHA256 $zip).Hash
$hash | Set-Content -Encoding ASCII ($zip + ".sha256")
Write-Host "OK $zip"
Write-Host "SHA256 $hash"
Get-ChildItem $stage | ForEach-Object { Write-Host ("  " + $_.Name) }
