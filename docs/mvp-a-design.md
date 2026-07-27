# Devin BYOK MVP-A 设计稿（待确认后开工）

## 0. 已确认范围

| 项 | 选择 |
|---|---|
| 目标 | A：Chat-only MVP |
| 上游协议 | OpenAI 兼容（`/v1/chat/completions`，支持 stream） |
| 登录 | 本地假登录（不接官方账号） |
| 非目标（本期不做） | Cascade 工具改文件/跑命令、Tab 补全、索引、企业 SSO |

成功标准：
1. 打开 Devin，无需官方登录
2. 在 Helper 里配置 `base_url + api_key + model`
3. 在 Devin Chat/Cascade 文本对话中能流式收到自定义模型回复

---

## 1. 总体架构

```text
┌─────────────────────────────────────────┐
│ devin-byok-helper (桌面端，Go/Wails 或 CLI) │
│ - 编辑 config.yaml                        │
│ - 启动/停止 local-api                     │
│ - 一键 Apply / Restore Devin 注入          │
│ - 上游连通测试 (OpenAI chat.completions)   │
└───────────────────┬─────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────┐
│ local-api (本地兼容层)                    │
│ listen: 127.0.0.1:8787                   │
│ 路径前缀: /_route/api_server              │
│ 协议: Connect-RPC / protobuf（Codeium）   │
└───────────────────┬─────────────────────┘
                    │ 转 OpenAI 兼容
                    ▼
            用户模型网关 (任意兼容服务)
```

### 为什么用 `portalUrl` 注入（关键）

Devin 扩展里存在多租户映射逻辑：

- 设置 `devin.portalUrl = http://127.0.0.1:8787`
- 自动得到：
  - `API_SERVER_URL = http://127.0.0.1:8787/_route/api_server`
  - `INFERENCE_API_SERVER_URL = 同上`
  - `MULTI_TENANT_MODE = true`

这是**官方已有配置入口**，比直接改内部 secret/魔改二进制更稳。

同时 Helper 仍要处理：
- 写入/注入本地假 `api_key` session（让 `isUserLoggedIn()` 为 true）
- 必要时写 `globalState` / `secrets` 中的 `windsurf_auth.*`

---

## 2. 配置文件设计

路径（Windows）：
`%APPDATA%\devin-byok\config.yaml`

```yaml
server:
  host: 127.0.0.1
  port: 8787
  # 对外 portal 根；Devin portalUrl 指向这里
  public_base: "http://127.0.0.1:8787"

auth:
  # 本地假登录用，任意非空字符串即可
  fake_api_key: "devin-byok-local-key"
  fake_email: "byok@local"
  fake_name: "BYOK Local"

upstream:
  base_url: "https://api.openai.com/v1"   # 可改中转站
  api_key: "sk-xxx"
  model: "gpt-4.1-mini"
  # 可选
  timeout_sec: 120
  default_headers: {}

devin:
  # Apply 时写入的设置
  portal_url_setting_keys:
    - devin.portalUrl
    - windsurf.portalUrl
  # 用户数据目录（自动探测优先）
  data_dir_candidates:
    - "%APPDATA%/Devin"
    - "%USERPROFILE%/.devin"
    - "%APPDATA%/Windsurf"
    - "%USERPROFILE%/.windsurf"

features:
  # MVP-A 只开聊天相关
  enable_chat: true
  enable_cascade_tools: false
  stub_unknown_rpc: true   # 未实现 RPC 返回空成功/安全默认，避免卡死
```

---

## 3. local-api 接口策略

### 3.1 对外路由

| 路径 | 作用 |
|---|---|
| `GET /healthz` | Helper 健康检查 |
| `POST /v1/chat/completions` | 可选：直接测上游（透传） |
| `/*/_route/api_server/*` 或 `/_route/api_server/*` | Devin/LS Connect 入口 |

Connect-RPC 典型路径形态：
`/exa.api_server_pb.ApiServerService/<Method>`
以及 seat/user 相关 service。

### 3.2 MVP-A 必做 RPC（P0）

> 最终名单以“首次启动抓包”校准；下列为静态证据下的最小集合。

**鉴权 / 用户状态**
- `GetStatus`
- `GetUserStatus`（seat 或 api_server 侧同名都要兜住）
- 任何返回 plan/tier/permission 的轻量接口 → 固定“已授权、无限额”

**模型**
- `GetModelProviders`
- `GetCascadeModelConfigs` / `GetCascadeModelConfigsForSite`
- `GetCommandModelConfigs` / `GetCliModelConfigs`（能 stub 就 stub）
- `GetExternalModel` / `GetExternalModelsGroup`（可返回配置中的自定义模型）

**聊天（核心）**
优先打通其中一条真实上游转换链路：
1. `GetStreamingExternalChatCompletions`（更贴近 external/BYOK）
2. 或 `GetChatMessage` / `GetChatCompletions`

转换逻辑：
```text
Codeium chat request
  -> 提取 messages / system / model
  -> OpenAI: POST {base_url}/chat/completions
       headers: Authorization: Bearer {api_key}
       body: {model, messages, stream:true}
  -> 把 SSE/stream chunk 映射回 Codeium 流式响应
```

### 3.3 一律安全 Stub（P1）

对未知 RPC：
- 记录 method + 粗略请求大小
- 返回“空成功”或预置 protobuf 默认值
- 不崩溃、不 401（避免把假登录打掉）

埋点类（`Record*` / `Log*`）全部 no-op 成功。

### 3.4 明确不做（MVP-A）

- Cortex 工具调用闭环
- 代码索引 / embeddings 质量
- SSO/OIDC
- 官方计费/seat 同步

---

## 4. 假登录注入设计

### Apply 流程
1. 启动 `local-api`
2. 健康检查 `/healthz`
3. 写 Devin 设置：
   - `devin.portalUrl = http://127.0.0.1:8787`
   - `windsurf.portalUrl = 同上`
4. 注入本地 session：
   - 生成/写入 `fake_api_key`
   - 尽量走扩展可识别的 auth 存储（`windsurf_auth.sessions` / secrets）
5. 提示用户 **重载窗口 / 重启 Devin**
6. 状态显示：`Applied / LoggedIn(local) / UpstreamOK`

### Restore 流程
1. 清空 portalUrl（或恢复备份值）
2. 删除本地假 session
3. 停止 local-api
4. 提示重启 Devin

### 备份
任何改 Devin 用户设置/状态前：
- 备份 `settings.json` / 相关 state
- 带时间戳 `.bak_YYYYMMDD_HHMMSS`
- 成功后对比，再删备份（遵循你的备份规范）

---

## 5. 工程目录（建议）

```text
devin-byok/
  README.md
  config.example.yaml
  cmd/
    devin-byok/          # CLI 入口（可先 CLI，后 Wails）
  internal/
    config/              # 配置加载
    upstream/openai/     # OpenAI 兼容客户端
    localapi/
      server.go          # HTTP server
      connect_router.go  # RPC 分发
      auth_stub.go       # 假登录/用户状态
      models_stub.go     # 模型列表
      chat_bridge.go     # 聊天协议转换
      stub.go            # 未知 RPC
    devin/
      detect.go          # 探测安装/数据目录
      apply.go           # 写入 portalUrl/session
      restore.go
      backup.go
    logx/
  scripts/
    smoke_upstream.ps1
    smoke_localapi.ps1
```

技术选型建议（MVP）：
- **Go + 标准库 net/http**（先 CLI）
- protobuf：先用“抓包样本 + 手工最小结构 / dynamic stub”，不全量生成官方 proto
- UI：第一期 CLI 足够；稳定后再 Wails

---

## 6. 开发里程碑

### M0：动态摸底（0.5–1 天）
- 启动 Devin（可先官方登录一次作对照，或直接 Apply 本地）
- 抓 `language_server` 出站 RPC 顺序
- 产出 `rpc-bootstrap-list.md`（启动必调用列表）
- 产出一次 Chat 请求的 request/response 样本

### M1：local-api 骨架
- healthz
- 路由 `/_route/api_server`
- 未知 RPC stub + 日志
- 假 GetStatus/GetUserStatus

### M2：OpenAI 上游
- config 读取
- non-stream / stream chat 客户端
- `devin-byok test-upstream`

### M3：Chat 桥接
- 打通 1 条流式聊天 RPC
- Devin 窗口可见 token 流

### M4：Apply/Restore
- portalUrl 注入
- session 注入
- 备份/恢复
- 一键脚本

### M5：验收
- 冷启动免登录
- 改 model/url/key 生效
- Restore 后回到原状

---

## 7. 验收用例

1. 未登录官方账号，Apply 后打开 Devin，不再被强制卡在登录墙（或可关闭登录墙进入聊天）
2. `config.yaml` 配自家中转，`test-upstream` 成功
3. Devin 发 “ping”，流式返回上游内容
4. 换 model 后再聊，请求体 model 字段变化
5. Restore 后 portalUrl/session 恢复，本地服务可停

---

## 8. 风险与对策

| 风险 | 对策 |
|---|---|
| 启动 RPC 比预期多 | M0 抓包；stub 默认放行 |
| 假登录注入点不准 | 双路径：portalUrl + secrets/globalState；必要时手动 auth token 命令 |
| protobuf 难解 | 先对聊天链路做最小字段映射；其余返回空消息 |
| 只通 Chat 不通 Cascade 文本框 | 两个入口都抓；优先用户实际点的面板 |
| 版本升级 | 锁定当前 Devin commit `2c489dfc...`；README 写明支持版本 |

---

## 9. 需要你确认的设计点

请直接回：

1. **交付形态**：先做 **CLI**，还是直接 **桌面 GUI**？  
   - 建议：先 CLI（快）
2. **项目落盘位置**：  
   - `D:\Devin-byok`？  
   - 还是当前 Codex outputs 旁的工作区？
3. **是否允许 M0 动态抓包**（可能需你本地启动一次 Devin）？  
   - 建议：允许
4. **上游 base_url 是否总是带 `/v1`**，还是允许用户写到根域名由程序补 `/chat/completions`？  
   - 建议：兼容两种，优先尊重用户完整 base_url

你确认后，我按 **M0 → M1** 开工。
