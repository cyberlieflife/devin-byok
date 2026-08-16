# 更新日志

本文件记录 devin-byok 的主要版本变更。

## [v1.4.0] - 2026-08-16

### 新增

- **英文界面支持**：Web 控制台（监控 / 模型 / 提示词 / 设置）完整中英双语，默认跟随浏览器/系统语言；页头常驻「中 / EN」按钮随时切换，选择持久化到浏览器本地（localStorage）。
- 服务端管理 API 消息按请求语言返回（`X-Lang` / `Accept-Language`，缺省中文保持向后兼容）；更新弹窗按界面语言优先展示对应语言的版本说明。

### 修复

- 升级时若新配置目录已存在"示例模板"（含占位域名 `api.example.com`），历史位置（`~/.devin-byok`）的用户配置此前不会被迁移，导致界面加载示例配置；现检测示例占位并自动迁移历史用户配置。
- WebView/浏览器缓存旧静态资源导致界面显示旧版（版本号、按钮、模型列表异常）；UI 静态资源 URL 增加版本查询参数强制重新加载。

### 工程与测试

- 新增前端 i18n 一致性校验脚本（`scripts/check-i18n.mjs`：字典对称、HTML/JS 引用覆盖、界面中文残留、服务端 uiMsg key 校验）与前端运行时单测（`scripts/test-i18n.mjs`：语言解析矩阵、切换链路、DOM 应用路径），并接入 CI 三平台门禁。
- 语言切换后动态区域、页头版本副标题与页脚更新状态即时以新语言重渲染；模型家族卡片的编辑/复制/删除操作修复（按钮事件绑定）。

### 平台支持

- Windows（官方发布物，.exe，amd64/arm64）
- macOS（官方发布物，.dmg，arm64/amd64）
- Linux（开发/CI 辅助支持，源码构建）

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
