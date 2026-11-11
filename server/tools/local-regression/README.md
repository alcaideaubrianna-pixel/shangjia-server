# 本地持续运行环境

这个目录提供与线上 `web`、`publish-worker`、`background-worker`、`media-worker` 相同启动入口的 Docker 运行环境。默认两个 publish-worker，使用 product 模式及本地 PostgreSQL/Redis，Telegram 请求全部指向 `EvilFreelancer/tgfake:1.5`。

tgfake 提供完整 Bot API、Webhook、图片/视频/媒体组、文件下载、按钮回调和故障注入能力，浏览器界面位于 `http://127.0.0.1:18790/`。业务角色通过 `http://tgfake:18790` 访问，测试机上的故障控制使用：

```bash
./tgfake-control.sh '{"default":{"action":"success","delayMs":2},"rules":[{"method":"sendMediaGroup","chatId":"-100999000012","action":"accepted_timeout"}]}'
```

旧的轻量 Hook 保留为 `legacy-hook` profile，仅用于对比旧回放：`docker compose --profile legacy-hook up tg-hook`。

重置 tgfake 后重新准备媒体：

```bash
YOUBAN_LOCAL_DB_NAME=youban_tgfake_test ./prepare-tgfake-media.sh
```

## 启动

```bash
cd server/tools/local-regression
./docker-up.sh
docker compose logs -f publish-worker
docker compose logs -f background-worker
```

环境默认连接宿主机上的 PostgreSQL `youban_hotgo`（5432）和 Redis（6379，DB 15），并强制设置 `YOUBAN_PUBLISH_PUSH_ENGINE=unified_stream`，用于验证全量 Stream 路径。HotGo 使用 `Asia/Shanghai` 时间，首次运行本地测试时请执行：

```sql
ALTER DATABASE youban_hotgo SET timezone TO 'Asia/Shanghai';
```

Hook 监听 `http://127.0.0.1:19100`，请求记录以 JSONL 逐条写入 Docker volume `regression_logs` 的 `hook.jsonl`，避免压测时重复重写全部历史记录。

构建代理与副本数可以显式设置；代理地址必须能从构建容器访问：

```bash
YOUBAN_LOCAL_BUILD_PROXY=http://host.docker.internal:6152 \
YOUBAN_LOCAL_PUBLISH_REPLICAS=2 ./docker-up.sh
```

`YOUBAN_LOCAL_REDIS_ADDRESS` 可指向专用测试 Redis，默认仍为宿主机 6379。改变地址时必须对所有角色使用同一设置。

本轮专用环境使用 `YOUBAN_LOCAL_REDIS_ADDRESS=host.docker.internal:16382`。Hook支持 `POST /__test/scenario` 动态注入故障；接口必须返回204，若返回通用Bot响应，应排查宿主机旧Hook进程占用了19100端口。

## 真实发送与回调场景

先准备本地完整表结构及1000份夹具；Gateway需要 `hg_sys_addons_install` 中的安装记录（见 `seed_publish_dependencies.sql`）。共享Gateway提供方需要的表使用仓库正式迁移补齐，例如：

```bash
docker exec -i youban-postgres psql -U postgres -d youban_hotgo -X -v ON_ERROR_STOP=1 \
  < ../../addons/youban_two_way_bot/resource/sql/install.pgsql.sql
YOUBAN_LOCAL_REDIS_ADDRESS=host.docker.internal:16382 ./docker-up.sh
bash run-runtime-scenarios.sh
```

脚本在实际角色上依次运行：1000条发送、延迟Webhook和未知结果的频道隔离、慢发送/慢删除、500历史积压中插入100实时采集+100循环+20手工任务。它逐条核对发送证据、同频道顺序、优先级及资料投影，报告保存至 `/tmp/youban-runtime-*`。场景结束或失败时自动恢复Hook默认行为。

范围从持久化job/outbox开始，采集入口、循环/批次定时器、协议号兜底和防扫图的完整角色测试仍需单独验证。Webhook由web角色接收，Gateway更新由已有background-worker消费；测试不启动真实Telegram账号长连接。

单轮只读复核：

```bash
bash verify-runtime-round.sh <run_id> <资料数量> <频道数量> any
```

`any`接受HTTP或Webhook的完整发送证据；`response`仅检查HTTP响应，用于定位确认路径差异。

## 真实运行采样

```bash
./measure-performance.sh 120 15
```

脚本只读取本地账户 999 的夹具任务，输出本次窗口的成功吞吐、频道间隔分位数和 Docker 资源采样到 `/tmp`。这不是代码回归测试；必须先运行真实 Worker 并准备资料。结果限制与已知问题见 [PERFORMANCE.md](PERFORMANCE.md)。

## 媒体故障恢复

`scenarios/media_recovery.sql` 为一份本地夹具资料注入处理进程中断。先观察其推送等待而其他频道继续，再启动项目已有的媒体角色：

```bash
docker compose --profile media up -d --no-build media-worker
```

该场景需逐项核验媒体状态、job 状态、成功消息数量和自动唤醒；执行 SQL 或启动角色本身不代表验证通过。

## 停止

```bash
./docker-down.sh
```

## 检查 Stream 消费组

```bash
docker exec r1rpc-redis redis-cli -n 15 XINFO GROUPS youban_publish:tg:ready:realtime
docker exec r1rpc-redis redis-cli -n 15 XINFO GROUPS youban_publish:tg:ready:bulk
```

Docker 服务使用 `restart: unless-stopped`，终端关闭后仍会持续运行；需要停止时执行 `docker-down.sh`。

### 一键干净回归

先停止业务角色并重建独立测试库，再启动两个 publish-worker 和两个 background-worker：

```bash
YOUBAN_LOCAL_DB_NAME=youban_tgfake_test \
YOUBAN_LOCAL_POLL_INTERVAL=0.2 \
./run-clean-tgfake-regression.sh
```

脚本会重置 PostgreSQL、Redis、tgfake 状态并重新上传媒体夹具，避免上一轮 pending/retry 任务污染结果。
