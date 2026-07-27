# devin-byok

让 **Devin**（Windsurf 改名）使用你自己的 OpenAI 兼容 / Anthropic 模型服务（BYOK）。

当前版本见 GUI 标题或 `GET /api/version`。

## 能力清单

| 功能 | 状态 |
|------|------|
| LS wrapper 劫持 | OK |
| pure_local 免登录 | OK |
| start 自动 apply / stop 自动 restore | OK |
| Family 多模型 + 思考强度 | OK |
| 模型内供应商（base_url/key/upstream） | OK |
| 流式正文 / 思考链 | OK |
| Cascade tools（非流式合并） | OK |
| Prompt cache | OK |
| 多模态（ImageData → vision） | OK |
| DeepWiki 流式 | OK |
| CodeMap Fast/Smart 分模型 | OK |
| GUI 监控 / 模型卡 / 设置 / 托盘 | OK |
| Anthropic 原生 `/v1/messages` | OK |
| RPC 日志按大小轮转 | OK |

## 快速开始

```powershell
cd D:\Devin-byok
# 1. 编辑 config.yaml（至少配置一个 Family 的 base_url / api_key / upstream_model）
# 2. 安装 LS wrapper（首次）
.\devin-byok.exe install
# 3. 启动（会自动 apply portal/api 到 Devin settings.json）
.\devin-byok.exe start
# 4. 打开 GUI（可选）
.\devin-byok-gui.exe
# 5. 完全退出 Devin 再打开，选择 BYOK 模型对话
```

停止（会自动 restore settings）：

```powershell
.\devin-byok.exe stop
```

## 配置摘要

```yaml
upstream:
  thinking:
    param: reasoning_effort
    default: medium
  families:
    - uid: grok-4.5-byok
      label: Grok 4.5
      provider: openai          # 或 anthropic
      base_url: http://localhost:8317/v1
      api_key: "123"
      upstream_model: grok-4.5
      context_window: 200000
      max_tokens: 8192
  models:
    - id: grok-4.5-byok-medium
      label: Grok 4.5 Medium
      upstream_model: grok-4.5
      thinking: medium
      family_uid: grok-4.5-byok

features:
  pure_local: true
  enable_stream: true
  enable_cascade_tools: true
  enable_deepwiki: true
  enable_codemap: true
  deepwiki_model: grok-4.5-byok-medium
  codemap_fast_model: grok-4.5-byok-low
  codemap_smart_model: grok-4.5-byok-high
```

Anthropic 示例：`provider: anthropic`，`base_url: https://api.anthropic.com`（或兼容网关），`upstream_model: claude-sonnet-4-...`。

## CLI

| 命令 | 作用 |
|------|------|
| install / uninstall | LS wrapper |
| start / stop | 后台 API + apply/restore |
| serve | 前台 API |
| apply / restore | 仅写/恢复 Devin settings |
| doctor | 健康检查 |
| autostart on\|off\|status | 开机自启 |
| gui | 启动 GUI |
| test-upstream | 测上游 |

## GUI

- **监控**：请求成功失败、Token、缓存命中、模型排行、DeepWiki/CodeMap 统计、实时日志（正序贴底）
- **模型**：功能模型绑定（DeepWiki / CodeMap Fast / Smart）+ Family 卡片
- **设置**：工具模式、流式、pure_local、桌面托盘/自启

## 路径

- 配置：`D:\Devin-byok\config.yaml`
- Devin settings：`%APPDATA%\Devin\User\settings.json`
- apply 元数据：`%APPDATA%\devin-byok\last-apply.json`
- 指标：`%APPDATA%\devin-byok\metrics.json`
- RPC 日志：`work\capture\localapi-rpc.jsonl`（约 32MB 轮转，保留 .1/.2/.3）
- GUI 偏好：`%APPDATA%\devin-byok\gui.json`

## 发布包

```powershell
.\scripts\pack-release.ps1
# 输出 dist/devin-byok-<version>-windows-amd64.zip
```

## 注意

1. `start` 会改 Devin settings 指向本机 8787；`stop` 会 restore。
2. 改二进制后必须 `stop` 再 `start`，GUI 静态资源已 no-cache。
3. tools 流式 incomplete 分片已刻意不做（易截断 JSON）。
4. 与 dao-proxy-pro 等改 SP/路由的扩展可能冲突。

## 在线更新

`powershell
.\devin-byok.exe update check
.\devin-byok.exe update apply
`

或在 GUI **设置 → 更新** 中检查/下载。

配置：

`yaml
update:
  enabled: true
  repo: cyberlieflife/devin-byok
  asset_contains: windows-amd64.zip
  auto_apply: false
`

Release 资产命名需包含 windows-amd64.zip，建议附带同名 .sha256。

## 许可证

本项目以 **GNU Affero General Public License v3.0 (AGPL-3.0)** 授权，见 [LICENSE](LICENSE)。
