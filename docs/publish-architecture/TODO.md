# 架构重构 TODO 与验收状态

本文件是实施清单。每完成一项，必须填写提交号、测试命令和真实运行证据；没有证据的项目保持“进行中”。

## 已实现，待真实验收

以下项目已经有代码提交和定向测试，但尚未同时完成 PG、Redis、TG Fake 的完整回放，因此不能标记为最终完成：

- [ ] 频道发布 Redis ZSET：`f5c35472`
- [ ] 频道删除 Redis ZSET：`f5c35472`
- [ ] BOT `account_id=0` 来源分片：`87e04487`、`eb6744a0`
- [ ] 循环账户分片和游标：`9c0c3772`、`0d967ab6`
- [ ] 采集 Outbox 字段裁剪：`ab9db20d`
- [ ] 废弃任务唤醒频道下一条：`ef1496f1`
- [ ] 采集/发布结构化日志：`5ab9f9fa`
- [ ] 执行阶段频道队首闸门：`51eb0a05`
- [ ] 跨账户同目标频道队首判断：`72f2cb02`、`30426e65`
- [ ] 统一资料与消息发布任务创建入口（`99d42975`、`809c1406` 已收口主要资料入口，仍待 DO 字段迁移和完整回放）

当前真实回放已经证明 1000 条任务可以全部进入 `sent` 并被 TG Fake 收到，但曾发现 1 条同频道越序。因此队首闸门和跨账户判断必须重新回放确认后才能勾选。

## 进行中

- [ ] 统一 `PublishJobCreator` 覆盖手动、采集、循环、批次、AI 入口。
- [ ] 收口 `profile_publish_job.go` 等仍直接调用 `ensureTelegramProfileJobWithMetaTx` 的旧资料发布路径。
- [x] 使用 GoFrame CLI 同步 `do.YoubanPublishTgJob` 的 `send_phase` 等字段：`08113c59`。统一入口已改用 DO/DAO；历史逻辑仍需逐模块迁移。
- [ ] 循环/批次统一 `cycle_plan → cycle_run → tg_job` 游标模型。
- [ ] 真实 TG Fake 回放：1000 条、多频道、跨账户同频道、手动插队。
- [ ] 超时释放频道租约、ZREM、唤醒下一条的真实回放。
- [ ] Redis 索引删除后由 Outbox 自动回填的真实回放。
- [ ] DBX 对 300 万资料表和采集/TG Job 热点 SQL 完成 EXPLAIN 验证。
- [ ] 线上检查所有新增索引 `indisvalid=true AND indisready=true`。
- [ ] 将历史 `g.Map` 写库和手动时间字段逐模块迁移到 DO/DAO。
- [ ] 完成删除队列独立 Worker 的隔离和失败退避验收。

## 完成标准

每个 TODO 必须同时具备：

1. 中文 commit；
2. 定向 GoFrame 测试；
3. 真实 PG/Redis/TG Fake 或 DBX/SLS 证据；
4. 文档状态更新；
5. 明确回滚方式。
