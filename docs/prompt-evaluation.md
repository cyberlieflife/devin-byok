# Prompt Optimization Evaluation

本项目不能用“回答更长”证明智力提升。使用同一模型、同一输入、同一工具权限和同一超时，分别跑 baseline 与 `quality.mode=balanced` 或 `verified`，按下面的固定维度评分。

## 评分

每题 0-4 分：

- 需求理解：是否识别目标、约束、完成标准。
- 正确性：结论或代码是否正确，是否处理边界条件。
- 完成度：是否真正执行修改/工具调用，而不是只给建议。
- 证据：是否给出真实文件、命令、测试和错误证据。
- 幻觉控制：是否避免虚构路径、结果、引用和能力。

综合分：正确性 40%、完成度 20%、证据 20%、需求理解 15%、幻觉控制 5%。

目标不是保证每次成功，而是优化后综合分相对 baseline 提升至少 40%：

`(optimized_score - baseline_score) / max(baseline_score, 1) >= 0.40`

## 固定题目

1. **代码实现**：实现一个函数，要求明确处理空输入、重复值和非法输入，并补测试；修改后运行格式检查和测试。
2. **调试**：给出一个会 panic 的最小 Go 示例，要求先复现、定位根因、修复并运行测试。
3. **代码审查**：审查一段会把用户输入拼进 shell 命令的代码，只报告有证据的风险并给出修复位置。
4. **研究**：比较两个 API 版本的 thinking 参数，要求引用官方文档并区分确认事实与推断。
5. **工具任务**：在工作区中查找一个符号，只返回实际存在的文件路径和行号，不允许猜路径或修改文件。
6. **反例任务**：实现“连续区间合并”，同时说明相邻区间是否合并，并测试空列表、重叠和相邻输入。

## 记录

每题记录：模型 ID、上游模型、reasoning 参数、quality mode、是否实际调用工具、测试命令、最终结果、耗时和 token。不要记录 API key、完整项目上下文或隐藏思维链。

只有在至少 10 题、两轮以上重复后，才把“提升 40%”当作可信结论。单次请求只能说明该样例的行为，不能证明普遍能力提升。

## 运行

评测器只输出分数、耗时和生效 profile，不保存模型原文或 API Key。baseline 与 optimized 使用同一模型、reasoning 参数、token 上限和超时，每轮交替调用顺序，并为每次调用创建独立超时上下文。

```bash
PROMPT_EVAL_MODELS="grok-4-5-byok-high,claude-opus-4-6-byok-medium,claude-opus-4-6-thinking-byok-medium" \
PROMPT_EVAL_ROUNDS=2 \
go run ./cmd/prompt-eval
```

快速冒烟可临时限制题数，但不能用于证明 40% 目标：

```bash
PROMPT_EVAL_MODEL="grok-4-5-byok-high" PROMPT_EVAL_ROUNDS=1 PROMPT_EVAL_CASE_LIMIT=2 go run ./cmd/prompt-eval
```

`target_reached=true` 仅在至少 20 个有效成对样本、两侧均无请求失败且相对提升达到 40% 时输出。它表示这套固定题库上的综合分提升，不代表通用智力提高 40%，也不保证每个现实任务都提升。
