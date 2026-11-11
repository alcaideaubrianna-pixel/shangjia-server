# syntax=docker/dockerfile:1
FROM golang:1.25 AS hook-build
WORKDIR /src
# Hook 单独构建，业务代码更新不会改变 Hook 镜像或中断正在发送的请求。
COPY go.mod go.sum ./
COPY tools/tg_hook_mock ./tools/tg_hook_mock
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -o /out/tg-hook ./tools/tg_hook_mock

FROM golang:1.25 AS hook
COPY --from=hook-build /out/tg-hook /usr/local/bin/tg-hook
ENTRYPOINT ["/usr/local/bin/tg-hook"]

FROM golang:1.25 AS build
WORKDIR /src
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -o /out/youban-server ./main.go

# 使用与 Go 构建阶段一致的发行版，避免 Go 1.25 构建出的二进制依赖
# 高于 bookworm 的 glibc/libstdc++，容器启动后反复重启。
FROM golang:1.25
WORKDIR /usr/local/bin
# Media-worker exercises the same video poster path as production. Keep the
# local full-flow image self-contained so the regression does not fail merely
# because ffmpeg is absent from the test container.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ffmpeg \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/youban-server /usr/local/bin/youban-server
COPY --from=build /src/manifest /usr/local/bin/manifest
ENTRYPOINT ["/usr/local/bin/youban-server"]
