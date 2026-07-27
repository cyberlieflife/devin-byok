# 监视 Devin / language_server 的 TCP 连接，写入 work/capture/net-watch.jsonl
$ErrorActionPreference = "Continue"
$outDir = "D:\Devin-byok\work\capture"
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
$outFile = Join-Path $outDir "net-watch.jsonl"
$procFile = Join-Path $outDir "proc-watch.jsonl"
Write-Host "watching Devin-related processes... Ctrl+C to stop"
Write-Host "output: $outFile"

function Get-TargetProcs {
  Get-CimInstance Win32_Process | Where-Object {
    $_.Name -match 'Devin|language_server|windsurf' -or
    ($_.CommandLine -and ($_.CommandLine -match 'language_server|Devin\.exe|windsurf'))
  } | Select-Object ProcessId, Name, CommandLine
}

$seenConn = @{}
while ($true) {
  $ts = (Get-Date).ToString("o")
  $procs = @(Get-TargetProcs)
  foreach ($p in $procs) {
    $pline = (@{
      ts = $ts
      pid = $p.ProcessId
      name = $p.Name
      cmd = $p.CommandLine
    } | ConvertTo-Json -Compress)
    Add-Content -LiteralPath $procFile -Value $pline -Encoding UTF8
  }
  if ($procs.Count -eq 0) {
    Start-Sleep -Seconds 2
    continue
  }
  $pids = $procs.ProcessId
  try {
    $conns = Get-NetTCPConnection -State Established, SynSent, CloseWait -ErrorAction SilentlyContinue |
      Where-Object { $pids -contains $_.OwningProcess }
  } catch {
    $conns = @()
  }
  foreach ($c in $conns) {
    $key = "{0}|{1}->{2}:{3}|{4}" -f $c.OwningProcess, $c.LocalAddress, $c.RemoteAddress, $c.RemotePort, $c.State
    if ($seenConn.ContainsKey($key)) { continue }
    $seenConn[$key] = $true
    $procName = ($procs | Where-Object { $_.ProcessId -eq $c.OwningProcess } | Select-Object -First 1).Name
    $row = (@{
      ts = $ts
      pid = $c.OwningProcess
      proc = $procName
      local = "$($c.LocalAddress):$($c.LocalPort)"
      remote = "$($c.RemoteAddress):$($c.RemotePort)"
      state = "$($c.State)"
    } | ConvertTo-Json -Compress)
    Add-Content -LiteralPath $outFile -Value $row -Encoding UTF8
    Write-Host $row
  }
  Start-Sleep -Seconds 2
}
