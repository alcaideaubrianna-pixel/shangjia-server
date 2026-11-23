# 腾讯云生产排障手册

生产环境为腾讯云新加坡轻量云 Docker Swarm 集群。发布、切流及回滚见 [Swarm 生产发布手册](../deploy/SWARM.md)。

| 入口 | 用途 |
|---|---|
| `https://api.xiaohuiji.cc/readyz` | 线上入口及应用依赖探测 |
| `pgsql.tke.xiaohuiji.cc`、`redis.tke.xiaohuiji.cc` | 腾讯云私有 DNS 的生产依赖记录；端口、TLS、逻辑库从已授权的在用角色配置核对 |
| `ubuntu@43.134.114.149` | PostgreSQL / Pigsty 主机，业务库 `railway` |
| `ubuntu@43.160.217.230` | Redis 主机、指标采集及转发 |
| `43.134.27.229` | 监控主机，Grafana、VictoriaMetrics、VictoriaTraces、Grafana Explore |
| `43.159.50.113` | 日志主机，VictoriaLogs，HTTPS 日志入口 `https://43.159.50.113.sslip.io` |
| [应用性能监控](https://43.134.27.229.sslip.io/d/xiaohuiji-sg-http-apm) · [PostgreSQL 慢 SQL](https://43.134.27.229.sslip.io/d/xiaohuiji-pgsql-query) · [Grafana Explore](https://43.134.27.229.sslip.io/explore) | HTTP 耗时和状态码、慢 SQL 明细、按 Trace ID 查链路 |

从 5–15 分钟的故障窗口开始，记录北京时间、环境、角色、Pod、实际镜像摘要、请求或任务 ID。默认走只读路径，先取证再变更。不要输出 Secret、整份 ENV、连接串、签名 URL、客户数据或带参数的 SQL。DBX 连接只有在核验它指向当前腾讯云实例后才可使用。

## 1. PostgreSQL

授权 SSH 登录数据库主机，在数据库主机上使用本地账号。Pod 排障按内网 DNS 和应用所在网络路径验证，避免把主机本地可用当作 Pod 到库也可用。

```bash
sudo -u postgres psql -X -P pager=off -v ON_ERROR_STOP=1 -d railway
```

```sql
SET statement_timeout = '5s';
SELECT now(), current_database(), pg_is_in_recovery(), current_setting('max_connections');
SELECT state, wait_event_type, wait_event, count(*)
FROM pg_stat_activity WHERE backend_type = 'client backend' GROUP BY 1, 2, 3 ORDER BY 4 DESC LIMIT 20;
WITH blocked AS (
  SELECT unnest(pg_blocking_pids(pid)) AS blocker
  FROM pg_stat_activity WHERE wait_event_type = 'Lock'
)
SELECT a.pid, a.application_name, a.state, a.wait_event,
       clock_timestamp() - a.xact_start AS transaction_age, count(*) AS blocked_sessions
FROM blocked b JOIN pg_stat_activity a ON a.pid = b.blocker
GROUP BY a.pid, a.application_name, a.state, a.wait_event, a.xact_start
ORDER BY blocked_sessions DESC LIMIT 15;
SELECT mode, relation::regclass, count(*)
FROM pg_locks
WHERE NOT granted AND relation IS NOT NULL AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
GROUP BY 1, 2 ORDER BY 3 DESC LIMIT 15;
SELECT datname, numbackends, deadlocks, temp_files, stats_reset
FROM pg_stat_database WHERE datname = current_database();
```

- 同时查 `df -h /pg/data`、当前 PostgreSQL 日志和 Grafana 慢日志。先 read-only `EXPLAIN`，生产大表只有在掌握代价和执行窗口时才用 `EXPLAIN ANALYZE`。用查询指纹、调用次数和统计重置时间区分累计指标与最近窗口。
- `pg_up=1` 仅说明探针可读。大量连接配合 `wait_event_type='Lock'` 时，先找锁链最上游，再定位待授予的强锁。禁止按 PID 批量杀会话或直接重启数据库。
- 已发生过 `pg_dump` 持有读锁、排队的独占锁阻挡后续查询的事故。2026-11-21 取消在线全备后，锁等待由 318 降到 0。当前 COS 目标桶为 `xiaohuiji-db-back-1512547869`。可以通过以下命令确认任务状态。该逻辑全备定时器已停用，首份完整 COS 备份尚未验收。恢复自动备份前应验证独立备份路径及恢复流程，避免直接重启这份定时任务。

```bash
systemctl list-timers --all xiaohuiji-pg-cos-backup.timer
sudo journalctl -u xiaohuiji-pg-cos-backup.service --since '1 hour ago' -n 100 --no-pager
```

PostgreSQL 官方参考见 [活动与统计视图](https://www.postgresql.org/docs/18/monitoring-stats.html)。

## 2. Redis 和任务队列

在 Redis 主机先确认本地监听与实际服务。使用已配置的 ACL 授权方式。以下命令在该实例的本地 `127.0.0.1:6379` 执行。若需密码，用终端交互的 `--askpass`，不将密码写进参数或 shell 历史。

```bash
redis-cli -h 127.0.0.1 -p 6379 --askpass PING
redis-cli -h 127.0.0.1 -p 6379 --askpass INFO clients memory stats persistence
redis-cli -h 127.0.0.1 -p 6379 --askpass SLOWLOG LEN
sudo journalctl -u redis-xiaohuiji-1-6379.service -u xiaohuiji-redis-forward.service \
  --since '15 minutes ago' -n 100 --no-pager
```

- 分别验证 `redis_up`、Pod 内 DNS/TCP、应用命令超时。客户端 `context deadline exceeded` 不能直接归因于 Redis 宕机。比较连接池、转发层、应用上下文耗时与 Redis 实例侧的阻塞客户端、连接拒绝、淘汰、内存及服务端慢命令。累计计数看故障窗口的增量。
- Asynq 查实际逻辑 DB、真实分片名和 Worker 监听配置，再核对 `pending/active/scheduled/retry/archived`、最老任务年龄及消费速率。发布分片示例为 `youban_publish_tg_urgent_<shard>` 和 `youban_publish_tg_<shard>`。沿 `tenantId/channelId/jobId/attemptId` 对照数据库持久状态和日志。
- `SLOWLOG GET` 可能含命令参数，仅限有权限的受控终端和小样本。禁止用 `KEYS *`、`MONITOR`、清库或批量重试定位问题。未知发送结果应先核对 Telegram 回执和任务状态。 [Redis INFO](https://redis.io/docs/latest/commands/info/)

## 3. 日志

生产日志统一写入自建 VictoriaLogs，入口使用 HTTPS 和 Basic Auth。密钥保存在值班机器的 `~/.codex/youban-log-token`，不写入仓库、不粘贴到排障记录。

```bash
tools/youban-logctl --help
tools/youban-logctl health
```

`health` 返回 `OK` 后，再查询最近错误或按 Trace ID、业务 ID 缩小范围：

```bash
tools/youban-logctl errors --role publish --node VM-0-8-ubuntu --since 30m
tools/youban-logctl query 'trace_id:"<trace-id>"' --service xiaohuiji-publish --limit 100
```

- 查询结果只摘取时间、等级、角色、Trace ID 和必要的错误摘要，避免把完整载荷、Token、签名 URL 或客户数据写入排障记录。
- `health` 失败时先检查日志主机 Caddy、VictoriaLogs 容器、磁盘水位和 HTTPS 证书，再检查应用 OTel exporter 队列；日志服务故障不能阻塞业务任务。
- 用 Grafana 的 HTTP 5xx 和分位耗时校验请求趋势，再按 Trace ID 查应用、数据库和 Redis 操作。不能用日志文本中的 `503` 替代真实 HTTP 状态码计数。

## 4. Pod、Deployment 与 Service

按 [Swarm 生产发布手册](../deploy/SWARM.md) 连接 Manager。只读排查先用服务名称和时间范围缩小检索。

```bash
"${k[@]}" get deploy,rs,pod,svc -o wide
"${k[@]}" get events --sort-by=.lastTimestamp | tail -30
"${k[@]}" get endpointslices -l kubernetes.io/service-name=web -o wide

pod='<实际 Pod 名>'
container='<实际容器名>'
"${k[@]}" describe pod "$pod"
"${k[@]}" logs "$pod" -c "$container" --since=15m --tail=200 --timestamps
# 容器已重启且存在上一次实例时执行
"${k[@]}" logs "$pod" -c "$container" --previous --tail=100 --timestamps
```

| 现象 | 检查 |
|---|---|
| Pending / FailedScheduling | CPU 和内存 requests、配额、容忍配置和网络子网 IP |
| ImagePullBackOff | TCR 仓库、部署摘要、节点访问及 imagePullSecret 引用 |
| CrashLoopBackOff / OOMKilled | 上一个实例退出原因、启动参数、资源上限、Secret 和 ConfigMap 引用 |
| Ready Pod 为零或响应 503 | 检查 `readyz`、Service selector、EndpointSlice ready 状态及 CLB 后端 |
| Pod 正常但消费停滞 | 按角色查队列年龄、租约、最后一条任务日志和依赖耗时 |

取证时只摘取事件或日志的必要字段。容器缺少工具时优先使用现有已授权的调试入口。需要额外探测时使用受限 Job 或调试 Pod、固定镜像摘要、短超时、资源限额和应用同一网络路径，执行 DNS、TLS、只读探测后清理。禁止在调试 Pod 或本地启动使用生产配置的消费者。

## 5. 故障收尾

记录影响角色、故障时段和样本、原因及证据、实际操作、回滚和剩余风险。分别复查入口 `/readyz`、一条真实只读业务请求、`tools/youban-logctl health` 与新增错误、PostgreSQL 锁和 Redis 队列趋势。健康探测通过只证明该次探测通过。

发布或采集故障沿同一业务 ID 关联来源消息、采集事件、资料状态、发布任务、发送尝试和 Telegram 回执。入口代码在 `server/addons/youban_publish/logic/sys/`。已配置的 `youban_ops` 工具可用于样本定位和修复预览，执行前核实实际环境及授权范围。
