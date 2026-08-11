# Railway 部署说明

项目使用同一个镜像创建 API、兼容 Worker、Collector Worker、Media Worker、Publish Worker、Account 和 Scheduler 服务。所有服务共享数据库、Redis 和业务环境变量，但启动命令与运行角色不同。

项目级 Railway Infrastructure as Code 位于 `.railway/railway.ts`，负责声明 PostgreSQL、Redis、四个运行服务、Singapore 区域和基础环境变量。它不是 Template，不会替代 Railway 项目本身；首次使用前先确认镜像地址和 Railway 项目环境。

三个服务必须使用同一个不可变镜像标签。推荐使用发布标签或 CI 生成的 SHA 标签，例如：

```text
ghcr.io/<owner>/youban-server:v20260811-01
ghcr.io/<owner>/youban-server:sha-xxxxxxx
```

禁止使用 `main`、`latest` 等可变标签作为生产部署版本，避免多分支构建互相覆盖。

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
- 迁移期间保留为兼容 Worker；确认三个独立 Worker 稳定后再逐步缩容或移除

## Collector Worker Service

- Start Command：`/app/hotgo collector-worker`
- `YOUBAN_RUNTIME_ROLES=collector-worker`
- Healthcheck Path：`/readyz`
- 负责实时/历史采集事件、后台采集任务和标准化交付
- 可以横向扩容，但历史展开必须继续遵守来源级背压和幂等

## Media Worker Service

- Start Command：`/app/hotgo media-worker`
- `YOUBAN_RUNTIME_ROLES=media-worker`
- Healthcheck Path：`/readyz`
- 负责采集媒体下载、媒体处理、对象存储和 PHash
- 可以根据媒体积压、CPU 和网络耗时独立扩容

## Publish Worker Service

- Start Command：`/app/hotgo publish-worker`
- `YOUBAN_RUNTIME_ROLES=push-worker`
- Healthcheck Path：`/readyz`
- 负责 Telegram 发布、单频道顺序、限流和失败重试
- 可以横向扩容，一个频道异常不得影响其他频道

## Account Service

- Start Command：`/app/hotgo account`
- `YOUBAN_RUNTIME_ROLES=account`
- Healthcheck Path：`/readyz`
- 不绑定公网业务域名
- 运行 Telegram Bot、账号监听和账号相关常驻任务
- 初始固定 1 个副本；完成账号租约压测后，再根据账号量增加副本

## Scheduler Service

- Start Command：`/app/hotgo scheduler`
- `YOUBAN_RUNTIME_ROLES=scheduler`
- Healthcheck Path：`/readyz`
- 不绑定公网业务域名
- 运行 HotGo Cron、发布调度和恢复调度
- `YOUBAN_TCP_CRON_ADDRESS=xiaohuiji-api.railway.internal:8099`
- 固定 1 个副本，禁止横向扩容

## 公共要求

- 所有应用服务必须使用同一个镜像标签和同一套数据库、Redis 配置
- Railway 会覆盖镜像 `ENTRYPOINT`，Start Command 必须填写 `/app/hotgo <role>`，不能只填写角色名
- 不要设置 `YOUBAN_SERVER_PORT` 或 `GF_SERVER_PORT`，让程序直接读取 Railway 自动注入的 `PORT`
- `YOUBAN_CACHE_ADAPTER` 和 `YOUBAN_QUEUE_DRIVER` 必须使用 `redis`
- 不要在生产环境使用默认启动命令或 `all`
- `/healthz` 只检查进程存活，`/readyz` 检查数据库和 Redis
- 数据库迁移通过显式 Job 或手动命令执行，不要放入三个服务的启动命令
- Account 滚动发布可能出现新旧实例短暂重叠，Telegram 账号仍需依赖租约及分布式锁保护
- Scheduler 必须保持单实例，避免 Cron 重复执行

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

配置文件不会内置镜像标签，执行 IaC 时必须显式传入完整镜像引用：

```text
YOUBAN_RAILWAY_IMAGE=ghcr.io/<owner>/youban-server:sha-xxxxxxx
```

如果仓库迁移到其他 GitHub Owner，只需要修改命令中的镜像地址，不需要修改 IaC 文件。私有 GHCR 镜像还需要在 Railway 配置 Registry 凭证。

`railway config apply` 只负责项目资源和服务配置，不负责构建镜像。镜像仍由 GitHub Actions 构建并推送。

使用指定标签部署：

```bash
export YOUBAN_RAILWAY_IMAGE=ghcr.io/<owner>/youban-server:v20260811-01
npx -y @railway/cli@latest config plan
npx -y @railway/cli@latest config apply
```

切换到另一个版本时，只替换 `YOUBAN_RAILWAY_IMAGE`，重新执行 `config plan` 和 `config apply`。三个服务会统一切换到同一个镜像版本。

## GitHub Actions 与 Railway 自动部署

GitHub Actions 只负责构建并推送不可变镜像。Railway 服务通过原生 GitHub 集成监听连接分支，等待 GitHub Actions 成功后自动部署，不再由 workflow 调用 Railway CLI。

Railway 服务设置中需要：

- 连接正确的 GitHub 仓库和部署分支；
- 启用 **自动部署**；
- 启用 **Wait for CI**，等待 GitHub Actions 成功后再部署；
- 确认 Railway GitHub App 对仓库具有贡献者访问权限。

GitHub Actions 不再需要 `RAILWAY_TOKEN` 或 `RAILWAY_PROJECT_ID`。Railway 部署失败、等待 CI 或分支未触发时，直接查看 Railway 服务的部署历史和 GitHub Actions 运行状态。

Railway 项目本身也支持 Project Webhook，可在 Railway 项目设置中订阅部署失败、部署状态变化和资源告警。Webhook 更适合接入 OpenObserve、Slack 或 Discord；Telegram 通知继续由 GitHub Actions 发送，避免额外维护一个 Webhook 转发服务。

## 自动部署

当前 GitHub Actions 负责构建并推送分支、发布标签和 `sha-xxxxxxx` 镜像。生产部署只选择明确的发布标签或 SHA 标签，不直接部署分支标签：

1. Redeploy Collector Worker
2. Redeploy Media Worker
3. Redeploy Publish Worker
4. Redeploy compatibility Worker
5. Redeploy Scheduler
6. Redeploy Account
7. Redeploy API

自动发布时，GitHub Actions 将本次构建产生的镜像标签推送到腾讯云镜像仓库；Railway 原生 GitHub 集成负责使用连接分支的最新提交触发部署。不要再维护 GitHub Actions 到 Railway 的第二套 CLI 部署链路。

## 推荐上线顺序

1. 显式执行 `telegram_collector` 插件安装或升级 SQL
2. 部署 Collector、Media、Publish Worker，并保持 `YOUBAN_TELEGRAM_COLLECTOR_ENABLED=true`
3. 部署 Account，确认同一 TG 账号始终只有一个连接租约持有者
4. 部署 API，验证现有 Bot Gateway、采集和发布链路不受影响
5. 观察 Event、Delivery、媒体命中率和重试指标
6. 确认实时与历史采集的资料数量、媒体数量和目标频道一致
7. 保留兼容 Worker，观察独立 Worker 稳定后再缩容
8. 根据各自队列积压分别扩容 Collector、Media、Publish 和 Account
