# spec-powers

从零实现的 Multica 类协作平台：issue 发布后由 AI/LLM 严格按 classic 流程拆分子任务，产物入库并与子任务关联。

- 技术栈：Go + React + Vite + PostgreSQL
- 服务端二进制：`spd`，命令行工具：`sp`

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
| `SP_STATIC_DIR` | 未设置 | 前端静态目录（`frontend/dist`）；设置后 spd 同端口托管前端（SPA 回退 index.html，`/api` 优先）。部署详见 `docs/deploy.md` |
| `SP_LLM_API_KEY` / `SP_LLM_MODEL` | 未设置 | OpenAI 兼容接口的密钥与模型；两者都设置后 AI 拆分才启用 |
| `SP_LLM_BASE_URL` | `https://api.openai.com/v1` | OpenAI 兼容接口地址 |
| `SP_LLM_PROMPT_DIR` | 内置默认 | 提示词模板目录（`<kind>.md` 覆盖对应 phase） |

## sp 命令行

`sp` 是工作流命令的唯一入口，全部通过 REST API 对接服务端，产物直接入库：

```bash
cd backend && go build -o sp.exe ./cmd/sp   # 或 go run ./cmd/sp

# 登录（token 存入 .specpower/session.json）
sp login --server http://localhost:8080 --email me@example.com --password *** [--register]

# 打开 change：issue 已拆分则绑定，否则触发 AI classic 拆分
sp open --issue <issue-id>

# 手动打开 change：不跑 AI 拆分，由 agent 技能流程自己产出各阶段产物
sp open --issue <issue-id> --manual

# 列出技能包（superpowers 流程：brainstorm → write-plan → subagent-driven-development）
sp skills

# 加载技能指令（agent 按指令驱动流程）
sp skill <key>

# 解析当前 change 应执行的下一个技能（proposal→brainstorm；specs/design→write-plan；tasks→subagent-driven-development）
sp next-skill [--change <change-id>]

# 手动写入产物（tasks 会解析 JSON 围栏块并自动创建带 stage 的子 issue 与任务映射）
sp artifact write <proposal|specs|design|tasks> --file plan.md
sp artifact write tasks --content "$(cat tasks.md)"

# 门禁：打印报告；无可推进且无可归档时以非零码退出
sp guard [--change <change-id>]

# 推进到下一 phase（写入 handoff 记录）
sp handoff [--change <change-id>]

# 记录命令检查：build 仅记录在 .specpower/state.json；
# verify 同时生成 YAML 报告（exit 0 → result: pass）提交服务端
sp state record-check <build|verify> --command "go test ./..." --exit-code 0

# 提交 verify 报告（YAML，--file / --content / stdin）
sp verify --file verify.yaml

# 归档 change（门禁全过后，按父子对唤醒父 issue 负责人）
sp archive [--change <change-id>]
```

- change 定位：`--change` 优先，否则使用 `.specpower/state.json` 里 `sp open` 绑定的 change。
- 认证：`--token` > 环境变量 `SP_TOKEN` > `.specpower/session.json`；服务器地址同理（`--server` > `SP_SERVER` > session）。
- 输出：默认人类可读，任意命令加 `--json` 输出结构化结果。
- 退出码：`0` 成功；`1` 门禁未过或 API 错误；`2` 用法错误。

### 本机 agent（注册 / 本地运行时）

```bash
# 在服务器上注册本机 agent（runtime=local），运行凭证存入 ~/.sp/agents/<name>.json
sp agent register --name worker [--description "本机工作机"] [--force] [SKILL...]

# 前台轮询模式：领取指派给本 agent 的 run，本地执行 LLM 工具循环，
# 评论 / 状态更新 / run 日志回传服务器（需要 SP_LLM_API_KEY / SP_LLM_MODEL）
sp agent run [--name worker] [--once] [--poll 3s] [--workdir PATH]

# 注销：删除服务器上的 agent（凭证随之失效）并清除本地凭证
sp agent deregister [--name worker]
```

- 领取幂等：服务端 `POST /api/v1/runtime/claim` 以 `FOR UPDATE SKIP LOCKED` 原子领取，同一 run 不会被两个运行时同时执行。
- 多 agent 协调：多个 agent 并存、issue 可指派给不同 agent；运行时可读 issue 上全部评论与产物；评论 @其他 agent 会为其入队 run（mention 触发），由对方的运行时领取执行。
- 服务端 worker 不领取 runtime=local 的 agent 的 run——它们只能被本机运行时领取。

## 测试

```bash
# 后端单元测试（无需数据库）
cd backend && go test ./...

# 后端集成测试（需要真实 PostgreSQL，如 compose 起的实例）
docker compose up -d
cd backend && SP_TEST_PG_DSN="postgres://specpowers:specpowers@localhost:5432/specpowers?sslmode=disable" go test ./...
```

集成测试（含 sp CLI 的集成测试：以 `server.Build` 起真实测试服务器，驱动 login → open → guard → verify → archive 全流程）由 `SP_TEST_PG_DSN` 守卫，未设置时自动跳过。**指向一个一次性数据库**（如 `specpowers_test`，用例会写入固定种子数据且不做完整清理），建议先 `dropdb --if-exists specpowers_test && createdb specpowers_test` 再跑：

```bash
SP_TEST_PG_DSN="postgres://specpowers:specpowers@localhost:5432/specpowers_test?sslmode=disable" go test ./...
```

生产部署（Docker Compose 一键全栈 / 主机二进制 / 前后端分离）见 `docs/deploy.md`。

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
| POST | `/changes` | 打开 change（body: `{issue_id, manual?}`；缺省跑 AI classic 拆分，`manual:true` 仅创建 proposal 阶段的空 change，由技能流程手动补产物） |
| GET | `/changes/{cid}` | change 详情（project_id / issue_id / phase / status） |
| GET | `/changes/{cid}/artifacts` | 产物列表（每类 kind 的最新版本，按 proposal→specs→design→tasks 排序） |
| GET | `/changes/{cid}/artifacts/{kind}` | 读取产物 markdown（kind: proposal/specs/design/tasks/verify；`?version=N` 指定版本，缺省最新） |
| POST | `/changes/{cid}/artifacts/{kind}` | 手动写入产物新版本（body: `{content}`；kind: proposal/specs/design/tasks；tasks 需含 ```json 围栏块并自动创建子 issue 与任务映射） |
| GET | `/changes/{cid}/tasks` | tasks 条目与子 issue 的映射（按 stage、position 排序） |
| GET | `/changes/{cid}/guard` | 门禁评估报告（phase 合法性 / handoff 新鲜度 / verify 通过 / 可推进 / 可归档，附未通过原因） |
| POST | `/changes/{cid}/guard` | 门禁推进：校验通过后进入下一 phase 并写入 handoff 记录 |
| POST | `/changes/{cid}/verify` | 提交 verify 报告（body: `{content}`，YAML；`result` 必须为 pass/fail，最新报告为 pass 才放行归档） |
| POST | `/changes/{cid}/archive` | 归档 change（要求 active、tasks 阶段、产物链自洽、handoff 新鲜、verify pass；归档后按父子对唤醒父 issue 负责人验收） |
| GET | `/skills` | 技能包列表（内嵌 superpowers 流程技能，按 flow 顺序） |
| GET | `/skills/{key}` | 读取单个技能的完整指令（brainstorm / write-plan / subagent-driven-development） |
| GET | `/changes/{cid}/skills/next` | 按 change 的 phase/status 解析下一个应加载的技能 |
| POST | `/agents/register` | 注册本机 agent（runtime=local），返回运行凭证（长时效 token，存 `~/.sp/agents/`） |
| POST | `/runtime/claim` | 本机运行时原子领取本 agent 的最旧 queued run（`FOR UPDATE SKIP LOCKED`，空队列返回 `{"run": null}`） |
| GET | `/runtime/issues/{iid}` | 运行时读取 issue 上下文（issue + 全部评论 + 元数据 + 项目资源；要求本 agent 在该 issue 上持有 run） |
| POST | `/runtime/issues/{iid}/comments` | 运行时以 agent 身份发评论（触发 mention 联动） |
| POST | `/runtime/issues/{iid}/status` | 运行时更新 issue 状态（校验状态机） |
| POST | `/runtime/runs/{rid}/log` | 追加 run 日志（llm_request / llm_response / tool_call / tool_result / error） |
| POST | `/runtime/runs/{rid}/finish` | 上报 run 终态（`{status: done\|failed, error?}`） |

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

### Agent 技能包（superpowers 流程）

技能 = 可被 agent 运行时加载的指令包，内嵌于服务端二进制（`internal/skill/skills/*.md`，frontmatter + markdown 正文），通过 `/api/v1/skills` 读取，不引入外部 superpowers 代码。核心流程三个技能：

1. **brainstorm（需求探索）**：澄清目标、盘点约束、比较方案，产出 proposal 产物并 handoff；
2. **write-plan（写实施计划）**：依次产出 specs / design / tasks 三份产物（tasks 自动拆出带 stage 的子 issue）；
3. **subagent-driven-development（子代理驱动开发）**：按 stage 顺序派发子任务给子代理执行、逐项验收，最后 verify + archive。

`GET /changes/{cid}/skills/next` 按 change 状态推导下一个技能（proposal→brainstorm；specs/design→write-plan；tasks→subagent-driven-development；非 active 无下一步）。不配置 LLM 时可用 `sp open --manual` + `sp artifact write` 驱动完整流程。

错误统一信封：`{"error":{"code":"...","message":"..."}}`。

## 数据库迁移

迁移脚本内嵌于 `backend/internal/store/postgres/migrations/`，服务启动时自动执行；通过 `schema_migrations` 表记账，重复执行自动跳过已应用版本。
