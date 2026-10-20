# 构建版本号

`build-images` 使用 `<run_number>.<run_attempt>` 作为构建号，例如
`123.1`。重新运行同一次工作流会生成 `123.2`。

构建号通过 Docker `BUILD_VERSION` 参数写入 Go 二进制，首页显示该编号；
本地未指定时显示 `dev`。HotGo 框架版本独立保留，不随业务构建改变。

镜像成功构建并推送后，工作流向实际构建的 commit 推送同名轻量 Git tag。
构建失败不创建 tag；既有 tag 不会被覆盖。需要仓库允许工作流的
`GITHUB_TOKEN` 写入 tag；若 tag 被仓库规则保护，需要允许该工作流创建。
现有 SHA 镜像标签和部署方式不变。
