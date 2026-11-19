# 腾讯云镜像构建与加速

工作流：`.github/workflows/build-server-image.yml`。

## 发布边界

- `feat/railway-runtime-ci-split` 和 `ci/tencent-build-speed` 的镜像相关改动触发构建，也支持手动触发。
- 不再安装阿里云 CLI、修改 ACR 白名单、获取 ACR Token、上传 ACR 或部署 SAE。
- 一次 BuildKit 构建，通过两个独立 image exporter 并行上传 GHCR、腾讯云。不要添加全局 `tags` 输入，否则 Buildx 会覆盖两个 exporter 各自的 `name`。
- 两个 exporter 都成功之后才创建成功构建 Git tag。单仓上传成功不代表整次发布成功。
- GHCR 使用分支标签和 `sha-<短 SHA>`。腾讯云使用短 SHA 标签；正式分支同时更新 `latest`，优化分支仅更新自己的分支标签，避免测试镜像覆盖正式 `latest`。
- 部署使用不可变 SHA 标签；工作流不自动部署业务服务。
- 同分支新提交取消旧构建。取消或单仓失败可能留下部分已经上传的标签；不要用浮动标签判断本次构建是否完整成功。

## 缓存与构建阶段

| 阶段 | 缓存失效条件 |
| --- | --- |
| 前端依赖安装 | 前端 lockfile、package.json 或基础镜像变化 |
| 前端构建 | 前端源码或前端构建参数变化 |
| Go 依赖下载 | go.mod、go.sum 或构建基础镜像变化 |
| 资源打包 | resource、storage、manifest/config 或前端产物变化 |
| Go 增量编译与链接 | Go 源码、打包产物或构建版本变化 |
| ffmpeg 等运行依赖安装 | Debian 基础镜像或安装命令变化，提交 SHA 不再使其失效 |

`BUILD_VERSION` 只传入最后的 Go 构建步骤，不再导致资源重新打包。后端代码变更复用前端和资源打包结果，Go 编译使用 `/root/.cache/go-build` 挂载。

GHA layer cache 之外，使用 Actions cache 和固定提交版本的 buildkit-cache-dance 恢复、注入并提取 Go 编译缓存。仅添加 Docker cache mount 无法在每次新建的 GitHub Runner 之间保留编译缓存。

Go 编译缓存按工具链、平台和 go.sum 分组，每个提交保存更新后的内容，下一次构建优先恢复同依赖版本的最近缓存。BuildKit layer cache 使用独立 `server-tencent-linux-amd64` scope，并尝试读取原 `server` scope。

文档、运维工具、Go 测试文件不触发镜像构建；后端工具和测试文件也不进入 Docker 构建上下文。测试验证仍可通过独立流程运行，本工作流不新增 `go test`。

## 配置

沿用现有凭据：

- GHCR：`GITHUB_TOKEN`，`packages: write`。
- 腾讯云：`TENCENT_REGISTRY_USERNAME`、`TENCENT_REGISTRY_PASSWORD`。
- 原有 Telegram 构建结果通知配置保留。

可选 Repository Variable：`BUILD_RUNNER`。默认 `ubuntu-latest`，也可指定已配置的专用 Linux AMD64 Runner label；该 Runner 必须提供 Docker/Buildx，并能访问两个仓库和 Actions cache。

## 继续加速的评估顺序

1. 分别比较冷构建、缓存恢复构建、单文件 Go 修改构建的耗时。记录依赖下载、资源打包、Go 编译、缓存导入导出、两个仓库上传各阶段，避免只看总时间。
2. 若 Go 编译仍占主导，评估专用 CPU 更强的 Runner，并配置持久化 BuildKit builder。只切换 Runner label 不会自动获得持久编译缓存。
3. 若每次 GHA 缓存传输比编译更慢，比较持久 builder 与当前 cache-dance 方案，避免重复传输大缓存。
4. 若最终链接和镜像上传占主导，再评估将前端静态资源从 Go 内嵌资源解耦、缩减运行镜像。两者改变制品或部署结构，应单独验证，不混入本次 CI 调整。

本地语法检查和小镜像 exporter 验证不代替真实业务镜像的首轮 CI 验收；加速比例以实际构建数据为准。
