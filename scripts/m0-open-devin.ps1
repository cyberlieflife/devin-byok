# M0 操作清单
Write-Host @"
==== Devin BYOK M0 抓包（无登录）====

终端 A:
  cd D:\Devin-byok
  go run .\cmd\devin-byok serve

终端 B:
  powershell -ExecutionPolicy Bypass -File D:\Devin-byok\scripts\watch-devin-net.ps1

然后打开 Devin（不要登录）:
  1. 等到界面稳定
  2. 打开 Chat/Cascade 面板
  3. 能输入就发: hello
  4. 点一下模型选择（如果有）
  5. 停 30-60 秒

完成后告诉我，我读取:
  D:\Devin-byok\work\capture\*.jsonl
"@
