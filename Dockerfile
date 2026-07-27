# syntax=docker/dockerfile:1.7

FROM node:20-bookworm-slim AS web-builder

WORKDIR /src/web

RUN corepack enable && corepack prepare pnpm@10 --activate

COPY web/package.json web/pnpm-lock.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile

COPY web ./

ARG VITE_PUBLIC_PATH=/admin
ARG VITE_BASE_URL=/
ARG VITE_DROP_CONSOLE=true
ARG VITE_GLOB_API_URL=
ARG VITE_GLOB_UPLOAD_URL=
ARG VITE_GLOB_IMG_URL=
ARG VITE_GLOB_API_URL_PREFIX=/admin
ARG VITE_BUILD_COMPRESS=none
ARG VITE_BUILD_COMPRESS_DELETE_ORIGIN_FILE=false
ARG VITE_GLOB_APP_TITLE=Youban
ARG VITE_GLOB_APP_SHORT_NAME=HG
ARG VITE_APP_DEMO_ACCOUNT=

ENV VITE_PUBLIC_PATH=${VITE_PUBLIC_PATH} \
    VITE_BASE_URL=${VITE_BASE_URL} \
    VITE_DROP_CONSOLE=${VITE_DROP_CONSOLE} \
    VITE_GLOB_API_URL=${VITE_GLOB_API_URL} \
    VITE_GLOB_UPLOAD_URL=${VITE_GLOB_UPLOAD_URL} \
    VITE_GLOB_IMG_URL=${VITE_GLOB_IMG_URL} \
    VITE_GLOB_API_URL_PREFIX=${VITE_GLOB_API_URL_PREFIX} \
    VITE_BUILD_COMPRESS=${VITE_BUILD_COMPRESS} \
    VITE_BUILD_COMPRESS_DELETE_ORIGIN_FILE=${VITE_BUILD_COMPRESS_DELETE_ORIGIN_FILE} \
    VITE_GLOB_APP_TITLE=${VITE_GLOB_APP_TITLE} \
    VITE_GLOB_APP_SHORT_NAME=${VITE_GLOB_APP_SHORT_NAME} \
    VITE_APP_DEMO_ACCOUNT=${VITE_APP_DEMO_ACCOUNT}

RUN pnpm run build


FROM golang:1.25.0-bookworm AS server-builder

WORKDIR /src/server

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/gogf/gf/cmd/gf/v2@v2.10.0

COPY server/go.mod server/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY server ./
RUN cp manifest/config/config.example.yaml manifest/config/config.yaml
RUN rm -rf resource/public/admin && mkdir -p resource/public/admin
COPY --from=web-builder /src/web/dist/. ./resource/public/admin/

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    rm -f internal/packed/packed.go \
    && gf pack resource,storage,manifest/config internal/packed/packed.go --keepPath=true \
    && CGO_ENABLED=1 go build -trimpath -o /tmp/hotgo ./main.go \
    && chmod +x /tmp/hotgo


FROM debian:trixie-slim AS runtime

ENV WORKDIR=/app \
    TZ=Asia/Shanghai

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates ffmpeg tzdata \
    && rm -rf /var/lib/apt/lists/*

WORKDIR ${WORKDIR}

COPY --from=server-builder /src/server/hack ./hack
COPY --from=server-builder /src/server/manifest/config ./manifest/config
COPY --from=server-builder /src/server/resource ./resource
COPY --from=server-builder /src/server/storage ./storage
COPY --from=server-builder /src/server/addons ./addons
COPY --from=server-builder --chmod=755 /tmp/hotgo ./hotgo
COPY --from=server-builder --chmod=755 /src/server/manifest/docker/entrypoint.sh ./entrypoint.sh

EXPOSE 8000

ENTRYPOINT ["./entrypoint.sh"]
