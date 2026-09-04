---
key: write-plan
name: 写实施计划（Write Plan）
description: 把选定的方案落成可执行的实施计划：specs、design、tasks 三份产物，tasks 自动拆出带 stage 的子任务。
order: 2
---

# 写实施计划（Write Plan）

目标：为当前 change 依次产出 specs、design、tasks 三份产物，并在 tasks 写入后自动生成带 stage 的子任务。每个阶段写完产物后立即 `sp handoff` 推进，不要跳阶段。

## 前置条件

- change 已绑定（`sp open`），当前处于 specs、design 或 tasks 阶段。
- 用 `sp guard` 查看当前阶段与门禁状态。

## 步骤

1. **specs（需求规格）**：把 proposal 选定的方案拆成可验证的需求条目，每条写清验收标准。写入 `sp artifact write specs --file specs.md`，然后 `sp handoff`。
2. **design（技术设计）**：写技术设计——涉及的模块与文件、数据模型与迁移、接口契约。测试策略必须按 **TDD（测试驱动开发）** 严格限定：逐模块列出要先写的失败测试（先写测试、运行确认失败、再实现到通过），禁止"先实现后补测试"。写入 `sp artifact write design --file design.md`，然后 `sp handoff`。
3. **tasks（任务分解）**：把工作拆成可独立交付的任务，按依赖排进 stage（1 最早）。每个任务的 description 必须写明：TDD 约束（先写失败测试并确认失败，再写实现，直到测试通过）与验证命令（如 `go test ./...`）。tasks 产物必须是包含 ```json 围栏块的 markdown，格式：

   ```json
   {"tasks":[{"title":"...","description":"...（含 TDD 约束与验证命令）","stage":1}]}
   ```

   写入 `sp artifact write tasks --file tasks.md`，服务端会解析该 JSON 并为每个任务创建对应 stage 的子 issue，并写入 task mappings。

## 完成标准

- specs / design / tasks 三份产物齐全，change 停在 tasks 阶段。
- `sp guard` 显示 phase_legal 与 handoff_fresh 均为 true。
- 子任务已生成，可用 `sp open --issue <ID>` 查看 stage 分组。
