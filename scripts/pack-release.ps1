param(
  [string]$Version = "",
  [string]$Arch = ""
)
$ErrorActionPreference = "Stop"
# 仓库根：优先 DEVIN_BYOK_REPO 环境变量，回退到脚本所在目录的上级。
# 不要硬编码本机路径，否则脚本无法在其他机器/CI 上复用。
$Root = if ($env:DEVIN_BYOK_REPO) { $env:DEVIN_BYOK_REPO } else { Split-Path -Parent $PSScriptRoot }
if (-not $Root -or -not (Test-Path (Join-Path $Root "go.mod"))) {
  throw "cannot locate repo root (go.mod not found under '$Root'); set DEVIN_BYOK_REPO or run from a checkout"
}
Set-Location $Root

if (-not $Version) {
  $vg = Get-Content ".\internal\version\version.go" -Raw
  if ($vg -match 'Version = "([^"]+)"') { $Version = $Matches[1] } else { $Version = "0.0.0" }
}

# 目标架构：优先 -Arch 参数，其次 GOARCH 环境变量（供 CI 交叉编译），缺省 amd64。
if (-not $Arch) { $Arch = $env:GOARCH }
if (-not $Arch) { $Arch = "amd64" }
$Arch = $Arch.ToLower()
if ($Arch -notin @("amd64", "arm64")) { throw "unsupported arch: $Arch" }
# Windows GUI/ls-wrapper 为纯 Go（无 C 代码）；交叉编译 arm64 时禁用 CGO，
# 避免依赖本机不存在的 arm64 C 交叉工具链。
if ($Arch -eq "arm64") { $env:CGO_ENABLED = "0" }
$env:GOARCH = $Arch

Write-Host "Building self-contained Windows GUI release $Version ($Arch) ..."
# 先同步配置模板
New-Item -ItemType Directory -Force -Path ".\internal\payload" | Out-Null
if (Test-Path ".\config.example.yaml") {
  Copy-Item ".\config.example.yaml" ".\internal\payload\config.example.yaml" -Force
} elseif (-not (Test-Path ".\internal\payload\config.example.yaml")) {
  throw "missing config.example.yaml for embed payload"
}
# 备份跟踪的内嵌 ls-wrapper，打包结束恢复，避免交叉编译污染 git 状态（对齐 pack-release.sh）。
$lw = ".\internal\payload\ls-wrapper.exe"
$lwBak = "$lw.bak_$(Get-Date -Format yyyyMMdd_HHmmss)"
if (Test-Path $lw) { Copy-Item $lw $lwBak -Force }

try {
  # 编译内嵌 ls-wrapper（随 $env:GOARCH 生成对应架构的包装器）
  go build -ldflags "-s -w" -o ".\internal\payload\ls-wrapper.exe" ./cmd/ls-wrapper
  if ($LASTEXITCODE -ne 0) { throw "ls-wrapper build failed" }

  $ld = "-X devin-byok/internal/version.Version=$Version -X devin-byok/internal/version.BuildTime=$(Get-Date -Format yyyy-MM-ddTHH:mm:ss)"
  # 客户端 仅供开发调试，不打进 zip
  go build -ldflags $ld -o devin-byok.exe ./cmd/devin-byok
  # 嵌入 Windows 应用图标（资源管理器/任务栏）
  $rsrc = Join-Path (go env GOPATH) "bin\rsrc.exe"
  if (-not (Test-Path $rsrc)) {
    # 锁 rsrc 版本保证可复现构建；生成的 .syso 是跟踪文件，不能依赖最新版漂移
    go install github.com/akavel/rsrc@v0.10.2
    $rsrc = Join-Path (go env GOPATH) "bin\rsrc.exe"
  }
  if (Test-Path $rsrc) {
    & $rsrc -ico ".\internal\desktop\icon.ico" -arch $Arch -o ".\cmd\devin-byok-gui\rsrc_windows_$Arch.syso"
    if ($LASTEXITCODE -ne 0) { throw "rsrc icon embed failed" }
  } else {
    Write-Host "WARN: rsrc not found; GUI may lack Windows file icon"
  }
  go build -ldflags "-H windowsgui -s -w $ld" -o devin-byok-gui.exe ./cmd/devin-byok-gui
  if ($LASTEXITCODE -ne 0) { throw "GUI build failed" }

  $dist = Join-Path $Root "dist"
  New-Item -ItemType Directory -Force -Path $dist | Out-Null
  $exe = Join-Path $dist ("devin-byok-" + $Version + "-windows-" + $Arch + ".exe")
  if (Test-Path $exe) { Remove-Item -Force $exe }
  Copy-Item .\devin-byok-gui.exe $exe
  $hash = (Get-FileHash -Algorithm SHA256 $exe).Hash
  $hash | Set-Content -Encoding ASCII ($exe + ".sha256")
  Write-Host "OK $exe"
  Write-Host "SHA256 $hash"
} finally {
  if (Test-Path $lwBak) {
    Copy-Item $lwBak $lw -Force
    Remove-Item $lwBak -Force
  }
}
