# 1Panel + HotGo 原生部署

当前部署方式固定为：GitHub Actions 构建并推送 GHCR 镜像，1Panel 只负责运行容器和升级镜像。HotGo 后端配置保持原生方式，运行时读取容器内 `/app/manifest/config/config.yaml`。

线上继续维护 `.env`。新镜像启动时会读取 `YOUBAN_*` / `GF_*` 环境变量，并覆盖镜像内置的 HotGo 配置；不再要求每次执行 `render-config.sh`。

HotGo 的部署规范不是每台服务器手动改 `config.yaml`。运行配置走 `.env`，数据库初始化走一次性 SQL/插件安装流程。当前镜像支持 `YOUBAN_AUTO_INIT_DATABASE=true`：启动时会在 `global.Init` 之前检查默认数据库，如果库里没有任何表，就自动导入 HotGo 基础 SQL；已有表时直接跳过。

## 首次在 1Panel 创建容器

需要先在 1Panel 手动创建一次容器，后续 GitHub Actions 才能通过 1Panel API 升级这个容器的镜像。

推荐直接使用 [deploy/1panel](../deploy/1panel/) 目录：

```bash
cp deploy/1panel/.env.example /opt/youban/.env
cp deploy/1panel/docker-compose.yml /opt/youban/docker-compose.yml
cp deploy/1panel/render-config.sh /opt/youban/render-config.sh
cd /opt/youban
chmod +x render-config.sh
vim .env
docker compose up -d
```

首次使用空 PostgreSQL 库时，`.env` 里保留：

```env
YOUBAN_AUTO_INIT_DATABASE=true
```

这个变量只负责初始化 HotGo 基础表，例如 `hg_sys_config`、`hg_sys_menu`、`hg_sys_addons_install`。它是空库保护逻辑，不会在已有库上重复导入。

悦伴业务表和插件表仍按 HotGo 插件规范安装/升级：基础表初始化成功后，在后台插件管理安装对应模块，或通过后续自动化脚本显式执行插件 install/upgrade。日常镜像升级只需要 GitHub Actions 推新镜像并让 1Panel 重建容器，不需要重新初始化数据库。

对应容器配置：

- 镜像：`ghcr.io/alcaideaubrianna-pixel/youban-server:latest`
- 容器名：`youban-server`
- 端口：宿主机 `8000` 映射到容器 `8000`
- 网络：加入 PostgreSQL、Redis 所在的 1Panel Docker 网络
- 重启策略：`unless-stopped`
- 时区环境变量：`TZ=Asia/Shanghai`

如果 GHCR 镜像不是公开包，需要先在服务器上执行 `docker login ghcr.io`，或者在 1Panel 镜像仓库配置 GHCR 凭据。

## 生产配置

日常修改 `.env`，再执行：

```bash
docker compose up -d --force-recreate
```

容器启动后会把 `.env` 中的变量覆盖到这些 HotGo 节点：

```yaml
system:
  appName: "youban"
  debug: false
  mode: "product"

server:
  address: ":8000"

cache:
  adapter: "redis"

token:
  secretKey: "change-me"

queue:
  driver: "redis"

redis:
  default:
    address: "1Panel-redis-Tn6k:6379"
    db: "3"
    pass: "change-me"
    idleTimeout: "20"

database:
  default:
    link: "pgsql:shangjia:change-me@tcp(1Panel-postgresql-ryg1:5432)/shangjia"
    debug: false
    Prefix: "hg_"
```

启动时报 `configuration missing for database node "database"`，通常是容器没有拿到数据库环境变量，或者镜像不是包含 env 覆盖逻辑的新版本。

启动时报 `pq: relation "hg_sys_config" does not exist`，说明数据库连接已经成功，但目标库还没有 HotGo 基础表。确认 `.env` 启用了 `YOUBAN_AUTO_INIT_DATABASE=true`，并且连接的是一个空库；如果库里已有其它业务表但没有 HotGo 表，需要先清理为真正空库，或手动导入 `storage/data/hotgo-pg.sql`。

如果日志里持续出现 `connect to 127.0.0.1:8099 error`，这是 HotGo 的 TCP 客户端在尝试连接主服务 TCP server。单容器只跑 HTTP 时可以忽略，或把启动命令固定为 `./main http`；后续拆出独立 cron 容器时，把 cron 容器的 `YOUBAN_TCP_CRON_ADDRESS` 设置为主服务容器地址，例如 `youban-server:8099`。

## 采集媒体队列配置

采集媒体使用实时优先队列、历史账号分片和账号级并发限制。Redis 只保存待执行任务，采集事件和媒体状态仍以 PostgreSQL 为准；容器重启后会根据数据库自动恢复未完成任务。

4 核 8G 单实例建议先使用：

```env
YOUBAN_PUBLISH_MEDIA_WORKER_CONCURRENCY=6
YOUBAN_PUBLISH_GLOBAL_MEDIA_CONCURRENCY=8
YOUBAN_PUBLISH_ACCOUNT_MEDIA_CONCURRENCY=2
YOUBAN_PUBLISH_MEDIA_FILE_CONCURRENCY=2
YOUBAN_PUBLISH_MEDIA_DOWNLOAD_THREADS=1
YOUBAN_PUBLISH_MEDIA_BULK_SHARDS=16
YOUBAN_PUBLISH_MEDIA_REALTIME_WEIGHT=32
YOUBAN_PUBLISH_MEDIA_BULK_WEIGHT=1
YOUBAN_PUBLISH_MEDIA_RECOVERY_BATCH_SIZE=300
YOUBAN_PUBLISH_MATERIAL_WINDOW_BATCH_SIZE=100
YOUBAN_PUBLISH_MATERIAL_VERIFY_WINDOW_SECONDS=300
```

参数说明：

- `YOUBAN_PUBLISH_MEDIA_WORKER_CONCURRENCY`：每个 HotGo 实例的媒体任务 Worker 数量。4 核机器建议先设为 `6`，观察稳定后可调整到 `8`。
- `YOUBAN_PUBLISH_GLOBAL_MEDIA_CONCURRENCY`：单实例同时执行的媒体下载上限。
- `YOUBAN_PUBLISH_ACCOUNT_MEDIA_CONCURRENCY`：同一 TG 账号在整个 Redis 集群中的媒体下载并发上限，使用分布式租约控制。
- `YOUBAN_PUBLISH_MEDIA_FILE_CONCURRENCY`：单个账号采集运行时允许并行处理的媒体文件数量。
- `YOUBAN_PUBLISH_MEDIA_DOWNLOAD_THREADS`：单个 TG 文件下载使用的分片线程数。优先增加 Worker，不建议盲目增加单文件线程。
- `YOUBAN_PUBLISH_MEDIA_BULK_SHARDS`：历史媒体队列分片数，最大 `16`，按 TG 账号稳定分片。
- `YOUBAN_PUBLISH_MEDIA_REALTIME_WEIGHT`：实时媒体队列调度权重。
- `YOUBAN_PUBLISH_MEDIA_BULK_WEIGHT`：每个历史媒体分片的调度权重。
- `YOUBAN_PUBLISH_MEDIA_RECOVERY_BATCH_SIZE`：每轮从数据库恢复的媒体任务数量。
- `YOUBAN_PUBLISH_MATERIAL_WINDOW_BATCH_SIZE`：单次采集资料分组窗口处理数量。
- `YOUBAN_PUBLISH_MATERIAL_VERIFY_WINDOW_SECONDS`：展示资料等待验证视频的最长时间，范围 `180～300` 秒，默认 `300` 秒。

SAE 修改环境变量后会通过新 Revision 重启实例并生效。扩容多个 HotGo 实例时，媒体 Worker 数量是每实例配置，例如两个实例均配置为 `6` 时，媒体 Worker 总数为 `12`；同一 TG 账号仍受集群级 `ACCOUNT_MEDIA_CONCURRENCY` 限制。

新队列版本首次部署后，可以执行一次队列重排：

```bash
/app/hotgo up -m=fix -a1=collectMediaQueueRebalance
```

该命令会清理媒体队列归档任务，将待执行任务按“实时优先、TG 账号轮询”重新投递，并跳过正在执行的任务。数据库中的采集业务状态不会被删除。

## GitHub Actions Secrets

在 GitHub 私有仓库的 Settings -> Secrets and variables -> Actions 里配置：

- `ONEPANEL_BASE_URL`：1Panel 面板地址，例如 `https://panel.example.com`
- `ONEPANEL_API_KEY`：1Panel API 密钥
- `ONEPANEL_CONTAINER_NAME`：需要升级的容器名，例如 `youban-server`
- `YOUBAN_TELEGRAM_BOT_TOKEN`：可选，构建通知用
- `YOUBAN_TELEGRAM_CHAT_ID`：可选，构建通知用
- `YOUBAN_SERVER_HOST`：可选，部署成功通知里的服务 IP 或域名，例如 `1.2.3.4`
- `YOUBAN_HTTP_PORT`：可选，部署成功通知里的服务端口，例如 `8000`

`sj/develop` 分支推送后，workflow 会构建：

```text
ghcr.io/alcaideaubrianna-pixel/youban-server:sha-xxxxxxx
```

然后调用 1Panel：

```http
POST /api/v2/containers/upgrade
```

请求体：

```json
{
  "names": ["youban-server"],
  "image": "ghcr.io/alcaideaubrianna-pixel/youban-server:sha-xxxxxxx",
  "forcePull": true
}
```

## 本地构建验证

根目录 Dockerfile 是唯一镜像构建入口：

```bash
docker build -t youban-server:local .
```

这个镜像会先构建 web，再把后台静态资源打入 server，最后使用 HotGo 原生 `server/manifest/docker/entrypoint.sh` 启动。
