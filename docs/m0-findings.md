# M0 运行时摸底结论（基于你这次真实操作）

时间：2026-07-26  
操作：打开 Devin → 登录 → 基本设置 → Agent/Editor → 发 hello → 模型回复 → 切模型 → 等待

## 1. 关键结论（先看这个）

1. **你这次聊天主路径是 Agent(ACP)，不是纯旧 Cascade-only。**
2. 本机同时存在两套 AI 运行时：
   - `language_server_windows_x64.exe`（Codeium LS）
   - `devin.exe acp`（本地 ACP Agent，日志里叫 chisel）
3. **系统代理劫持对 LS 可能无效**：LS 启动参数带 `--detect_proxy=false`
4. **更稳的接管点是改 `api_server_url`**（启动参数/配置注入），不是只靠 MITM
5. 当前 `local-api(127.0.0.1:8787)` **没有收到请求**（尚未 Apply portalUrl，符合预期）
6. 外连 IP 落在 `198.18.x.x`，说明本机有 **Fake-IP/透明代理（如 Clash）**；域名被代理层接管

## 2. 进程与启动参数

### 2.1 Language Server
```
language_server_windows_x64.exe
  --api_server_url https://server.codeium.com
  --inference_api_server_url https://inference.codeium.com
  --extension_server_port 56467
  --ide_name windsurf
  --random_port                  # 实际监听 127.0.0.1:56491
  --database_dir C:\Users\cyber\.codeium\windsurf\database\...
  --codeium_dir .codeium/windsurf
  --extensions_dir C:\Users\cyber\.devin\extensions
  --windsurf_version 3.5.17
  --stdin_initial_metadata
  --detect_proxy=false
```

含义：
- 官方云：`server.codeium.com` / `inference.codeium.com`
- 本地扩展服务端口：`56467`（extension server）
- LS 自身随机端口：`56491`
- **忽略系统代理探测** → 单纯改 Windows/系统代理不一定能劫持 LS

### 2.2 ACP Agent（你点的 Agent/Editor 主路径）
日志证据（`Devin ACP devin-cli.log`）：
- 启动：`devin.exe acp`
- 认证：`method_id=windsurf-api-key`
- meta keys：`["api_key", "api_server_url"]`
- 策略：`ACP host is the sole source of credentials`
- 结果：`Authenticated bundled agent with Devin API key`

另有 cloud 通道：
- `wss://app.devin.ai/api/acp/live`（`Devin ACP devin-cloud.log`）

所以 Agent 模式至少有：
1. **本地 ACP + api_key/api_server_url**
2. **远程 ACP WebSocket（app.devin.ai）**

你的 hello/切模型，极大概率走了 ACP 体系（本地 chisel + 远端/官方 API），而不只是 LS chat RPC。

## 3. 用户数据落点（已确认）

| 路径 | 作用 |
|---|---|
| `%APPDATA%\Devin` | Electron 用户数据（VS Code 系） |
| `%USERPROFILE%\.devin` | argv/extensions |
| `%USERPROFILE%\.codeium\windsurf` | LS 数据库、user_settings.pb、cascade 会话 |
| `%LOCALAPPDATA%\devin` | CLI/team_settings/telemetry |
| `%USERPROFILE%\CascadeProjects` | 工作区相关 |

模型列表缓存于：
`%USERPROFILE%\.codeium\windsurf\user_settings.pb`  
内含大量官方模型 slug（如 `claude-opus-5-medium`、`gpt-5.6-sol` 等）和 `https://server.codeium.com`。

## 4. 网络观察

Established 外连样本（进程侧）：
- LS / NetworkService / ACP → `198.18.1.x:443`
- 这是典型 Fake-IP 段，**真实目标域名被本地代理解析**，不能直接当公网 IP 解读

本地环回：
- 大量连接打到 `127.0.0.1:56491`（LS）
- extension server `56467`

`net-watch.jsonl` 早期几乎没记到，是因为监视脚本在 Devin 启动前只看到 devin-byok 自身；后续以实时 `Get-NetTCPConnection` 与日志为准。

## 5. 对 MVP-A 架构的修正

原方案只强调：
`portalUrl -> API_SERVER_URL -> mock server.codeium.com`

现在要并列两条链路：

```text
路径 A（LS/旧 Cascade 能力）
  extension 启动 LS
    --api_server_url <目标>
    --inference_api_server_url <目标>
  => 我们的 local-api

路径 B（Agent/ACP，你这次实际用的）
  extension 调 ACP authenticate
    meta.api_key
    meta.api_server_url
  => devin.exe acp 用这对凭证访问 API
  另可能连 wss://app.devin.ai/api/acp/live
```

### MVP-A 优先级调整
1. **先接管路径 B 的 `api_server_url` + 假 api_key**（对齐你当前 UI 用法）
2. 同步接管路径 A 的 LS `--api_server_url`（portalUrl/配置注入）
3. cloud websocket `app.devin.ai`：先观察是否可关/可旁路；若强制，再决定是否要 mock/阻断

## 6. Apply 注入策略（下一阶段）

### 6.1 低侵入（优先）
- 写 `devin.portalUrl` / `windsurf.portalUrl` = `http://127.0.0.1:8787`
- 期望扩展把 LS/相关客户端指到 `http://127.0.0.1:8787/_route/api_server`
- 注入本地假 session/api_key，满足 isUserLoggedIn / ACP authenticate

### 6.2 中侵入（若 portalUrl 不够）
- 包装 `language_server_windows_x64.exe` 启动参数，强制：
  - `--api_server_url http://127.0.0.1:8787/_route/api_server`
  - `--inference_api_server_url http://127.0.0.1:8787/_route/api_server`
- 或设置扩展可识别的 config/env（若存在 LANGUAGE_SERVER_ENV）

### 6.3 Agent 专项
- 确认 ACP authenticate 的 `api_server_url` 是否跟随 portalUrl/API_SERVER_URL
- 若否：在扩展侧配置/补丁点改 meta
- local-api 需实现 ACP/chisel 认的最小鉴权与模型/对话接口（可能与纯 ApiServerService 不完全相同）

## 7. local-api 下一步要实现的“观察清单”

启动后应在 `localapi-rpc.jsonl` 看到的请求类型（预期）：
- 鉴权/用户状态（GetUserStatus / GetStatus 等）
- 模型配置（GetCascadeModelConfigs / GetCliModelConfigs / GetModelProviders）
- 聊天/Agent（GetChatMessage 或 ACP 对应 HTTP API）
- 大量 Record/Log 可 stub

若 Apply 后仍只有 `app.devin.ai` websocket、没有打到 8787：
- 说明 Agent 仍走 cloud ACP，需要额外旁路策略

## 8. 风险更新

| 风险 | 等级 | 说明 |
|---|---|---|
| Agent 走 cloud ACP | 高 | 仅 mock server.codeium.com 可能不够 |
| LS detect_proxy=false | 中 | MITM 方案降级，必须改 URL |
| 本机 Fake-IP 代理 | 中 | 抓包解读需结合日志，不要只看 198.18 |
| 日志默认很安静 | 中 | windsurf 扩展 log 信息少，要靠参数/流量 |
| 已登录状态污染 | 低 | 你已登录；无登录方案还需单独验证冷启动 |

## 9. M0 状态

- [x] 确认用户数据目录
- [x] 确认 LS 启动参数与官方 URL
- [x] 确认 Agent/ACP 路径与认证 meta
- [x] 确认模型缓存文件
- [ ] Apply 后把流量打进 local-api（M1/M4）
- [ ] 抓到首个聊天 RPC/ACP HTTP 样本并做 OpenAI 转换（M3）

## 10. 建议的下一步（请你选）

**S1（推荐）**：实现 `devin-byok apply`  
- 写 portalUrl  
- 备份设置  
- 重启提示  
- 再让你用 Agent 发 hello，看 8787 是否进请求  

**S2**：先做 LS wrapper（强制 api_server_url）  
- 比 portalUrl 更硬  

**S3**：先深挖 `devin.exe acp` 协议  
- 专门对着 Agent 路径逆向  

我建议 **S1 → 若流量不进来再 S2/S3**。
## Cascade「你好」补充抓包（apply 前）

- 新会话 pb: `%USERPROFILE%\.codeium\windsurf\cascade\3c7a2d23-f4a8-4794-a368-5a79b8be92e5.pb` (22:50)
- LS 仍指向: `--api_server_url https://server.codeium.com`
- LS 外连: `198.18.1.156:443` / `198.18.1.158:443`（Fake-IP 代理后）
- 本地: 大量 `127.0.0.1 -> LS:56491`
- local-api 当时无请求（未 apply）

## S1 apply 已执行

- settings.json:
  - `devin.portalUrl = http://127.0.0.1:8787`
  - `windsurf.portalUrl = http://127.0.0.1:8787`
- local-api: 运行中
- 需用户完全重启 Devin 后验证 LS 参数是否变为本地 URL

## 根因：正式版忽略 settings 里的 API URL

`getConfig` 对 `codeium.*` 在非 dev 模式下直接走默认值 `h(A)`，**用户 settings.json 的 `codeium.apiServerUrl` / portalUrl 不会进入 LS 启动参数**。

## S2 已安装 LS 包装器

- 原文件备份: `language_server_windows_x64.exe.bak_20260726_232315`
- 真身: `language_server_windows_x64.real.exe`
- 包装器: `language_server_windows_x64.exe` (~3MB)
- 强制改写:
  - `--api_server_url http://127.0.0.1:8787/_route/api_server`
  - `--inference_api_server_url` 同上
- 冒烟日志: `%APPDATA%\devin-byok\ls-wrapper-last.json` 已确认改写成功
- 卸载: `python D:\Devin-byok\scripts\uninstall-ls-wrapper.py`

下一步：用户完全重启 Devin 后再发消息，检查 localapi-rpc.jsonl
