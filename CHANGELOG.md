# 更新日志 / Changelog

本文件记录 devin-byok 的主要版本变更。自 v1.4.0 起，每个版本的条目使用中英双语（中文在前、英文在后）。
This file records the major changes of devin-byok. From v1.4.0 onward, each version entry is written bilingually (Chinese first, then English).

## [v1.4.3] - 2026-08-17

### 新增 / Features

- **Agent/ACP 通道接管**：Devin 的 Agent/Editor 模式（`devin.exe acp`）此前一直走官方 `server.codeium.com`（`codeium.apiServerUrl` 不是 Devin 注册的配置键，settings 注入无法改变它），导致 Agent 模式模型列表与账号仍为官方。实测 `devin.exe` 优先使用 `WINDSURF_API_SERVER_URL` 环境变量覆盖认证 meta 里的 API 地址，因此：Windows 把 bundle 内 `devin.exe` 替换为包装器（注入环境变量后 exec 真身），macOS 因签名 bundle 不可写改用 `launchctl setenv` 用户级环境变量。接管后模型列表（`GetCliModelConfigs`/`GetUserStatus`）、账号与聊天（`GetChatMessage`，与 LS 同一协议）全部走本地兼容层。
- **Agent/ACP channel takeover**: Devin's Agent/Editor mode (`devin.exe acp`) previously always talked to the official `server.codeium.com` (`codeium.apiServerUrl` is not a registered Devin config key, so settings injection cannot change it), leaving the Agent-mode model list and account official. Since `devin.exe` prefers the `WINDSURF_API_SERVER_URL` environment variable over the API URL in the authenticate meta, Windows now replaces the bundled `devin.exe` with a wrapper (injects the env var then execs the real binary), and macOS uses `launchctl setenv` (the signed bundle cannot be written). After takeover, the model list (`GetCliModelConfigs`/`GetUserStatus`), account, and chat (`GetChatMessage`, same protocol as the LS) all go through the local compatibility layer.

### 修复 / Fixes

- 修复 Agent/ACP 模式下模型列表只有官方模型、账号非 BYOK（与 Cascade 通道不一致）：根因是 ACP 通道的 API 地址来自扩展 `getConfig(Config.API_SERVER_URL)`（官方默认），本地 settings 注入无效；现通过包装 `devin.exe` / `launchctl setenv` 注入 `WINDSURF_API_SERVER_URL` 覆盖。
- Fixed Agent/ACP mode showing only official models and a non-BYOK account (inconsistent with the Cascade channel): the ACP channel's API URL came from the extension's `getConfig(Config.API_SERVER_URL)` (official default), which local settings injection cannot change; the fix wraps `devin.exe` / uses `launchctl setenv` to inject `WINDSURF_API_SERVER_URL`.

## [v1.4.2] - 2026-08-17

### 修复 / Fixes

- 修复 Windows 上「本地模型服务已启用但 Devin 仍走官方账号」：3.7.16 适配时停用了 bundle 植入（为 macOS 签名保护），但替代的 settings 覆盖链路（.real 拷贝 + Codeium.codeium-dev 扩展壳）未实现，Windows 一直依赖旧 bundle wrapper 残留，bundle 被还原或 Devin 更新后本地服务收不到任何请求。现在 Windows 恢复自动植入 wrapper，bundle 被还原或 Devin 更新后重新启用 GUI 即可自动恢复。
- Fixed "local model service enabled but Devin still uses the official account" on Windows: the 3.7.16 adaptation disabled bundle injection (for macOS code signing), but the replacement settings-override path (.real copy + Codeium.codeium-dev shell) was never implemented, so Windows relied on a stale bundle wrapper; after the bundle was restored or Devin updated, the local service received no requests. Windows now re-enables automatic wrapper injection, and re-enabling the GUI after a bundle restore or Devin update restores the local path.
- 修复 Family 级配置下模型列表只剩官方模型且聊天报 "Model provider unreachable"：下发给 Devin 的模型卡片 model_info.base_url（protobuf 字段 11）此前只写 legacy 全局 upstream.base_url，Family 卡配置的 base_url 缺失该字段，Devin 语言服务器判定 provider 不可达并过滤 BYOK 模型。现按 模型级→family 级→全局 逐级解析写入。
- Fixed "model list keeps official models" and "Model provider unreachable" with family-based configs: model_info.base_url (protobuf field 11) previously only carried the legacy global upstream.base_url, so the family-card base_url was missing and the Devin language server treated the provider as unreachable, filtering out BYOK models. It now resolves model→family→global.

### 新增 / Features

- macOS 本地链路完善：自动从 Devin.app 拷贝真实语言服务器到数据目录 bin（.real 副本），并安装 Codeium.codeium-dev 扩展壳使 codeiumDev.languageServerBinaryPath 生效（签名 bundle 不可替换，macOS 唯一可行的本地启动路径）。
- Completed the macOS local path: automatically copies the real language server from Devin.app into the data dir bin (.real copy) and installs the Codeium.codeium-dev extension shell so codeiumDev.languageServerBinaryPath takes effect (the signed bundle cannot be replaced; this is the only viable local startup path on macOS).

## [v1.4.1] - 2026-08-17

### 修复 / Fixes

- 修复「本地模型服务已启用但界面仍显示未启用」：启用时写入 Devin 设置的键集合与状态检查不一致（状态检查多出一个从不写入的 `security.workspace.trust.enabled`），导致导入状态永远判定为未导入（v1.3.0 引入）。
- Fixed "local model service enabled but UI still shows disabled": the settings keys written on enable and those checked by the status endpoint differed (the check included `security.workspace.trust.enabled`, which was never written), so the import state was always judged as not imported (introduced in v1.3.0).
- 修复 Windows 上启动与操作时闪现的终端窗口：子进程（taskkill / tasklist / cmd）启动时隐藏控制台窗口。
- Fixed flickering console windows on Windows startup and actions: child processes (taskkill / tasklist / cmd) now start with the console window hidden.
- 页头 Logo 更换为与窗口一致的图标（此前为占位图标）。
- The header logo now matches the window icon (previously a placeholder icon).

### 新增 / Features

- 恢复底栏「赞赏支持」按钮（v1.3.0 重写界面时被移除），点击弹出赞赏二维码弹窗。
- Restored the "Support" button in the footer (removed when the UI was rewritten in v1.3.0); clicking it opens the donation QR modal.
- **启用状态持久化**：启用过一次后，每次打开 GUI 自动恢复启用状态，无需再次手动点击「启用并一键导入」；显式点击「停止并恢复」后才彻底停止（下次启动不再自动启用）。
- **Persistent enable state**: after enabling once, the GUI automatically restores the enabled state on every launch without clicking "Enable & Import" again; only explicitly clicking "Stop & Restore" fully stops it (no auto-enable on the next launch).

## [v1.4.0] - 2026-08-16

### 新增 / Features

- **英文界面支持**：Web 控制台（监控 / 模型 / 提示词 / 设置）完整中英双语，默认跟随浏览器/系统语言；页头常驻「中 / EN」按钮随时切换，选择持久化到浏览器本地（localStorage）。
- **English UI support**: the Web console (Monitor / Models / Prompts / Settings) is fully bilingual; it follows the browser/system language by default, and a persistent "中 / EN" button in the header switches anytime, persisted to browser localStorage.
- 服务端管理 API 消息按请求语言返回（`X-Lang` / `Accept-Language`，缺省中文保持向后兼容）；更新弹窗按界面语言优先展示对应语言的版本说明。
- Server admin API messages are returned in the request language (`X-Lang` / `Accept-Language`, defaulting to Chinese for backward compatibility); the update modal prefers the version notes matching the UI language.

### 修复 / Fixes

- 升级时若新配置目录已存在"示例模板"（含占位域名 `api.example.com`），历史位置（`~/.devin-byok`）的用户配置此前不会被迁移，导致界面加载示例配置；现检测示例占位并自动迁移历史用户配置。
- On upgrade, if the new config directory already contained an example template (with placeholder domain `api.example.com`), the user config at the legacy location (`~/.devin-byok`) was not migrated, so the UI loaded the example config; the app now detects example placeholders and automatically migrates the legacy user config.
- WebView/浏览器缓存旧静态资源导致界面显示旧版（版本号、按钮、模型列表异常）；UI 静态资源 URL 增加版本查询参数强制重新加载。
- WebView/browser caching of old static assets showed a stale UI (version, buttons, model list); static asset URLs now carry a version query parameter to force reload.

### 工程与测试 / Engineering & Testing

- 新增前端 i18n 一致性校验脚本（`scripts/check-i18n.mjs`：字典对称、HTML/JS 引用覆盖、界面中文残留、服务端 uiMsg key 校验）与前端运行时单测（`scripts/test-i18n.mjs`：语言解析矩阵、切换链路、DOM 应用路径），并接入 CI 三平台门禁。
- Added a frontend i18n consistency check (`scripts/check-i18n.mjs`: dictionary symmetry, HTML/JS reference coverage, UI Chinese residue, server uiMsg key check) and runtime tests (`scripts/test-i18n.mjs`: language resolution matrix, switch flow, DOM application paths), wired into the CI three-platform gate.
- 语言切换后动态区域、页头版本副标题与页脚更新状态即时以新语言重渲染；模型家族卡片的编辑/复制/删除操作修复（按钮事件绑定）。
- Dynamic regions, the header version subtitle and footer update state re-render immediately in the new language on switch; family card edit/copy/delete actions fixed (button event binding).

### 平台支持 / Platform Support

- Windows（官方发布物，.exe，amd64/arm64） / Windows (official builds, .exe, amd64/arm64)
- macOS（官方发布物，.dmg，arm64/amd64） / macOS (official builds, .dmg, arm64/amd64)
- Linux（开发/CI 辅助支持，源码构建） / Linux (dev/CI auxiliary support, source builds)

## [v1.3.2] - 2026-08-16

### 安全与隐私

- RPC 抓包改为显式开关 `capture.enabled`，默认关闭（不再无条件落盘完整请求体）。
- 开启抓包时：文件权限收紧为 0600、记录脱敏（authorization / Bearer / api_key / sk-*）、启动时输出警告。
- 修复 protobuf 解析对超大长度 varint 的整数溢出，避免畸形/恶意请求触发 panic（DoS）。

### 工程与测试

- CI 增加 `go build ./...` 完整构建门禁、`-race` 竞态检测与 `go mod tidy` 一致性校验。
- 新增 pbwire 金标/fuzz 测试、ideinject 注入/幂等/恢复测试、扩展资源一致性断言。
- 移除开发期抓包脚本与硬编码本机路径的测试；机器相关抓包测试用 `manual` 构建标签隔离。
- 抓包逻辑从 `server.go` 抽离到 `capture.go`；修正 README 引用与 START.txt 编码。

### 平台支持

- Windows（官方发布物，.exe，amd64/arm64）
- macOS（官方发布物，.dmg，arm64/amd64）
- Linux（开发/CI 辅助支持，源码构建）


## [v1.3.1] - 2026-08-15

### 新增

- **Windows arm64 支持**：新增 windows-arm64 原生发布物（GOARCH=arm64 交叉编译），GUI 图标资源、language_server 定位与在线更新资产匹配均按架构区分。

### 平台支持

- Windows（官方发布物，.exe，amd64/arm64）
- macOS（官方发布物，.dmg，arm64/amd64）
- Linux（开发/CI 辅助支持，源码构建）

## [v1.3.0] - 2026-08-15

### 新增

- **macOS 平台支持**：新增 Devin BYOK.app 应用（WebView 窗口、Dock 图标、LaunchAgent 自启），发布物为 .dmg（arm64/amd64）。
- **Prompt Composer**：route/task/model 感知的提示词组装——稳定注入顺序、ID+内容双去重、优先级排序、replace 双重警告、旧 JSON 向后兼容。
- **Quality 模式**：fast / balanced / verified 三档提示词深度（默认 enabled + balanced），verified 走提示词+真实工具结果闭环。
- **本地运行时**：一键启动/停止/重启 Devin、本地伪账户免登录、聊天记录导出（zip）、服务状态一目了然。
- **控制面板 UI**：服务状态横幅、启停/重启/导出按钮、模型 Family 卡片、监控指标、提示词管理页。
- **模型身份统一应答**：询问"你是谁/什么模型"时本地直接返回当前模型名，不泄露路由/内部实现。
- **Anthropic thinking 支持**：thinking{type, budget_tokens} 请求体与校验；OpenAI/Grok reasoning_effort，provider 各自发送不串字段。
- **在线更新升级**：darwin/exe 安装包 + SHA256 校验，更新后自动重启。
- **CI/CD**：GitHub Actions 三平台（Windows/macOS/Linux）vet/build/test 矩阵；打 tag 自动打包 Windows exe 与 macOS dmg 并发布 GitHub Release。

### 修复

- macOS DMG 在线更新后 GUI 未自动重启（更新脚本缺少 GUI 变量）。
- Windows zip 更新路径失败时静默报"成功"（改为错误终止 + 复制校验 + 写锁超时）。
- 升级用户数据目录迁移丢失配置（恢复旧默认位置迁移候选）。
- darwin x64 资产命名不一致导致 Intel Mac 更新匹配失败。
- 管理 API 缺少跨站请求防护（新增 Origin/Referer 校验）。
- 身份提问启发式误拦截真实技术问题（如"模型为什么崩溃"）。
- 评测器短路：strict 模式误触发导致 reliability/verification profile 被跳过。
- 响应缓存 key 与流中断续传后的实际上下文不一致。
- UI 内联 onclick 双上下文转义失效（改为 data-* + addEventListener）。
- Unix 工作区外搜索防护失效（补 Unix 根提取与绝对路径识别）。
- 其他：compose 注入预算保护、newID 碰撞、评测代码执行沙箱、脚本健康检查虚假成功等。

### 平台支持

- Windows（官方发布物，.exe）
- macOS（官方发布物，.dmg，arm64/amd64）
- Linux（开发/CI 辅助支持，源码构建）

## [v1.2.9] - 2026-07

### 新增
- 会话标题独立模型管控与语言提示词注入。

## [v1.2.8] - 2026-06

### 新增
- Responses API 与简化模型配置。

---

> 更早版本见 GitHub Releases。
