# 2026-11-04 发布与连接超时代码排查

时间：Asia/Shanghai。此轮仅本地代码修复与隔离测试，没有部署、重发或修改线上任务。

## 已复现并修复：轻量防扫图退出后使用已取消的 context

线上 tenant_id=73 的 job 2747807、2747811 对应 account task 6116861、6116860，错误为“读取生成媒体归属失败 … context canceled”。

`prepareTelegramAntiScanUploadFile` 先注册索引 defer，随后创建 lightweightCtx、注册 cancel defer，并把 ctx 替换为 lightweightCtx。defer 逆序执行：cancelLightweight 先执行，索引 defer 再读被替换的 ctx，导致成功生成图片后数据库归属查询必然收到 context canceled。背景替换直接成功不经过该分支，轻量处理和背景替换失败降级会触发。

修复：索引 defer 在注册时捕获调用方 context；图像生成仍使用原有子超时。未使用 Background/WithoutCancel 绕开父任务取消，索引失败仍阻止发送，保持可搜索性和重试语义。

git blame 显示子 context 的替换和取消由 84b74b26（2026-11-01）引入；这说明代码触发条件，不证明今晚所有网络超时由此产生。

## 验证

新增真实本机 PostgreSQL 集成用例 lightweight_upload_indexes_with_live_context。修复前报与线上相同错误，且 parentErr=nil；修复后生成文件成功并写入四个搜索别名桶。取消调用方的现有测试继续通过。

通过命令：

```sh
YOUBAN_SCAN_STORAGE_PG=1 go test -race ./addons/youban_publish/logic/sys \
  -run '^TestMediaSearchStorageLocalPostgres$/(lightweight_upload_indexes_with_live_context|original_media_and_projection)$|^TestPrepareTelegramAntiScanUploadFileStopsOnCancelledContext$|^TestPrepareTelegramAntiScanMediaSetTimeoutFallsBackWithoutCancelingPublish$' -count=1
```

数据库仅监听 127.0.0.1:35432，使用临时表/事务，无 Telegram 发送。既有 generation_failure_falls_back_to_source_upload 用例另行运行失败：测试未提供云资源配置表，查询使事务进入 aborted 状态；不能将此用例列为通过。

## 其余排查结论与边界

- 连接释放：检查的发布、资料、地区查询使用 GoFrame Scan/One/All/Transaction，没有发现业务代码直接持有未关闭的 sql.Rows。不能据此宣称全仓无泄漏，也不能从 ClientWrite 单独推导出连接池耗尽。ClientWrite 仅证明数据库等待客户端/链路接收结果。
- 慢查询：采集列表仍取 e.*，需要结合请求范围、实际结果字节数和执行计划评估；省份目录已有一分钟以下的查询超时（当前实现为一秒）、缓存和刷新互斥。尚无证据支持本轮改索引或扩大连接池。
- 任务锁：featureMu 的查询已在锁外。媒体组 finalize 路径已在 consumeProfileCreatePart 前解锁；单条消息 consumeProfileSessionMessage → consumeProfileSessionMessageLocked → consumeProfileCreatePart 仍持会话锁跨媒体解析/持久化，存在同会话阻塞风险，不能简单移除锁破坏状态幂等。
- Redis：token.Parse 的缓存读取错误被替换为“登录身份已失效”，真实 Redis 故障只在 Debug 级别记录，导致运维误判。应区分存储不可用与 token 不存在，保留失败关闭鉴权。该基础库按仓库规范禁止直接修改，本轮未改。
- 请求日志：hook/access_log 已使用 SafeGo，日志队列失败不能直接解释为请求线程同步等待。但每请求启动后台协程，没有并发上限，Redis 故障时存在额外压力；尚未证明它是连接池耗尽源头。

## 上线与回滚

本轮修改未提交、未部署。需构建并部署使用该函数的 publish/account 路径后，再核对上述任务及对应发送确认记录。不可直接把未知发送结果改 pending，避免重复发送。代码回滚仅涉及本轮两个文件的具体差异，不回滚用户其他修改。
