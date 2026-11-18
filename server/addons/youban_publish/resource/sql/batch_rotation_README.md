# 批次连续轮播共享读模型

必须先迁移再部署。成功回写依赖新列，不提供运行时 DDL/schema fallback。
全新安装结构已同步 install.pgsql.sql；既有数据库必须显式执行以下迁移，不在启动或在线升级时自动建索引。
数据库角色需具备 sequence USAGE；sequence 从 1 起独立分配，无 MAX 大表初始化。

## 显式执行顺序

使用已核对环境的 psql 连接，以下文件均位于本目录：

1. `psql -X -v ON_ERROR_STOP=1 -f batch_rotation.pgsql.sql`
2. **独立 autocommit** 执行 `psql -X -v ON_ERROR_STOP=1 -f batch_rotation_indexes.pgsql.sql`。
   禁止 `-1`、外层 BEGIN 或事务型 migration runner。
3. `psql -X -v ON_ERROR_STOP=1 -f batch_rotation_validate.pgsql.sql`，检查 `pg_index.indisvalid/indisready`。
   并发建索引失败可能留下 invalid index，`IF NOT EXISTS` 不会修复它。
   确认无正在构建任务后独立执行 `DROP INDEX CONCURRENTLY idx_ybp_batch_rotation`，重新执行步骤 2、3。
4. 执行 `psql -X -v ON_ERROR_STOP=1 -f cycle_batch_snapshot.pgsql.sql` 创建每日快照结构。
5. 独立 autocommit 执行 `psql -X -v ON_ERROR_STOP=1 -f cycle_batch_snapshot_indexes.pgsql.sql`，核实索引 valid/ready。
6. 历史回填完成并验证之后启用 scheduler。索引不可用时禁止启用新读路径。

结构与索引迁移可重复；sequence 不重置。回滚应用可保留新列/索引，不回退 sequence。

## API 与语义

- `batchRotationCandidatesTx(ctx, tx, tenantID, channelID int64, limit int) ([]batchRotationCandidate,error)`：
  `ProfileID/AccountID/Order int64`；limit≤0 返回空，硬上限 5000。
  从 partial 索引按 `(batch_order,profile_id)` keyset 获取 materialized 小页，
  再以 profile 主键及 account/channel 权限复核。拒绝的行也推进游标。
  不检查历史 sent、task_status、媒体就绪；order=0 不参与。
  极端大量失效映射可能需要多个有界查询；没有运行时 COUNT 或历史回填。
  调用方负责频道锁、事务及任务预留。estimate 可以直接复用 cp 的
  tenant_id/channel_id/status/batch_order 和 `batchRotationEligibilitySQL()`。
- `s.admitBatchRotationProfileTx(ctx,tx,job)`：新 job insert 后同事务调用。
  调用方已有 profile 生命周期锁和权限校验；status2/private/deleted 不入队。
  account0 合法。仅新增 active 成员或补 active 的零 order；
  不改 last_job_id，不改 active 已有正 order；显式新 job 的 ID 大于已有 last_job_id 和成功水位时，可恢复 inactive 并分配新 order。旧成功回调不走 admission。
- 成功 hook `upsertChannelProfileFromJobTx`：独立 sequence 移至队尾，
  用 `batch_last_sent_job_id` 严格递增防止同 job 重放及旧 job 迟到。
  inactive 冲突行绝不恢复；account0 同样处理。
  profile 生命周期锁仍由调用方维护。迁移完成前不得部署此 hook。

## 既有零 order：先暂存完整范围，再按历史时间赋值

暂停目标频道调度/写入，记录固定 `upper_id`（显式主键上界，可通过 PK DESC LIMIT 1 获取）。
对 mapping 表以主键分页，每次≤1000，带
`-v tenant_id=... -v channel_id=... -v after_id=... -v upper_id=... -v batch_size=500`
执行 `batch_rotation_backfill.pgsql.sql`。
按输出页面最后一个 id 推进 after_id，不能按插入暂存行推进。空页结束。
该阶段每页只扫描≤1000 个 mapping，关联 `last_job_id` 的 job 主键取得 sent_at。
临时中断可重跑，暂存表 mapping_id 唯一。

完整 scope 暂存后，重复执行 `batch_rotation_assign.pgsql.sql`（tenant_id/channel_id/batch_size），
直到 consumed=0。按 `(sent_at NULLS FIRST,mapping_id)` 分批取暂存索引，分配 sequence。
未知 sent_at 放在已知成功之前，按 mapping PK 作确定性 fallback；
**零 order 本身没有历史顺序，未知时间绝不称为真实发布顺序**。
相同 sent_at 以 mapping PK 打破并列；不同 scope 间无需相对顺序。
整个 scope 必须单执行器，暂存完整前禁止赋值，否则分批历史时间不能保证全局顺序。
部署新代码前执行回填；赋值带 `cp.last_job_id=暂存.last_job_id` CAS。
旧代码若并发成功写入，CAS 跳过并报告 cas_skipped，消耗该暂存行以便重新采集。
在停写窗口重新暂存完整固定范围并重跑，已赋正 order 保持不变。
严格恢复全频道历史顺序必须从完整采集至赋值期间停写；仅 CAS 重跑保证安全和收敛，
不保证并发写入期间重建精确历史先后。新代码部署后由成功 hook 接管顺序维护。
在线已有正 order 保留，不会声称离线回填可重建与并发新发送的完整历史。

## 缺失映射

按已授权的单 tenant/account/channel 运行 `batch_rotation_missing.pgsql.sql`，
同样用 profile PK 固定上界分页，每次≤1000；不做运行时扫表。
新增资料没有历史 sent 也参与，以明确的初始化顺序进入队尾；inactive 不恢复。
该工具是显式 admission，操作者须预先核对手动/默认频道授权；候选读取仍复核权限。

## 定向测试

`batch_rotation_postgres_test.go: TestBatchRotationPostgres` 使用
`YOUBAN_BATCH_ROTATION_TEST_PG_PORT=55491`，固定 localhost、postgres、trust、
独立高端口；未显式设置时跳过，其他端口拒绝执行。事务临时表覆盖资料资格、
account0、未 sent 入队、重复 admission、不覆盖 last_job_id、inactive、
重复/迟到成功幂等。最终收口的上限 5000、新 job 重新 admission 及 reserve 改动由主 agent 统一复测。主 agent 负责整包低并发验证，本任务仅用隔离文件 harness。

## 冻结的 snapshot 领取 API

`reserveBatchRotationCandidatesTx(ctx,tx,tenantID,channelID,candidates) error`，最多5000项。
以 ProfileID/AccountID/原 Order CAS 分配 sequence 新位置；复核 active 和 profile status1/public/未删除。
不改发送状态、last_job_id 或成功水位。snapshot 保存原 Order，必须与 reserve 位于同一事务；
任意行身份/资格/顺序变化返回 error，调用方必须回滚整个事务并重新取候选，禁止吞错提交。
因此 batch_order 表示轮播位置，snapshot 尝试领取与实际成功均会移队尾，避免前排失败饿死后排。
