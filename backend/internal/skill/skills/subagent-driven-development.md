---
key: subagent-driven-development
name: 子代理驱动开发（Subagent-Driven Development）
description: 按 stage 顺序把子任务派发给子代理执行，逐项验收，最后走 verify 与 archive 门禁收尾。
order: 3
---

# 子代理驱动开发（Subagent-Driven Development）

目标：把 tasks 产物拆出的子任务真正做完：按 stage 顺序派发、逐项验收、最终 verify + archive。

## 前置条件

- change 处于 tasks 阶段，`sp open --issue <ID>` 已能看到 stage 分组的子任务。

## 步骤

1. **按 stage 顺序执行**：从 stage 1 开始，同一 stage 内的任务可以并行派发给子代理；禁止把某个 stage 的任务与更晚 stage 的任务并行推进（stage 内禁并行晋升）。
2. **派发子代理（严格 TDD）**：每个子任务派给一个子代理，任务描述必须带上 TDD 约束：先写测试并运行确认失败，再写实现，反复直到测试全部通过——禁止先实现后补测试。子代理完成报告必须附测试先行的证据（失败输出与最终通过输出），无证据视为未完成。
3. **逐项验收**：对照任务描述与验收标准核验子代理的结果：测试确实先失败后通过、实现最小、无绕过测试的捷径；核验不过就打回重做。验收通过后将该子任务置为完成。
4. **晋升下一 stage**：当前 stage 全部完成后才进入下一 stage。
5. **验证收尾**：全部 stage 完成后，运行整体构建与测试，记录检查结果：
   - `sp state record-check build --command "<构建命令>" --exit-code <N>`
   - `sp state record-check verify --command "<测试命令>" --exit-code <N>`
   - 或 `sp verify --file report.yaml`（YAML：`result: pass|fail` + `summary:`）。
6. **归档**：`sp guard` 确认 can_archive 为 true 后执行 `sp archive`；归档会唤醒父 issue 的负责人做验收。

## 完成标准

- 所有子任务完成，各 stage 依次晋升。
- verify 报告 result 为 pass。
- change 已归档。
