# 扫图下载前标识查询：本地验证

日期：2026-10-22。仅修改 youban-server，未部署、未发送 Telegram 消息、未修改线上业务数据。

## 修改

- Bot 单图和媒体组入口先提取各尺寸 file_unique_id、真实 file_id、Bot ID、转发来源。
- 指纹缓存命中直接进入搜索；否则按当前账户可见范围只读查询发送记录，命中后跳过 getFile 和下载。
- 未命中的媒体继续原来的缩略图解析、指纹下载及搜索流程。预查询限制 3 秒，失败记录日志并继续原流程。
- 搜索与入口预查询共用身份匹配逻辑，保持原来的候选排序、最多 20 条结果和权限范围。
- 视频标识命中不再要求有缩略图。未命中且没有缩略图仍提示用户发送封面。
- 常规 Telegram 媒体优先稳定标识；仅缺少稳定标识时才使用同 Bot 的 file_id。

## 样本与本地测试

通过 DBX 从最近 5,000 条成功发送、非删除且有稳定标识的发送记录中，关联有效图片媒体与资料状态，随机抽取 100 条。媒体关联使用 hg_youban_publish_media；其 ID 不能直接作为 hg_content_media 的 ID。

私有样本保存在 server/.tmp/scan-published-100.json，不纳入提交；没有 Token。

使用本机 PostgreSQL 127.0.0.1:35432 的事务临时表重放，事务结束移除临时表，不操作正式表。覆盖 1、2、5、10、30、100 张组合，共 185 批，每种组合都遍历全部 100 条样本。30 张最后不足一组按剩余数量测试。

| 每批图片数 | 本地查询最大耗时（race 模式） |
|---|---:|
| 1 | 13.53 ms |
| 2 | 1.97 ms |
| 5 | 2.20 ms |
| 10 | 2.54 ms |
| 30 | 4.22 ms |
| 100 | 7.42 ms |

全部样本包含预期资料命中。附加验证：图片不同尺寸别名、同 Bot file_id、错误 Bot 不命中、转发频道消息匹配、未知标识、跨租户/账户隔离、已删除和失败发送记录排除。Bot 入口测试使用缓存未命中的桩，且不配置 Bot Token，验证发送记录命中后无需 getFile；包括没有缩略图的视频。

同时通过了相关原有下载并发、超时、候选合并和排序测试及 race 检查。未运行整个仓库测试。

重跑：

```sh
cd server
YOUBAN_SCAN_IDENTITY_FIXTURE="$PWD/.tmp/scan-published-100.json" go test -race ./addons/youban_publish/logic/sys ./addons/youban_bot/logic/sys -run 'Test(ScanLedger|BotMedia|RankBot|LoadBot|CachedScan|Forwarded|ScanPhoto)' -count=1 -v
```

## 线上只读 SQL 检查

DBX 对对应身份查询执行 EXPLAIN ANALYZE，包含租户/账户范围、有效资料与资料状态校验：

| 标识数 | SQL 执行耗时 |
|---|---:|
| 1 | 0.166 ms |
| 10 | 1.045 ms |
| 30 | 2.181 ms |
| 100 | 约 11 ms |

1、10、30 的执行计划确认命中 idx_ybp_tg_message_file_unique。以上为单次观察，不是压测百分位；不包含 DBX 网络、权限查询、资料详情和 Bot 回复。

## 部署条件与范围

部署前需单独执行 server/addons/youban_publish/resource/sql/upgrade_scan_file_identity.pgsql.sql，为缺失稳定标识时的 file_id 查询增加索引。使用 CONCURRENTLY，不能放入事务或插件升级事务。本轮没有执行线上迁移。

本轮覆盖身份查询及 Bot 入口免下载路径，未对真实 Telegram 下载、截图/重编码图片的相似搜索、资料详情加载和回复相册发送做全链路计时。因此不能据此宣称任意扫图完整链路 3 秒或百分百命中。本地仅 100 条临时数据，规模差异由线上只读 SQL 计划检查补充，仍不等同持续高并发压测。

现有发送记录保存的单个图片尺寸保持不变，本轮通过检查入站全部尺寸提高匹配覆盖，没有给发布成功事务增加写操作。
