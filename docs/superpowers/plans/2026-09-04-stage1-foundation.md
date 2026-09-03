# Stage 1 基础框架实施计划（SP-12）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 搭建可运行的基础框架：Go 后端 + PostgreSQL + 认证（JWT + 项目级 owner/member 权限）+ React/Vite 前端骨架。

**Architecture:** 单体两段式 —— `backend/` 为 Go 服务（chi 路由、pgx 连接池、内嵌 SQL 迁移运行器、接口化 Store 层），`frontend/` 为 Vite + React + TS SPA（react-router 守卫 + fetch API client）。前后端通过 `/api/v1` REST + Bearer JWT 通信。

**Tech Stack:** Go 1.27、chi v5、jackc/pgx v5、golang-jwt/jwt v5、golang.org/x/crypto/bcrypt；React 18 + Vite + TypeScript + react-router-dom + vitest/@testing-library；PostgreSQL 16（docker-compose 提供）。

**Spec:** Multica issue SP-12（Stage 1），父 issue SP-11 定稿决策：Go+React+Vite、PostgreSQL、小团队多用户、权限按项目管理、严格 TDD、不引入 comet/multica/superpowers/openspec 代码、CLI 统一 `sp`（CLI 属 SP-19，本期不做）。

## Global Constraints

- 严格 TDD：每个可测单元先写失败测试，再实现，再转绿，再提交。
- 数据库仅 PostgreSQL；迁移必须可重复执行（已应用的版本跳过）。
- 错误统一信封：`{"error":{"code":"<code>","message":"<msg>"}}`。
- API 前缀 `/api/v1`；JWT 走 `Authorization: Bearer <token>`。
- 所有命令行工具统一命名 `sp`（本期仅服务端二进制 `spd`，`sp` CLI 由 SP-19 交付）。
- 不引入 comet/multica/superpowers/openspec 的任何代码。
- 本机无 PostgreSQL/Docker：依赖真实 PG 的集成测试以 `SP_TEST_PG_DSN` 环境变量守卫，未设置时 `t.Skip`；单元测试全部用 fake/mock，本地可全绿。
- 提交信息 conventional commits（feat/test/chore/docs）。

## 文件结构

```
backend/
  go.mod                          module specpowers/backend
  cmd/spd/main.go                 服务端入口
  internal/config/config.go       环境变量配置加载
  internal/httpapi/
    errors.go                     AppError + 统一信封
    respond.go                    JSON 响应工具
    router.go                     路由装配 + 健康检查
    middleware.go                 recover / logger / auth 中间件
  internal/domain/models.go       User/Workspace/Member/Role/Project
  internal/store/store.go         Store 接口定义
  internal/store/postgres/
    pool.go                       pgxpool 连接管理
    migrate.go                    迁移运行器（schema_migrations 记账）
    migrations/0001_init.sql      初始表结构（embed）
    users.go workspaces.go members.go projects.go   SQL 实现
  internal/auth/
    token.go                      JWT 签发/校验
    service.go                    注册/登录业务（bcrypt + 默认工作区）
    handler.go                    /auth/register /auth/login /me
  internal/project/
    service.go                    项目创建/查询 + owner/member 权限判定
    handler.go                    /projects /projects/{id}/members
frontend/
  package.json vite.config.ts tsconfig.json index.html
  src/main.tsx src/App.tsx
  src/api/client.ts               fetch 封装 + 401 处理
  src/auth/AuthContext.tsx        登录态存储/登录/注册/登出
  src/components/RequireAuth.tsx  守卫
  src/components/Layout.tsx       基础布局
  src/pages/LoginPage.tsx ProjectsPage.tsx
  src/**/*.test.tsx               vitest 测试
docker-compose.yml                开发用 PostgreSQL 16
README.md                         启动/测试说明
```

### Task 1: 配置加载

**Files:** Create `backend/go.mod`、`backend/internal/config/config.go`；Test `backend/internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Load(getenv func(string) string) (Config, error)`；`Config{Addr, DatabaseURL, JWTSecret, Env string}`。

- [ ] Step 1 失败测试：未设 JWT_SECRET 且 Env=production 时报错；默认值 Addr=":8080"、Env="dev"、开发默认 DatabaseURL/JWTSecret。
- [ ] Step 2 运行确认失败（Load 未定义）。
- [ ] Step 3 实现 Load。
- [ ] Step 4 测试转绿；Step 5 `go mod tidy` + commit `feat(backend): config loading with env validation`。

### Task 2: 统一错误 + 健康检查 + 路由骨架

**Files:** Create `internal/httpapi/errors.go|respond.go|router.go|middleware.go`；Test `internal/httpapi/router_test.go|errors_test.go`

**Interfaces:**
- Produces: `type AppError struct{ Status int; Code, Message string }`；`httpapi.NewRouter(deps Deps) http.Handler`，`Deps{Auth *auth.Handler, Project *project.Handler}`（Task 4 前用 nil 占位）；`respond.JSON(w, status, v)`；`respond.Error(w, *AppError)`；健康检查 `GET /api/v1/health` → `{"status":"ok"}`。
- AppError 常量：`ErrInvalid(400,invalid_request)` `ErrUnauthorized(401,unauthorized)` `ErrForbidden(403,forbidden)` `ErrNotFound(404,not_found)` `ErrConflict(409,conflict)` `ErrInternal(500,internal)`。

- [ ] Step 1 失败测试：errors 信封 JSON 断言；router 请求 `/api/v1/health` 得 200 `{"status":"ok"}`；未知路由得统一信封 404；handler panic 被 recover 中间件转为 500 信封。
- [ ] Step 2 确认失败 → Step 3 实现 → Step 4 转绿 → Step 5 commit `feat(backend): http skeleton with unified errors and health check`。

### Task 3: 迁移运行器（可重复执行）

**Files:** Create `internal/store/postgres/migrate.go`、`migrations/0001_init.sql`；Test `migrate_test.go`

**Interfaces:**
- Produces: `type DBTX interface{ Exec(context.Context, string, ...any) (pgconn.CommandTag, error); QueryRow(...); Query(...) }`；`func Migrate(ctx context.Context, db DBTX, fsys fs.FS) error` —— 按文件名升序执行未应用脚本，`schema_migrations(version text PRIMARY KEY, applied_at timestamptz)` 记账，重复调用幂等。
- 0001_init.sql：users(id uuid pk default gen_random_uuid(), email citext unique not null, password_hash text, display_name text, created_at/updated_at)；workspaces(id, name, created_by fk users)；members(id, workspace_id fk, user_id fk, role_id fk, unique(workspace_id,user_id))；roles(id smallint pk, name unique) + seed (1,owner)(2,member)；projects(id, workspace_id fk, name, created_by fk)；project_members(id, project_id fk, user_id fk, role text check in('owner','member'), unique(project_id,user_id))。`CREATE EXTENSION IF NOT EXISTS pgcrypto; CREATE EXTENSION IF NOT EXISTS citext;`。

- [ ] Step 1 失败测试（fake DBTX 记录 Exec）：空 FS 不建表即返回；两个脚本按序执行；fake 预填已应用版本后第二次调用跳过该脚本（幂等核心逻辑）。
- [ ] Step 2 确认失败 → Step 3 实现运行器 → Step 4 转绿 → Step 5 commit `feat(backend): idempotent sql migration runner`。
- [ ] Step 6 另建守卫测试 `migrate_integration_test.go`：`SP_TEST_PG_DSN` 未设则 Skip；设了则连真库跑两遍 Migrate 验证幂等（本地 Skip 属预期）。

### Task 4: pgx 连接池 + Store 接口与 Postgres 实现

**Files:** Create `internal/store/store.go`、`internal/domain/models.go`、`internal/store/postgres/pool.go|users.go|workspaces.go|members.go|projects.go`；Test `store 每实现配 _test.go`（守卫式集成）+ fake 单测业务层

**Interfaces:**
- Produces（store 包）:
```go
type UserStore interface {
  CreateUser(ctx context.Context, u *domain.User) error
  GetUserByEmail(ctx context.Context, email string) (*domain.User, error) // 未找到返回 store.ErrNotFound
  GetUser(ctx context.Context, id string) (*domain.User, error)
}
type WorkspaceStore interface {
  CreateWorkspace(ctx context.Context, w *domain.Workspace) error
}
type MemberStore interface {
  AddMember(ctx context.Context, m *domain.Member) error
  ListWorkspaceIDsForUser(ctx context.Context, userID string) ([]string, error)
}
type ProjectStore interface {
  CreateProject(ctx context.Context, p *domain.Project) error
  GetProject(ctx context.Context, id string) (*domain.Project, error)
  ListProjectsForUser(ctx context.Context, userID string) ([]domain.Project, error)
  AddProjectMember(ctx context.Context, pm *domain.ProjectMember) error
  GetProjectMember(ctx context.Context, projectID, userID string) (*domain.ProjectMember, error)
}
```
- `pool.New(ctx, dsn) (*pgxpool.Pool, error)`；`ErrNotFound` 哨兵错误。
- domain 模型：`User{ID,Email,PasswordHash,DisplayName,CreatedAt,UpdatedAt}`、`Workspace{ID,Name,CreatedBy}`、`Member{WorkspaceID,UserID,RoleID}`、`Project{ID,WorkspaceID,Name,CreatedBy}`、`ProjectMember{ProjectID,UserID,Role}`。
- SQL 实现的测试：`SP_TEST_PG_DSN` 守卫式（迁移 + 事务回滚断言），本地全 Skip；业务层（auth/project service）用 fake 全测。

- [ ] Step 1 定义 domain + 接口（纯类型，编译验证）→ commit `feat(backend): domain models and store interfaces`。
- [ ] Step 2 写守卫式集成测试（Register/GetByEmail/冲突 email/迁移后种子 roles）→ 实现 pool + SQL → 本地确认 Skip 生效、`go vet` 干净 → commit `feat(backend): postgres store implementations`。

### Task 5: JWT token 服务

**Files:** Create `internal/auth/token.go`；Test `token_test.go`

**Interfaces:**
- Produces: `func NewTokenService(secret string, ttl time.Duration) *TokenService`；`(t) Issue(userID string) (string, error)`；`(t) Verify(tokenStr string) (userID string, error)`（过期/篡改/错签名返回错误）。

- [ ] Step 1 失败测试：签发→校验往返；过期 token 拒绝；篡改 payload 拒绝；错误 secret 签的 token 拒绝。
- [ ] Step 2~4 TDD 循环 → Step 5 commit `feat(backend): jwt token service`。

### Task 6: 注册/登录 service + handler + 认证中间件

**Files:** Create `internal/auth/service.go|handler.go`；Modify `internal/httpapi/middleware.go`（加 `RequireAuth`）、`router.go`（装配真实路由）；Test `service_test.go|handler_test.go|middleware_test.go`

**Interfaces:**
- Produces: `auth.NewService(users store.UserStore, workspaces store.WorkspaceStore, members store.MemberStore, tokens *TokenService) *Service`；
  - `Register(ctx, email, password, displayName) (*domain.User, error)`：校验 email 格式、密码 ≥8 位；重复 email → `ErrConflict`；成功后创建默认工作区（名 = displayName）并以 role owner 加为成员（同一事务语义由 service 顺序保证）。
  - `Login(ctx, email, password) (token string, user *domain.User, error)`：bcrypt 比对失败/用户不存在 → `ErrUnauthorized`（不区分，防枚举）。
  - handler：`POST /api/v1/auth/register` 201 `{user:{id,email,display_name}}`；`POST /api/v1/auth/login` 200 `{token, user}`；`GET /api/v1/me`（需认证）200 `{user}`。
  - `httpapi.RequireAuth(tokens *auth.TokenService)`：解析 Bearer，失败 401 信封；将 `user_id` 放入 `context.Context`（key: `httpapi.CtxUserID`，`httpapi.UserIDFrom(ctx) string`）。
- 测试策略：service 用 fake UserStore/WorkspaceStore/MemberStore 全单元覆盖（重复注册、密码短、登录失败、默认工作区创建）；handler 用 `httptest` + 真 TokenService + fake store 覆盖各状态码。

- [ ] Step 1 service 失败测试 → 实现 → 转绿 → commit `feat(backend): register/login service`。
- [ ] Step 2 handler + RequireAuth 失败测试 → 实现 → 转绿 → commit `feat(backend): auth handlers and middleware`。

### Task 7: 项目级权限基础实现

**Files:** Create `internal/project/service.go|handler.go`；Test `service_test.go|handler_test.go`

**Interfaces:**
- Consumes: store.ProjectStore、auth handler 的用户注入。
- Produces:
  - `project.NewService(projects store.ProjectStore, users store.UserStore, workspaces store.MemberStore) *Service`。
  - `CreateProject(ctx, userID, name) (*domain.Project, error)`：取用户第一个工作区（无则建默认工作区），建项目 + project_members(role=owner)。
  - `RequireProjectRole(ctx, userID, projectID, minRole)`：member < owner；owner 要求时 member → `ErrForbidden`；项目不存在 → `ErrNotFound`。
  - `ListProjects(ctx, userID)`：用户可见项目（本人为 project 成员或所在工作区 owner——本期实现为"本人是 project 成员"）。
  - handler：`POST /api/v1/projects` 201 `{project}`；`GET /api/v1/projects` 200 `{projects:[...]}`；`POST /api/v1/projects/{id}/members`（仅 owner）201 `{member}`。
- 测试：service 用 fake ProjectStore/MemberStore 全覆盖（非成员 forbiddent、owner 通过、member 升权拒绝）；handler httptest 覆盖。

- [ ] Step 1 service 失败测试 → 实现 → 转绿 → commit `feat(backend): project service with owner/member permission`。
- [ ] Step 2 handler 失败测试 → 实现 → 转绿 → commit `feat(backend): project handlers`。

### Task 8: main 装配 + 全量后端回归

**Files:** Create `backend/cmd/spd/main.go`

- [ ] Step 1：main 读 config → 连池 → Migrate → NewRouter → `http.ListenAndServe`；优雅关闭（signal → Shutdown）。
- [ ] Step 2：`go build ./... && go vet ./... && go test ./...` 全绿 → commit `feat(backend): wire server entrypoint`。

### Task 9: 前端骨架 + API client（TDD）

**Files:** Create `frontend/package.json vite.config.ts tsconfig.json index.html src/main.tsx src/App.tsx src/api/client.ts src/setupTests.ts`；Test `src/api/client.test.ts`

**Interfaces:**
- Produces: `apiFetch<T>(path, opts?)`：baseURL=`import.meta.env.VITE_API_BASE ?? "/api/v1"`；自动带 `Authorization: Bearer <token>`（token 由 auth 模块存 localStorage `sp_token`）；`res.ok===false` 时抛 `ApiError{status, code, message}`（解析统一信封）；401 时清除本地登录态并派发 `sp:unauthorized` 事件（App 监听跳 /login）。
- React 18 + react-router-dom v6；vitest + jsdom + @testing-library/react。

- [ ] Step 1 手写脚手架文件（package.json 等）→ `npm install`。
- [ ] Step 2 client 失败测试：注入 fetch mock 断言（带 token 请求头 / 信封解析为 ApiError / 401 清 token）→ 实现 client → 转绿 → commit `feat(frontend): api client with auth header and error envelope`。

### Task 10: 登录态、登录页、守卫、布局、受保护页

**Files:** Create `src/auth/AuthContext.tsx src/pages/LoginPage.tsx src/pages/ProjectsPage.tsx src/components/RequireAuth.tsx src/components/Layout.tsx src/App.tsx src/main.tsx`；Test `AuthContext.test.tsx RequireAuth.test.tsx LoginPage.test.tsx`

**Interfaces:**
- `AuthProvider`：状态 `{user, token}`（持久化 localStorage `sp_token`/`sp_user`），`login(email,pwd)` 调 `POST /auth/login`、`register(...)`、`logout()` 清态。
- `RequireAuth`：无 user → `<Navigate to="/login" replace>`；有 → `<Outlet/>`。
- 路由：`/login` 公开；`/` → RequireAuth → Layout → `/`（ProjectsPage，拉 `GET /projects`）。
- Layout：顶栏（应用名 + 当前用户 + 退出按钮）+ 内容区。
- ProjectsPage：加载项目列表、空态、加载失败提示。

- [ ] Step 1 AuthContext 失败测试（login 成功存 token/user；失败抛出且不存）→ 实现 → 转绿。
- [ ] Step 2 RequireAuth 失败测试（未登录重定向 /login）→ 实现 → 转绿。
- [ ] Step 3 LoginPage 失败测试（提交调用 login；错误显示信封 message）→ 实现 + 布局/受保护页 → 转绿 → commit `feat(frontend): auth flow with guarded routes and layout`。

### Task 11: 收尾 —— docker-compose、README、全量回归

**Files:** Create `docker-compose.yml README.md`（仓库根）

- [ ] Step 1 docker-compose：postgres:16-alpine，库/用户 specpowers/specpowers，5432，健康检查；volume 持久化。
- [ ] Step 2 README：本地启动（compose 起库 → `go run ./cmd/spd` → `npm run dev`）、测试命令、`SP_TEST_PG_DSN` 集成测试说明、默认端口。
- [ ] Step 3 全量回归：`go test ./...`、`go vet ./...`、`cd frontend && npm test -- --run && npm run build` 全绿。
- [ ] Step 4 commit `chore: dev compose, readme, final regression`。

## Self-Review 结论

- 范围覆盖：骨架/配置/REST/错误/健康（Task 1-2）、PostgreSQL 接入/迁移/表结构（Task 3-4）、认证 + 项目级权限（Task 5-7）、前端骨架全部要件（Task 9-10）、迁移可重复执行（Task 3 幂等 + 守卫集成测试）、TDD（各 Task 步骤即红绿循环）——SP-12 验收逐条有对应任务。
- 占位符扫描：无 TBD/TODO；所有 Interfaces 块给出真实签名。
- 类型一致性：Store 接口签名在 Task 4 定义，Task 6/7 消费同名方法；`ErrConflict/ErrUnauthorized` 等复用 httpapi.AppError 语义，service 层直接返回 `*httpapi.AppError`。
