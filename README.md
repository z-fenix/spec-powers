# spec-powers

从零实现的 Multica 类协作平台：issue 发布后由 AI/LLM 严格按 classic 流程拆分子任务，产物入库并与子任务关联。

- 技术栈：Go + React + Vite + PostgreSQL
- 服务端二进制：`spd`（命令行工具 `sp` 由后续 Stage 交付）

## 目录结构

```
backend/    Go 服务端（internal/{config,httpapi,auth,project,store,domain}）
frontend/   React + Vite 前端
docs/       实施计划等文档
```

## 本地启动

```bash
# 1. 启动 PostgreSQL
docker compose up -d

# 2. 启动后端（默认 :8080，自动执行数据库迁移，可重复运行）
cd backend && go run ./cmd/spd

# 3. 启动前端（Vite dev server，/api 代理到 :8080）
cd frontend && npm install && npm run dev
```

打开 http://localhost:5173 ，注册账号并登录即可进入受保护的项目列表页。

### 环境变量（后端）

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `SP_ADDR` | `:8080` | HTTP 监听地址 |
| `SP_DATABASE_URL` | `postgres://specpowers:specpowers@localhost:5432/specpowers?sslmode=disable` | PostgreSQL 连接串 |
| `SP_JWT_SECRET` | dev 默认值 | JWT 签名密钥；`SP_ENV=production` 时必填 |
| `SP_ENV` | `dev` | 运行环境 |

## 测试

```bash
# 后端单元测试（无需数据库）
cd backend && go test ./...

# 后端集成测试（需要真实 PostgreSQL，如 compose 起的实例）
docker compose up -d
cd backend && SP_TEST_PG_DSN="postgres://specpowers:specpowers@localhost:5432/specpowers?sslmode=disable" go test ./...

# 前端测试与构建
cd frontend && npm test -- --run && npm run build
```

## API 一览（/api/v1）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/health` | 健康检查 |
| POST | `/auth/register` | 注册（创建默认工作区并成为 owner） |
| POST | `/auth/login` | 登录，返回 JWT |
| GET | `/auth/me` | 当前用户（需 Bearer token） |
| POST | `/projects` | 创建项目（创建者成为项目 owner） |
| GET | `/projects` | 我参与的项目列表 |
| GET | `/projects/{id}` | 项目详情（成员） |
| PATCH | `/projects/{id}` | 更新名称/描述（仅 owner） |
| POST | `/projects/{id}/archive` | 归档/恢复项目（仅 owner，body: `{archived: bool}`） |
| POST | `/projects/{id}/members` | 添加项目成员（仅 owner，role: owner/member） |
| GET | `/projects/{id}/resources` | 资源绑定列表（成员） |
| POST | `/projects/{id}/resources` | 绑定资源（仅 owner，type: github_repo/local_directory） |
| DELETE | `/projects/{id}/resources/{rid}` | 移除资源绑定（仅 owner） |
| GET | `/projects/{id}/context` | 读取项目上下文（成员） |
| PUT | `/projects/{id}/context` | 写入项目上下文（仅 owner） |

错误统一信封：`{"error":{"code":"...","message":"..."}}`。

## 数据库迁移

迁移脚本内嵌于 `backend/internal/store/postgres/migrations/`，服务启动时自动执行；通过 `schema_migrations` 表记账，重复执行自动跳过已应用版本。
