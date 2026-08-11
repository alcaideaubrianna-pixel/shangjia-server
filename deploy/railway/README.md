# Railway 部署说明

项目使用同一个镜像创建三个 Railway Service。三个服务共享数据库、Redis 和业务环境变量，但启动命令与运行角色不同。

项目级 Railway Infrastructure as Code 位于 `.railway/railway.ts`，负责声明 PostgreSQL、Redis、三个运行服务、Singapore 区域和基础环境变量。它不是 Template，不会替代 Railway 项目本身；首次使用前先确认镜像地址和 Railway 项目环境。

推荐三个服务都使用 CI 已发布的 GHCR `main` 标签，例如 `ghcr.io/<owner>/youban-server:main`。每次发布仍同时保留 `sha-xxxxxxx` 不可变标签用于审计和回滚。

## API Service

- Start Command：`/app/hotgo web`
- `YOUBAN_RUNTIME_ROLES=web`
- Healthcheck Path：`/readyz`
- 对外绑定业务域名
- 初次部署使用 1 个副本，验证完成后再增加 API 副本

## Worker Service

- Start Command：`/app/hotgo worker`
- `YOUBAN_RUNTIME_ROLES=worker`
- Healthcheck Path：`/readyz`
- 不绑定公网业务域名
- 消费 HotGo Queue 和上架系统 Asynq Worker，可根据队列积压增加副本

## Runtime Service

- Start Command：`/app/hotgo runtime`
- `YOUBAN_RUNTIME_ROLES=runtime`
- Healthcheck Path：`/readyz`
- 不绑定公网业务域名
- 运行 Cron、Telegram Bot、账号监听、Scheduler 和后台恢复任务
- `YOUBAN_TCP_CRON_ADDRESS` 配置为 API Service 的 Railway 私网地址和 `8099` 端口，用于后台动态刷新 HotGo Cron
- 固定 1 个副本；账号监听完成租约压测前不要横向扩容

## 公共要求

- 三个服务必须使用同一个镜像标签和同一套数据库、Redis 配置
- Railway 会覆盖镜像 `ENTRYPOINT`，Start Command 必须填写 `/app/hotgo <role>`，不能只填写角色名
- 不要设置 `YOUBAN_SERVER_PORT` 或 `GF_SERVER_PORT`，让程序直接读取 Railway 自动注入的 `PORT`
- `YOUBAN_CACHE_ADAPTER` 和 `YOUBAN_QUEUE_DRIVER` 必须使用 `redis`
- 不要在生产环境使用默认启动命令或 `all`
- `/healthz` 只检查进程存活，`/readyz` 检查数据库和 Redis
- 数据库迁移通过显式 Job 或手动命令执行，不要放入三个服务的启动命令
- Runtime 滚动发布可能出现新旧实例短暂重叠，Telegram 账号和调度任务仍需依赖现有租约及分布式锁保护

## Infrastructure as Code

在仓库根目录执行 Railway CLI：

```bash
npm install
railway link
railway config plan
railway config apply
```

Railway IaC 使用根目录 `package.json` 中的 `railway` TypeScript SDK，要求 Node.js 22 或更高版本。第一次拉取仓库或 SDK 版本更新后需要先执行 `npm install`。

第一次执行 `config plan` 必须先确认资源名称和变更内容。若 Railway 项目中已经手动创建了同名服务或数据库，先核对 CLI 的资源匹配结果，不要直接创建重复的 PostgreSQL 或 Redis。

配置文件中的 GHCR 镜像目前是：

```text
ghcr.io/mjiadfwaff-bot/youban-server:main
```

如果仓库迁移到其他 GitHub Owner，需要同步修改 `.railway/railway.ts` 的 `imageRef`。私有 GHCR 镜像还需要在 Railway 配置 Registry 凭证。

`railway config apply` 只负责项目资源和服务配置，不负责构建镜像。镜像仍由 GitHub Actions 构建并推送。

## 自动部署

当前 GitHub Actions 已负责构建并推送 `main`、`latest` 和 `sha-xxxxxxx` 镜像。首次部署建议先手动执行 `railway config apply` 并验证三种角色稳定，再在镜像构建成功的 Job 后追加 Railway CLI 自动发布：

1. Redeploy Worker
2. Redeploy Runtime
3. Redeploy API

生产环境只允许 `main` 分支触发自动发布，建议使用 GitHub `production` Environment 保存 `RAILWAY_TOKEN` 并按需开启人工审批。不要让三个 Railway Service 分别从 GitHub 重复构建同一个 Dockerfile。

## 推荐上线顺序

1. 部署 Worker，确认队列可以正常消费
2. 部署 Runtime，保持 1 个副本并确认 `/readyz` 正常
3. 部署 API，绑定域名并验证登录、上传、支付回调和 Telegram Webhook
4. 观察任务积压和错误日志后，仅扩容 API 与 Worker
