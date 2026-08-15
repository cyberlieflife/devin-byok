# 更新日志

本文件记录 devin-byok 的主要版本变更。

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
