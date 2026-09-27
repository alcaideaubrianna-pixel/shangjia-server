# Youban Agent 运维与交付手册

本文档是开发 Agent 完成需求后的统一交付入口。适用于 `youban-server` 及其 Dokploy 运行环境。

## 1. 环境与节点

生产环境采用腾讯云新加坡内网通信，Railway 仅作为迁移/回滚环境保留，未经确认不得同时运行会消费同一批生产队列的 Worker。

| 节点 | 地址 | 职责 |
| --- | --- | --- |
| 应用 1 | `10.3.0.17` | API 两副本、account、scheduler、publish-worker |
| 应用 2 | `10.3.0.3` | 保留基础设施与监控；业务 Worker 默认缩容为 0 |
| 应用 3 | `10.3.4.11` | worker、publish-worker、media-worker、collector-worker |
| 数据库 | 腾讯云 PostgreSQL 内网地址 | 主数据库；连接信息只放 Dokploy 环境变量 |
| Redis | 腾讯云 Redis 内网地址 | Asynq、缓存、去重和运行队列 |
| 监控 | `10.3.4.2` | Pigsty、VictoriaMetrics/Logs/Traces、VMAlert、Alertmanager、OTel Collector |

公网入口：

- 前端/API：`https://api-test.xiaohuiji.cc/`
- Telegram Bot webhook：`https://api.xiaohuiji.cc/`
- 监控面板：由监控节点提供，访问密码不得写入代码或文档

所有应用到 PostgreSQL、Redis、监控的连接优先使用内网地址；公网域名只用于浏览器、Telegram 和外部回调。

## 2. 开发交付流程

1. 从当前工作分支创建功能分支，先阅读仓库 `AGENTS.md` 和相关模块现有实现。
2. 按 GoFrame 分层修改：API → controller → service → logic → dao。不要手改生成的 `entity`、`do`、`dao/internal` 文件。
3. 前端使用 Vue 3、Pinia、Naive UI 和 `@/utils/http/axios`，不要引入其他 UI 库或直接调用 axios。
4. 完成后执行与改动相关的格式化、编译和测试。最低检查：

   ```bash
   gofmt -w <changed-go-files>
   go test ./...
   go build ./...
   ```

   前端改动另执行项目已有的 `pnpm lint`、`pnpm typecheck` 或等价脚本。

5. 检查工作区，只提交本次需求相关文件，不覆盖用户已有改动：

   ```bash
   git status --short
   git diff --check
   git diff --stat
   git add <files>
   git commit -m "<type>: <简短说明>"
   git push origin <branch>
   ```

提交信息建议使用 `feat`、`fix`、`refactor`、`chore`、`docs`。不要提交 `.env`、Token、密码、生产数据库 URL 或临时导出文件。

## 3. 自动构建与发布

GitHub Actions 是唯一构建入口。推送到约定分支后，Workflow 应执行：

1. 编译并运行测试；
2. 构建 Docker 镜像；
3. 推送到 GHCR 或配置的镜像仓库；
4. 使用不可变镜像标签，例如 `sha-<git-sha>`，禁止生产使用 `latest`；
5. 按顺序调用 Dokploy Webhook，让各服务拉取同一个镜像摘要并滚动部署。

Dokploy 部署 Webhook 统一保存在 `deploy/dokploy-targets.json`，方便集中维护；其他仓库 Token、API Key 和生产凭据仍放在 GitHub Actions Secrets 或 Dokploy Secret 中。不要在源码其他位置复制 Webhook，也不要在日志打印请求头。

发布前检查：

```bash
git rev-parse HEAD
docker build --pull -t <image>:sha-<git-sha> .
docker image inspect <image>:sha-<git-sha> --format '{{.Id}}'
```

Dokploy 中七个应用必须引用同一镜像标签/摘要。推荐顺序为：API → account/scheduler → 普通 Worker → media/collector Worker；每个服务健康后再通知下一个。发布失败时停止后续 Webhook，不要重复触发可能造成重复消费的 Worker。

## 4. 平滑发布与回滚

- API 使用滚动替换，必须先确认新副本 `/readyz` 返回 200，再下线旧副本。
- Worker 发布前确认队列消费者数量、同频道串行锁和 Redis 连接正常。
- 数据库迁移必须可重复执行、向后兼容；先备份/验证，再切换应用连接。
- 回滚只切换到上一个已验证的 `sha-<git-sha>` 镜像，不使用源码现场构建。
- 任何会改变队列消费组、Webhook 地址、数据库结构的发布，都要记录变更时间、镜像 SHA 和回滚 SHA。

## 5. 监控与告警

监控节点由 Pigsty 管理：

- VictoriaMetrics：指标，端口 `8428`
- VictoriaLogs：应用日志，端口 `9428`，保留 3 天
- VictoriaTraces：Trace，端口 `10428`，保留 3 天
- OTel Collector：接收 Go OTLP，`4317`/`4318`
- VMAlert：规则评估，端口 `8880`
- Alertmanager：告警路由、去重、恢复通知，端口 `9059`
- Blackbox Exporter：API HTTP 健康探测，端口 `9115`

应关注：API `/readyz` 失败/延迟、Node CPU/Load、Docker/cAdvisor、AgentDown、Worker 队列积压、OTel Collector 导出失败、Go 错误日志和 Trace 错误 span。告警统一通过 Alertmanager 发送 Telegram，凭据只存在监控节点 Secret/环境文件。

常用只读检查：

```bash
curl -fsS https://api-test.xiaohuiji.cc/readyz
curl -fsS https://api.xiaohuiji.cc/readyz
curl -fsS http://<monitor-private-ip>:8428/-/ready
ssh ubuntu@<monitor> 'sudo docker ps; sudo docker logs --since 10m otel-collector'
```

遇到告警先确认：是否真实业务错误、是否单节点资源异常、是否队列积压、是否数据库/Redis 连接异常。不要因为单个任务失败直接停止整个频道；按照现有 Job 重试、失败标记和频道级串行策略处理。

## 6. 日志与排障要求

业务关键节点必须带结构化日志和 `ctx`：请求 ID、TraceID、tenant、account、profile、job、channel、消息 ID、重试次数和最终状态。禁止打印密码、Token、完整媒体 URL 签名和用户隐私内容。

排查发布问题时按以下顺序关联：

`Telegram 原消息 → collect event/profile → tg job → dispatch/send 日志 → Telegram 消息 ID`。

排查媒体问题时记录媒体 ID、source key、文件类型、HTTP 状态、缓存状态和失败原因；不要只看前端返回。日志和 Trace 默认只保留 3 天，长期审计信息写入业务表而不是无限堆积日志。

## 7. 变更完成标准

需求只有在以下事项全部完成后才算交付：

- 代码、迁移和配置已提交并推送；
- CI 构建成功，记录镜像 SHA；
- Dokploy 服务按顺序部署并通过健康检查；
- API、Bot webhook、数据库、Redis、队列和监控均完成冒烟验证；
- 监控面板能看到新版本指标/日志/Trace；
- 变更摘要包含影响范围、验证结果、回滚方式和未解决风险。
