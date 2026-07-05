# 1Panel 部署模板

这个目录用于线上 `/opt/youban`。日常只维护 `.env`，不要手改容器内的 HotGo 配置。

## 首次部署

```bash
cp .env.example .env
vim .env
./render-config.sh
docker compose up -d
```

`render-config.sh` 会在第一次执行时从 `YOUBAN_SERVER_IMAGE` 镜像里读取原生配置：

```text
/app/manifest/config/config.yaml
```

然后生成宿主机文件：

```text
./server.config.yaml
```

Compose 会把它挂载回容器：

```text
./server.config.yaml -> /app/manifest/config/config.yaml
```

## 修改配置

修改 `.env` 后执行：

```bash
./render-config.sh
docker compose up -d --force-recreate
```

## 更新镜像

只更新镜像时，修改 `.env` 里的 `YOUBAN_SERVER_IMAGE` 后执行：

```bash
./render-config.sh
docker compose pull
docker compose up -d --force-recreate
```

GitHub Actions 调 1Panel 容器升级接口时，只会替换容器镜像，不会重新渲染 `server.config.yaml`。如果 `.env` 配置没有变化，这是正常的。
