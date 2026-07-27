# Devin BYOK · Companion 扩展 + 系统提示词设计

> 基于 `dao-proxy-pro-9.9.358.vsix` 裁剪重写。本文件是设计，实现分阶段。

## 1. 参考项目结论（dao-proxy-pro）

dao-proxy-pro 做了三件大事：

1. **改系统提示词（SP）**：invert / passthrough / custom，本地 origin 控制面（默认 8937/37808）
2. **模型路由**：官方 UID → 自配渠道
3. **活动栏三面板**：本源观照 / 渠道配置 / 模型路由

BYOK 已有 family 供应商与本地 8787，因此：

- **不复刻** 渠道路由与经文 invert
- **借鉴** 控制面 API + Devin 内 Webview UI + 热生效
- **控制面并入 8787**，不新开 8937

## 2. 改 SP 的双路径

### 路径 A — 服务端注入（主路径，无扩展也可用）

在 chat 组装 messages 时：

- `mode=off`：不改官方 SP
- `mode=append`：官方 SP + 用户 SP（默认）
- `mode=replace`：仅用户 SP（可保留最小 tools 约束）

配置示例：

```yaml
system_prompt:
  mode: append
  text: |
    你是用户的本地 BYOK 助手……
  preserve_tools_hint: true
```

GUI 设置页可直接编辑并热重载。

### 路径 B — Companion 扩展（Devin 内界面）

- 扩展 id：`devin-byok.companion`
- 活动栏单面板：**BYOK 系统提示词**
- Webview：mode / 文本 / 预览 / 与 8787 连接状态
- 只调 BYOK API，**不做模型路由**

API 草案：

- `GET /api/system-prompt`
- `PUT /api/system-prompt`
- `GET /api/system-prompt/preview`（最近一次实际 system 前 2k）

## 3. start / stop 生命周期

```
start:
  apply portal/api
  ensure companion vsix installed
  enable companion extension

stop:
  disable companion extension
  restore portal/api
```

内置 vsix：`resources/devin-byok-companion.vsix`（随 CLI/GUI 分发）。
探测 Devin CLI 安装扩展；失败则仅服务端 SP 生效并打日志。

## 4. 扩展目录草图

```
extensions/devin-byok-companion/
  package.json
  extension.js
  media/icon.svg
  readme.md
```

贡献点：activitybar 视图 + 命令 `byok.openSystemPrompt` + 配置 `byok.apiBase=http://127.0.0.1:8787`。

## 5. GUI 与扩展关系

- **BYOK GUI**：编辑 SP、是否 start 时装扩展
- **Devin Companion**：同一 config 的可视化编辑
- **服务端**：唯一生效点（每次 chat 注入）

## 6. 实施顺序

1. **S1** 服务端 system_prompt + chat 注入 + GUI 编辑（无扩展）
2. **S2** `/api/system-prompt` + preview
3. **S3** 精简 companion 扩展（webview）
4. **S4** start/stop 自动 install/enable/disable
5. **S5** 打进 release 资源包

## 7. 风险

- Devin 扩展 CLI 与 VS Code 参数差异 → S4 需实机
- replace 可能丢工具约束 → 默认 append
- 与 dao-proxy-pro 同装可能双改 SP → 文档互斥
- disable 扩展后服务端 SP 仍生效（预期）

## 8. 本轮已落地

- 日志：正序、新在底部、贴底滚动
- 多模态：ImageData → OpenAI image_url content parts
- 设计：本文档

待确认后优先做 S1 或 S1+S3。
