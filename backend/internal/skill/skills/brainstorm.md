---
key: brainstorm
name: 需求探索（Brainstorm）
description: 在动手之前探索需求：澄清目标、盘点约束、比较可选方案，并沉淀为 proposal 产物。
order: 1
---

# 需求探索（Brainstorm）

目标：把一个模糊的想法变成一份可评审的 proposal，写入当前 change 的 proposal 产物。不要在此阶段写实现代码。

## 前置条件

- 已登录 sp CLI（`sp login`）。
- 用 `sp open --issue <ID> --manual` 为目标 issue 创建（或绑定）一个 change，并绑定本地状态。

## 步骤

1. **理解目标**：复述你对目标的理解，指出 issue 描述中含糊或矛盾的地方。
2. **澄清关键决策**：只问会改变方案走向或验收标准的问题；能自己决定的事项直接决定并说明理由，不要逐条求确认。
3. **盘点约束**：技术栈、既有代码结构、不引入外部代码的约定、时间与风险。
4. **比较方案**：给出 2-3 个候选方向，每个写明取舍（代价 / 收益 / 风险），并给出推荐。
5. **写 proposal**：把目标、约束、被否决的备选方案及否决理由、选定方案整理成 markdown，保存为文件。
6. **提交产物**：`sp artifact write proposal --file proposal.md`，成功后执行 `sp handoff` 进入 specs 阶段。

## 完成标准

- proposal 产物已写入，包含目标、约束、方案比较与推荐理由。
- change 已通过 `sp handoff` 进入 specs 阶段。
