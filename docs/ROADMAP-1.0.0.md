# devin-byok 1.0.0 交付记录与遗留计划

> 分支：`1.0.0`（已推送 `origin/1.0.0`）
> 状态：核心功能全部完成并测试通过；编程能力量化评估因耗时长暂未跑完全量，见下文遗留项。

## 已完成

### 提交历史（origin/1.0.0）

| commit | 内容 |
|---|---|
| `f7e0bd1` | prompt-eval 真实执行编程题库（固定测试 + 陷阱题） |
| `99cbc8a` | 控制面板 UI：服务状态、一键重启、导出聊天、1.0.0 品牌 |
| `3560305` | 本地运行时引导、伪账户、进程控制、聊天导出、平台修复 |
| `afd4a19` | Compose 集成、prompt 预览 API、缓存指纹、模型身份统一回复 |
| `919f6e2` | route/task/model 感知的 Prompt Composer + 可靠性 profiles + 评估框架 |

### 功能清单

- 阶段一：`internal/promptstore/compose.go` — 稳定注入顺序（原始 system → 工具契约 → core-reliability → task/route profile → 工作区提示）、priority 排序、ID+内容双去重、replace 双重 warning、旧 JSON 向后兼容、`ApplyToMessages` 兼容包装。
- 阶段二：主聊天 / fast_context / deepwiki / codemap 四条路径独立 route；缓存指纹含 model+effort+prompt_hash+msgs+tools；metrics 只记 route/task/quality/profile_ids/hash，不记完整提示词。
- 阶段三：`config.Quality`（fast/balanced/verified），默认 enabled+balanced+1 轮；verified 第一版为提示词+真实工具结果闭环，无隐藏双模型。
- 阶段四：Anthropic `thinking{type, budget_tokens}` 请求体与校验（>0、<max_tokens、非法报错）；OpenAI/Grok `reasoning_effort`；provider 各自发送、不串字段。
- 阶段五：`GET /api/prompts/preview`（route/model/task/quality_mode → profile_ids/warnings/hash/estimated_tokens/截断 system 预览），与真实请求共用 Composer，不泄露 key 与完整上下文。
- 阶段六：spec 全部测试就位（含补齐的 `TestLegacyPromptJSONStillLoads`、`TestApplyToMessagesBackwardCompatible`、`TestDifferentProfilesDoNotShareCache`）。
- BYOK：本地伪账户凭证、一键启动/停止/重启 Devin、明显状态展示、导出聊天记录、模型身份问题统一回复当前模型名（`internal/localapi/model_identity.go`）。

### 验证

- `go test ./...` 全绿；`go vet ./...` 干净；`go test -race ./...` 全绿。
- 打包产物：
  - `dist/devin-byok-1.0.0-darwin-arm64.dmg`（6.0M）
    SHA256 `fe36ec2e457b54e7e93816e92f58d3e36703b2673cfe137e4bfdad475c066892`
  - `dist/devin-byok-1.0.0-windows-amd64.exe`（GUI，PE32+，含图标资源）
    SHA256 `046ba55a59188bebc79ab0b732eb669bedbbdee7185c071f435f6aa83c54fbb0`
  - `dist/devin-byok-1.0.0-windows-amd64-cli.exe`（调试用）
    SHA256 `706c020e6843ba96ea8be7f5b63faee2f4bb459c23babd3e0636fed5e4133ea6`

## 遗留计划（后续会话继续）

### P0 — 编程能力量化评估（40% 目标验证）

已完成题库改造：`cmd/prompt-eval` 现在把模型代码写入临时目录、用预置固定测试真实 `go test`，按通过率计分（正确性 40%/完成度 20%/证据 20%/理解 15%/幻觉 5%）。12 道陷阱题（merge-intervals 相邻不合并、round-half-away 负数、rotate 负 k、手写 atoi 溢出、LRU 访问序、CSV 引号转义等），参考实现在 `codecheck_test.go` 全满分自检通过。

**待执行**（约 20~40 分钟，48 次真实 API 调用，依赖本机 config.yaml 上游 key）：

```bash
export PATH=$HOME/sdk/go1.24.6/go/bin:$PATH
go run ./cmd/prompt-eval 2>&1 | grep -E '"case"|SUMMARY|RESULT'
# 可选：PROMPT_EVAL_ROUNDS=3 PROMPT_EVAL_MODELS=grok-4-5-byok-medium
```

判定标准（`docs/prompt-evaluation.md`）：paired ≥ 20 且 `(opt-base)/max(base,1) ≥ 0.40`。

若未达标，按数据迭代：
1. 找出 optimized 失分题目，缩小到具体 profile 文案（疑似长中文 profile 稀释注意力）；
2. 精简 coding-execution / verification / constraint-audit 的重复内容；
3. 必要时对编程 task 增加英文短契约段落（中英双语，中文为主）。
注意：评估记录不落盘模型原文，改文案后重跑同一命令对比。

### P1 — Windows 实机验收

- EXE 为 Mac 交叉编译产物，未在 Windows 实机运行过。需在 Win10/11 上验证：一键免登录、服务启停状态、Devin 拉取模型、重启生效、导出聊天。
- 如需原生打包：Windows 上跑 `scripts/pack-release.ps1`（结果与本目录 dist 一致）。

### P2 — UI 精修（可选，低风险）

- 当前面板已有状态横幅、启停/重启/导出按钮与渐变卡片样式。
- 可做：深色模式跟随系统、导出进度条、模型切换的确认弹窗。所有改动在 `internal/localapi/ui/`，改后需重打包（改动 CSS/JS 需重跑 Phase 4 打包）。

### P3 — 后续版本（按 spec「暂时不要做」）

- 隐藏双模型 reviewer（verified 模式的 `reviewer_model` 字段已预留）；
- 自动修改用户仓库 / 服务端任意 shell；
- 自动联网注入外部内容；
- 多 Agent 调度框架。

## 回归红线

- 不执行 `git reset` / `git checkout --`；
- 保留 system-prompts.json / config.yaml 向后兼容；
- 只追加不替换 Devin 原始 system prompt；
- 不把思维链、API key、完整用户请求写入日志/UI。
