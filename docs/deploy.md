# 部署指南

三种部署方式，按环境选择。所有方式共用同一套环境变量（见 README「环境变量」）。

## 方式 A：Docker Compose（推荐）

单命令拉起 全栈（PostgreSQL + spd + 前端静态资源）：

```bash
# 生产：先准备密钥（.env 或 shell 环境变量）
export SP_JWT_SECRET="$(openssl rand -hex 32)"
export SP_LLM_API_KEY=sk-...      # 可选：启用 AI classic 拆分与 agent LLM 执行
export SP_LLM_MODEL=gpt-4o        # 任意 OpenAI 兼容模型

docker compose up -d --build
```

浏览器访问 http://localhost:8080 —— 注册账号登录即用（spd 同端口服务 API 与前端）。`sp` CLI 已内置在镜像中，可 `docker compose exec spd sp --help`。

## 方式 B：主机二进制部署

```bash
# 1. 构建二进制与前端
cd frontend && npm ci && npm run build && cd ..
cd backend && go build -o /usr/local/bin/spd ./cmd/spd && go build -o /usr/local/bin/sp ./cmd/sp && cd ..

# 2. 准备 PostgreSQL（12+ 均可；迁移启动时自动执行）
createdb specpowers

# 3. 启动（systemd / NSSM / 直接运行均可）
export SP_DATABASE_URL="postgres://specpowers:***@localhost:5432/specpowers?sslmode=disable"
export SP_JWT_SECRET="$(openssl rand -hex 32)"   # 生产必填
export SP_STATIC_DIR="/opt/spec-powers/frontend/dist"   # 前端静态目录；不设则仅 API
export SP_ADDR=":8080"
spd
```

前端无需单独的 web 服务器：`SP_STATIC_DIR` 指向 `npm run build` 产物即可由 spd 直接托管（SPA 路由回退 index.html，`/api/*` 始终优先）。

Windows 本机验证示例（本仓库实际联调所用）：

```powershell
$env:SP_ADDR=":8090"; $env:SP_DATABASE_URL="postgres://user:pwd@localhost:5433/specpowers?sslmode=disable"
$env:SP_STATIC_DIR="D:\workplace\project\spec-powers\frontend\dist"
backend\spd.exe
```

## 方式 C：分离部署（前端独立托管）

不设 `SP_STATIC_DIR` 时 spd 仅提供 API；前端 `dist/` 可交由 nginx/CDN 托管，`vite.config.ts` 中将 `/api` 代理指向 spd 地址（开发模式 `npm run dev` 已内置代理到 :8080）。

## 部署验收清单

1. `GET /api/v1/health` 返回 `{"status":"ok"}`。
2. 浏览器打开根路径出现登录页；注册后进入项目列表。
3. 创建项目与 issue，`sp open` 触发 classic 拆分（或 `--manual` 手动流程）。
4. 四类产物入库、guard 推进、verify pass 后 archive 成功。
5. 归档后父 issue 负责人收到 wakeup 通知（通知中心可见）。

## 升级与回滚

- 数据库迁移内嵌于二进制，启动时自动增量执行、可重复运行（并发安全：advisory lock 串行化），回滚旧版本二进制无需回滚 schema（迁移向前兼容）。
- 附件目录（`SP_ATTACHMENT_DIR`）与 agent 工作目录需随版本持久化。

## 本地开发快速启动

见 README「本地启动」。
