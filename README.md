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
| `SP_ATTACHMENT_DIR` | `data/attachments` | issue 附件的本地存储目录 |

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
| POST | `/projects/{id}/issues` | 创建 issue（成员；支持 parent_id / stage，见下） |
| GET | `/projects/{id}/issues` | issue 列表（成员；查询参数 status / stage / parent=root） |
| GET | `/projects/{id}/issues/{iid}` | issue 详情 |
| PATCH | `/projects/{id}/issues/{iid}` | 更新标题/描述/优先级/指派人/截止日期/标签/stage/position/parent |
| DELETE | `/projects/{id}/issues/{iid}` | 删除 issue（级联删除子 issue） |
| POST | `/projects/{id}/issues/{iid}/status` | 看板状态流转（body: `{status}`，校验状态机） |
| GET | `/projects/{id}/issues/{iid}/children` | 子 issue 列表（按 stage、position 排序） |
| POST | `/projects/{id}/issues/{iid}/comments` | 发表评论（body: `{content, parent_id?}`；parent_id 为根评论 id 即回复，线程单层） |
| GET | `/projects/{id}/issues/{iid}/comments` | 评论列表（按时间排序，parent_id 标记所属线程） |
| POST | `/projects/{id}/issues/{iid}/attachments` | 上传附件（multipart `file`，可选 `comment_id` 关联评论；单文件上限 20MB） |
| GET | `/projects/{id}/issues/{iid}/attachments` | 附件列表 |
| GET | `/projects/{id}/issues/{iid}/attachments/{aid}` | 下载附件内容 |
| GET | `/projects/{id}/issues/{iid}/metadata` | 元数据 KV 列表（按 key 排序） |
| PUT | `/projects/{id}/issues/{iid}/metadata/{key}` | 设置元数据（upsert，body: `{value, type?}`；type: string/number/bool，缺省 string） |
| DELETE | `/projects/{id}/issues/{iid}/metadata/{key}` | 删除元数据 |
| GET | `/changes?issue_id={iid}` | issue 的工作流实例（classic 拆分，一 issue 一 change） |
| GET | `/changes/{cid}` | change 详情（project_id / issue_id / phase / status） |
| GET | `/changes/{cid}/artifacts` | 产物列表（每类 kind 的最新版本，按 proposal→specs→design→tasks 排序） |
| GET | `/changes/{cid}/artifacts/{kind}` | 读取产物 markdown（kind: proposal/specs/design/tasks/verify；`?version=N` 指定版本，缺省最新） |
| GET | `/changes/{cid}/tasks` | tasks 条目与子 issue 的映射（按 stage、position 排序） |
| GET | `/changes/{cid}/guard` | 门禁评估报告（phase 合法性 / handoff 新鲜度 / verify 通过 / 可推进 / 可归档，附未通过原因） |
| POST | `/changes/{cid}/guard` | 门禁推进：校验通过后进入下一 phase 并写入 handoff 记录 |
| POST | `/changes/{cid}/verify` | 提交 verify 报告（body: `{content}`，YAML；`result` 必须为 pass/fail，最新报告为 pass 才放行归档） |
| POST | `/changes/{cid}/archive` | 归档 change（要求 active、tasks 阶段、产物链自洽、handoff 新鲜、verify pass；归档后按父子对唤醒父 issue 负责人验收） |

### Issue 状态机

状态：`backlog / todo / in_progress / in_review / done / blocked / cancelled`，其中 `done` 与 `cancelled` 为终态。

合法流转：backlog→todo；todo→in_progress/blocked；in_progress→in_review/blocked/todo；in_review→done/in_progress；blocked→in_progress/todo；任意非终态→cancelled。其余流转（含终态出入）返回 400。

子 issue 全部到达终态时，父 issue 记录一条唤醒（`issue_wakeups`，按父子对幂等），供后续 agent 运行时消费。

### Classic 门禁（guard / verify / archive）

change 的 phase 顺序为 proposal→specs→design→tasks，AI 拆分器每次推进都会写入 handoff 记录（迁移 `0007_change_gates`）。门禁规则：

- **phase 合法性**：当前 phase 及其之前的每个 phase 都必须有产物（禁止非法空跳）；
- **handoff 新鲜度**：进入当前 phase 必须有对应的 handoff 记录，且此前各 phase 的产物未在 handoff 之后重新生成（过期即拦截）；
- **verify 报告**：`POST /changes/{cid}/verify` 提交 YAML 报告，`result` 必须为 `pass` 或 `fail`；最新一份报告为 pass 才放行归档；
- **归档**：全部门禁通过后 `POST /changes/{cid}/archive` 置 change 为 archived，并联动父 issue 收尾——若 change 所属 issue 存在父 issue，按 Multica 一致的规则记录父唤醒，由父 issue 负责人确认验收。

错误统一信封：`{"error":{"code":"...","message":"..."}}`。

## 数据库迁移

迁移脚本内嵌于 `backend/internal/store/postgres/migrations/`，服务启动时自动执行；通过 `schema_migrations` 表记账，重复执行自动跳过已应用版本。
