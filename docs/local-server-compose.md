# 本地 Server 依赖

这个 Compose 文件只启动 HotGo Server 本地开发依赖，不启动 Go 服务本身。

当前 `server/manifest/config/config.yaml` 默认连接：

- PostgreSQL: `127.0.0.1:5432`，数据库 `youban_hotgo`，账号 `postgres`，密码 `postgres`
- Redis: `127.0.0.1:6379`，默认使用 `db: 3`

## 启动依赖

在项目根目录执行：

```bash
docker compose -f docker-compose.server.yml up -d
```

查看状态：

```bash
docker compose -f docker-compose.server.yml ps
```

停止依赖：

```bash
docker compose -f docker-compose.server.yml down
```

PostgreSQL 和 Redis 都使用命名卷保存数据，普通停止或重启不会丢库。

## 启动 Server

进入 `server` 目录，按 HotGo 文档的热编译方式启动：

```bash
gf run main.go --args "all"
```

或使用 Makefile：

```bash
make all
```

## 端口冲突

如果本机已经有 Redis 或 PostgreSQL 占用默认端口，可以通过环境变量改 Compose 暴露端口。

例如只改 Redis 宿主机端口：

```bash
HOTGO_REDIS_PORT=6380 docker compose -f docker-compose.server.yml up -d
```

同时需要把 `server/manifest/config/config.yaml` 中的 Redis 地址改为对应端口。

## 数据初始化

该 Compose 文件不会自动导入 SQL，也不会初始化 demo 数据。需要初始化数据库时，请使用项目显式初始化流程或手动导入对应 SQL。
