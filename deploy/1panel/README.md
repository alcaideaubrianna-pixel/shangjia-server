# 1Panel 部署模板

这个目录用于线上 `/opt/youban`。日常只维护 `.env`，应用启动时会用 `YOUBAN_*` 环境变量覆盖镜像内置的 HotGo 配置。

## 首次部署

```bash
cp .env.example .env
vim .env
docker compose up -d
```

不需要挂载 `server.config.yaml`，也不需要执行 `render-config.sh`。

## 修改配置

修改 `.env` 后执行：

```bash
docker compose up -d --force-recreate
```

## 更新镜像

只更新镜像时，修改 `.env` 里的 `YOUBAN_SERVER_IMAGE` 后执行：

```bash
docker compose pull
docker compose up -d --force-recreate
```

GitHub Actions 调 1Panel 容器升级接口时，只会替换容器镜像，容器环境变量继续来自 1Panel/Compose 当前配置。

## 备用渲染

`render-config.sh` 只作为兼容旧部署的备用工具保留。新部署默认不需要使用它。
