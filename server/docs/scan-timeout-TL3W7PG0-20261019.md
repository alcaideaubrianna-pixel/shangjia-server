# TL3W7PG0 扫图超时修复

## 根因

线上四张图片均已命中指纹缓存，但解析阶段仍调用 Telegram getFile，最长约 2.49 秒。权限范围为租户 5、账户 6/552，原 LSH 候选查询在 MD5、精确 pHash 查询之后超出共用的 2 秒预算；组图 fail-fast 随即取消其余查询。

第三轮回放还发现防扫图指纹表大量页面未达到 all-visible，覆盖索引仍产生约 1.4 万次 Heap Fetches，首次查询的该分支耗时约 1.75 秒，导致再次触及预算。线上执行普通 VACUUM ANALYZE，并将该表自动 VACUUM 的更新/插入比例调为 1%、ANALYZE 比例调为 2%，降低默认 20% 阈值下维护滞后的风险。自动维护不能替代负载监控，也不能保证任意数据库负载下的延迟。

另发现媒体组回执查询缺少 chat_id/media_group_id 组合索引，导致扫描频道历史并产生并发数据库负载。

## 修改

- 已有有效指纹时跳过 getFile，搜索服务仍重新验证缓存。
- 通过 EXPLAIN（不执行查询）的行数估算选择 SQL：小范围读取一份完整分桶投影并计算距离，大范围保留 LSH。只缓存策略 30 秒，不缓存权限判断或扩大权限。
- 原图及防扫图指纹都参与查询，阈值仍为 12，保留存活资料、媒体、租户和账户校验。
- 组图共享 2 秒候选查询预算、最多并行 2 张。一张失败不取消已确认的匹配；全部失败仍返回错误，不伪装成未找到。
- 新增媒体组回执部分覆盖索引，线上已并发创建并确认有效。

## 验证

针对性 PostgreSQL 集成测试及 race 检查通过，覆盖距离 12/13 边界、租户隔离、两种 SQL 结果一致、失败成员隔离和缓存命中跳过 Telegram 解析。

使用 TL3W7PG0 的真实四张指纹、真实权限及生产只读数据库回放搜索服务：4 个单张、2/3/4 张组图，每例首次清空测试进程的查询缓存，再重复查询。前两轮共 28 次均通过结果与耗时断言：首次 57–676ms，重复 3.6–19.1ms。候选资料编号与阈值 12 的基准 SQL 一致；此次未额外人工判断图片视觉一致性。

不包含 Telegram 网络下载和消息收发耗时，不能据此声称所有新图片端到端都已满足 3 秒。未清空生产数据库共享缓冲区。

可复用只读回放入口为 TestBotSearchProductionFingerprintReplay：显式设置 YOUBAN_SCAN_LIVE_REPLAY=1 和 YOUBAN_SCAN_FINGERPRINT_FIXTURE；JSON 包含 tenantId、accountId、fingerprints、singleExpected、expected。fixture 涉及线上数据，禁止提交仓库。测试强制数据库默认事务只读，不发送 Telegram 消息、不写生产缓存。

完整包测试仍存在其他既有失败（抠图并发默认值、帮助设置校验和缺少本地数据库环境），不声明完整测试套件通过。

## 发布

应用代码需构建发布 account；本次 Bot 入口和搜索服务同属该角色。account 发布沿用先停旧实例、确认 0，再启动新版本单实例的流程。

索引脚本：addons/youban_publish/resource/sql/upgrade_bot_message_source_group_match_20261019.pgsql.sql。须在事务外执行；生产本次已完成，无需重复创建。

指纹表维护脚本：addons/youban_publish/resource/sql/upgrade_phash_alias_vacuum_20261019.pgsql.sql；本次线上已执行。第三轮失败后完成维护，继续回放，不以首次两轮通过代替稳定性验证。

完成维护后，连续三轮共 42 次查询全部通过：首次 47–731ms，重复 3.8–6.0ms；同一防扫图分支 Heap Fetches 从 14131 降到 95，实测耗时从 1752ms 降到 4.7ms，完整候选 SQL 约 27–30ms。以上为测试时负载下的实测值，不是所有外部网络场景的保证。
