
<!-- 项目约束（人工维护，优先于默认行为） -->

# Project Conventions

## Go 错误处理

- 错误断言一律使用 `errors.As(err, &target)` 或泛型形式 `errors.AsType[*T](err)`（本仓库 Go 1.27 支持），使 wrapped error 链上的错误也能被正确识别；**禁止**直接类型断言 `err.(*T)` 或逗号-ok 断言 `v, ok := err.(*T)`——包装过的错误会被漏判。
- 错误判等使用 `errors.Is`。
- 参考提交：backend/internal/agent/flow.go、backend/internal/auth/handler.go 中的既有写法。
