# 推送耗时稳定性 review

## 已验证的优化

`telegramVideoMeta` 对有效的可复用 file_id 不再读取附件表、下载视频或运行 ffprobe。内存中已有宽高时仍使用已有元数据。单视频和相册视频共用该入口；强制上传、防扫图上传、copy 引用仍走原路径。没有改变 API 成功确认、Attempt、发送顺序或未知结果的处理。

线上 job 2065649 的样本：sendMediaGroup 1567ms、sendVideo 895ms、视频元数据准备 2211ms。这个优化消除的是元数据准备的外部依赖，不能据单条样本推算全站提速比例。

本地测试覆盖：无源文件的 file_id 元数据快路径、已有元数据保留、相册视频构建、真实发送方法对本地 HTTP 模拟 API 的单视频请求和成功响应、请求取消后仅一次发送。测试不连接生产 DB、Redis 或 Telegram。

## 其他路径的结论

- 媒体读取：展示和验证分别查询，可合并为一次读取再按用途分组。必须保持 sort_index/id 排序、用途隔离、同 Bot file_id 资格、防扫图与编辑媒体限制。需要单独验证当前快照与编辑并发行为，本次不混入。
- 任务读取：锁前、锁后、认领后重复读取，不全是冗余；锁后重查和认领条件用于处理下架、替代操作及并发 worker，不能直接删除。
- 元数据缓存：已有附件持久化缓存，但缓存未命中会同步下载探测。后续应由 Media 提前生成，按有效附件/内容版本关联，避免原视频元数据错误套到编辑或转换后的文件。
- 频道衔接：已有释放锁后 wakeNextTelegramChannelJob。它只选择已到 next_retry_at 的任务；若只有频道忙退避未到期，会留下短暂空档。可增加明确等待原因后只提前唤醒频道忙任务，不能取消 FloodWait、网络错误、媒体未就绪等退避。最近快照没有未来的频道忙退避，不能认定这是当前所有频道慢的原因。
- 发布配置已有5分钟缓存，不能把它当成每条必查数据库的优化项。
- 成功确认事务、发送前可发布校验继续保留；不要以异步写入或同频道并行交换速度，否则引入重复或乱序风险。
- 资料清理仍使用频道发送锁；拆锁必须先区分独立删除和同资料重新上架前清理，避免删除与新发送竞态。本次不改该状态机。

## 线上耗时日志

每次 SendTelegramJob 执行结束新增一条 `TG推送耗时汇总`，`log_type=telegram_publish_summary`。按 jobId 串联重试，按 executionId 区分每次消费，channelId 区分频道；保留 taskId、queueName、instance、traceId。SLS 可先搜索“TG推送耗时汇总”及目标 jobId，再用同 executionId 查“TG推送链路”各阶段 start/end/error/slow。

- executionDurationMs：本次执行墙钟耗时；不是资料从创建到发送成功的总耗时。
- jobAgeMs / dispatchAgeMs：最近加载任务时距创建/派发的时间；派发时间可能随重试变化，不能当作纯队列等待耗时。
- stageDurationMs / stageCounts：同名阶段累计耗时/调用次数；阶段存在嵌套和并发，不能全部相加或相减推算数据库耗时。
- deferredReason：如 channel_lease_busy、media_not_ready、earlier_channel_job。
- returnedError：只表示本次调用是否返回错误；false 不代表已经发送成功。最终成功仍看 job.complete 及数据库终态；协议号降级和对账可能跨执行完成。

汇总是执行内存诊断，不新增数据库/Redis 请求或队列，不持久化文件 URL、file_id 或 Bot Token。进程被强杀可能没有结束汇总，仍需用现有阶段 start/slow 与实例重启日志排查。

## 验证边界

相关测试、频道锁/顺序/Attempt 回归及构建通过。全量 go test ./... 未通过，包括安装 SQL 测试、已知分类/防扫图断言失败，以及依赖本地 PostgreSQL/Redis、数据库驱动和存储配置的失败。完整输出在本机 /tmp/youban-publish-stability-tests.log，未提交。不能将本次局部测试通过表述为全量通过或线上稳定性已验证。
