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

Write-Host "Building version $Version ..."
$ld = "-X devin-byok/internal/version.Version=$Version -X devin-byok/internal/version.BuildTime=$(Get-Date -Format yyyy-MM-ddTHH:mm:ss)"
go build -ldflags $ld -o devin-byok.exe ./cmd/devin-byok
go build -ldflags "-H windowsgui $ld" -o devin-byok-gui.exe ./cmd/devin-byok-gui
go build -o devin-byok-ls-wrapper.exe ./cmd/ls-wrapper

$dist = Join-Path $Root "dist"
$stage = Join-Path $dist "devin-byok-$Version-windows-amd64"
if (Test-Path $stage) { Remove-Item -Recurse -Force $stage }
New-Item -ItemType Directory -Force -Path $stage | Out-Null

Copy-Item .\devin-byok.exe,.\devin-byok-gui.exe,.\devin-byok-ls-wrapper.exe $stage
Copy-Item .\config.example.yaml $stage\config.example.yaml
Copy-Item .\README.md $stage\README.md
if (Test-Path .\docs\tool-checklist.md) { Copy-Item .\docs\tool-checklist.md $stage\ }
@"
# 快速开始
1. 复制 config.example.yaml 为 config.yaml 并填写 Family 供应商
2. 管理员可选：将本目录加入 PATH
3. .\devin-byok.exe install
4. .\devin-byok.exe start
5. .\devin-byok-gui.exe
6. 重启 Devin 后选择 BYOK 模型

版本: $Version
"@ | Set-Content -Encoding UTF8 (Join-Path $stage "START.txt")

$zip = Join-Path $dist "devin-byok-$Version-windows-amd64.zip"
if (Test-Path $zip) { Remove-Item -Force $zip }
Compress-Archive -Path (Join-Path $stage "*") -DestinationPath $zip -Force
# sha256
$hash = (Get-FileHash -Algorithm SHA256 $zip).Hash
$hash | Set-Content -Encoding ASCII ($zip + ".sha256")
Write-Host "OK $zip"
Write-Host "SHA256 $hash"
