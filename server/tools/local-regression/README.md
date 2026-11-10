# 通用本地回归测试 Agent

本目录是所有业务模块共用的本地完整链路回归入口。测试必须优先验证状态机和事件链，而不是只验证单个函数。

## 强制规则

1. 测试 fixture 的表结构必须从生产迁移或 `information_schema` 导出，禁止手写“近似表结构”。
2. 临时表字段不得多于线上真实表；尤其不能为了让查询通过而补充线上不存在的字段。
3. 每条新增 SQL 必须在真实 PostgreSQL schema 上执行一次集成测试。
4. 回归必须覆盖成功、失败、超时、重试、重复点击、部分落库和 worker 重启。
5. 测试必须断言数据库状态、队列状态、调用顺序和最终通知，不能只断言 HTTP 200。
6. Telegram 调用统一指向本地 Hook Mock，禁止访问真实 Telegram。

## 媒体上架必测链路

```text
创建资料
→ 保存媒体意图
→ 媒体任务入队
→ 点击上架
→ waiting_media
→ 媒体任务完成
→ 自动唤醒
→ display Attempt
→ verify Attempt
→ 发布成功或可重试失败
```

如果第一次点击返回 `waiting_media`，测试必须继续等待媒体任务完成，并断言无需第二次点击即可创建发布任务。

## 当前回归命令

```bash
YOUBAN_PUBLISH_INTEGRATION=1 \
go test ./addons/youban_publish/logic/sys -count=1
```
