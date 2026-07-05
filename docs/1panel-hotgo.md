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

## GitHub Actions Secrets

在 GitHub 私有仓库的 Settings -> Secrets and variables -> Actions 里配置：

- `ONEPANEL_BASE_URL`：1Panel 面板地址，例如 `https://panel.example.com`
- `ONEPANEL_API_KEY`：1Panel API 密钥
- `ONEPANEL_CONTAINER_NAME`：需要升级的容器名，例如 `youban-server`
- `YOUBAN_TELEGRAM_BOT_TOKEN`：可选，构建通知用
- `YOUBAN_TELEGRAM_CHAT_ID`：可选，构建通知用

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
