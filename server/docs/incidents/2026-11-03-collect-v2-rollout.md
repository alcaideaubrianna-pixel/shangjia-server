# 采集 V2 接手与上线记录

## 2026-11-03 15:25 Asia/Shanghai

本轮目标仍为完整架构替换与旧路径删除，尚未完成。禁止将以下测试当作生产全链路验收。

- 独立 `collect-process-worker` 角色已实现（239ec4fc、74e39153），尚无独立 SAE 应用映射，未切换线上角色。
- spool 重复接收保持原始凭据、完成状态和重试时间不变，PostgreSQL 使用 `RETURNING id`（0ff00e99）。
- 缺少事件标识的 spool payload 隔离为 dead，更新失败向上返回，不吞掉隔离失败。
- DBX 已在 PostgreSQL_Xiaohuiji 执行新增 spool 表和索引迁移；回查 13 个字段、主键、租户事件唯一索引及 due 索引均存在。未修改旧事件或清队列。
- SAE collector 只读核对：镜像 b2add972、collector-worker、1 副本、1000 CPU、2048 MiB。尚未部署本轮版本。

## 本地验证

- 临时 PostgreSQL 仅监听 127.0.0.1:55489，数据目录位于系统临时目录。
- `YOUBAN_SPOOL_TEST_PORT=55489 go test -race ./addons/telegram_collector/logic/sys -run TestIngestSpoolPostgres -count=1` 通过：迁移重复执行、重复事件、保留重试调度、完成后重复接收、跨租户同键、损坏事件隔离。
- 私有线上 fixture 4371 条回放通过：6 accounts、83 sources、2601 realtime、1770 history、2642 media、2163 grouped；queue-to-test-sink P95 861.617ms，duplicates=0、lost=0。测试接收端不会发送 Telegram 或写生产资料。
- Collector、Publish logic、Account runtime 相关包测试通过；应用编译通过。
- `go test ./...` 失败：Bot 菜单/审计帮助断言、Relay 输入校验、casbin/handler/hook 数据库驱动初始化。菜单/审计帮助及 Relay 输入校验失败在独立 b2add972 worktree 上复现；数据库初始化失败尚未完成基线复验。

## 剩余门槛

1. 等待最新提交镜像完成，核对不可变标签和 SHA；不得部署旧的构建任务版本。
2. 部署前快照全部角色镜像、ENV 差异及回滚目标；独立 worker 需新增应用或明确迁移方案。
3. spool 恢复仍缺少多 worker 领取租约；入口当前仍同步完成 Event 和 enqueue，并非最终异步接收架构。
4. source circuit、spool 故障恢复、媒体/分组/审核/发布隔离仍需补齐测试和线上验收。
5. 按架构文档完成 5/15/30 分钟及连续 24 小时对账后，再逐段删除旧消费者、兼容路由和旧恢复路径。

回滚保持新增 SQL 表和全部任务；仅回滚应用镜像与消费者配置。不能清空 Redis 或批量重置任务。

## 2026-11-03 16:25 Asia/Shanghai 更新

- 全部旧 SAE 角色已部署 a2150ee，并保持 Web 2 副本、Account 1 副本。
- spool 恢复已增加原子 SKIP LOCKED 领取及 attempt fencing。新增处理租约索引已在线上创建。
- PostgreSQL race 测试验证 12 个并发领取不重复、过期重新领取、迟到 ACK 不覆盖新 owner；Redis 故障后恢复成功且 Event 不重复。
- 有界生产样本：实时 process Outbox delivered=983，采集 Event processed=170、dispatched=15；仍存在历史 processing 积压，不代表全链路 SLO 达标。
- 新 SAE 应用 e3a5862c-5746-4d47-8b11-bc7ae25eb090（xiaohuiji-sae-collect-process）已创建但未部署；1 核、2048 MiB，DB pool 上限 12，HTTP readiness=/readyz、liveness=/healthz。
- 新隔离开关 YOUBAN_COLLECT_PROCESS_ISOLATED 默认 false。独立 worker 启动 Delivery 和业务处理；开启后 collector 仅保留原始 Event 处理，publish 不再消费 Collector Delivery，scheduler 不再运行采集 recovery。
- 切换必须先通过部署脚本启动 collect-process，再给 collector、publish、scheduler 设置隔离开关。脚本会核对独立 worker 就绪且镜像与目标一致。回滚先关闭旧角色开关恢复消费，再停止独立 worker，保留全部队列和表。
- CI 镜像标签使用 7 位 SHA；不能把 8 位提交简称查询失败当作构建/推送缺失。
