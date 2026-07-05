# 1Panel + HotGo 原生部署

当前部署方式固定为：GitHub Actions 构建并推送 GHCR 镜像，1Panel 只负责运行容器和升级镜像。HotGo 后端配置保持原生方式，运行时读取容器内 `/app/manifest/config/config.yaml`。

线上可以继续维护 `.env`，但 `.env` 不是 HotGo 直接读取的配置。部署目录里的 `render-config.sh` 会把 `YOUBAN_*` 变量渲染成 `server.config.yaml`，再挂载到容器内的 `/app/manifest/config/config.yaml`。

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
./render-config.sh
docker compose up -d
```

对应容器配置：

- 镜像：`ghcr.io/alcaideaubrianna-pixel/youban-server:latest`
- 容器名：`youban-server`
- 端口：宿主机 `8000` 映射到容器 `8000`
- 网络：加入 PostgreSQL、Redis 所在的 1Panel Docker 网络
- 重启策略：`unless-stopped`
- 时区环境变量：`TZ=Asia/Shanghai`
- 配置挂载：`/opt/youban/server.config.yaml` 挂载到 `/app/manifest/config/config.yaml`

如果 GHCR 镜像不是公开包，需要先在服务器上执行 `docker login ghcr.io`，或者在 1Panel 镜像仓库配置 GHCR 凭据。

## 生产配置

日常修改 `.env`，再执行：

```bash
./render-config.sh
docker compose up -d --force-recreate
```

最终生成的 `server.config.yaml` 至少会包含这些 HotGo 节点：

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

启动时报 `configuration missing for database node "database"`，通常不是数据库连不上，而是容器没有加载到 `/app/manifest/config/config.yaml`，或者 `server.config.yaml` 没生成/挂载路径写错。

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
