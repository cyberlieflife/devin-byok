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

Write-Host "Building GUI-only release $Version ..."
$ld = "-X devin-byok/internal/version.Version=$Version -X devin-byok/internal/version.BuildTime=$(Get-Date -Format yyyy-MM-ddTHH:mm:ss)"
go build -ldflags $ld -o devin-byok.exe ./cmd/devin-byok
go build -ldflags "-H windowsgui $ld" -o devin-byok-gui.exe ./cmd/devin-byok-gui

$dist = Join-Path $Root "dist"
$stage = Join-Path $dist ("devin-byok-" + $Version + "-windows-amd64")
if (Test-Path $stage) { Remove-Item -Recurse -Force $stage }
New-Item -ItemType Directory -Force -Path $stage | Out-Null

Copy-Item .\devin-byok-gui.exe $stage\
Copy-Item .\config.example.yaml $stage\

$start = @"
Devin BYOK v$Version (GUI only)

1. Copy config.example.yaml to config.yaml
2. Edit config.yaml (base_url / api_key / upstream_model)
3. Run devin-byok-gui.exe
4. Click Start service in GUI (auto apply to Devin)
5. Fully quit and reopen Devin, pick a BYOK model

Stop: click Stop service in GUI (auto restore)

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
