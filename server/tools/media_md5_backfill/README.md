# 媒体 MD5 止血与历史回填

修复分支：`fix/collect-media-md5-20261016`。程序只处理未软删除、`ready`、MD5 为空的图片/视频；从 `file_url`（或 HTTP storage_path）流式下载原文件，不使用封面、不猜测 ETag、不修改发布状态。

依赖 Python 3、psycopg2、requests。数据库服务器已有依赖，使用 postgres 本地 peer 认证，无需导出密码。默认 dry-run，必须显式传 `--apply` 才写库。

线上目录：`/var/lib/postgresql/media-md5-backfill-20261016`。

```sh
# 试算：独立 dry-run/apply 断点，试算不会跳过正式写入。
sudo -u postgres python3 /var/lib/postgresql/media-md5-backfill-20261016/backfill.py --state /var/lib/postgresql/media-md5-backfill-20261016/full.sqlite --limit 100
# 正式回填。相同 state 重跑自动续传；高水位固定为首次运行最大 ID。
sudo -u postgres python3 /var/lib/postgresql/media-md5-backfill-20261016/backfill.py --apply --state /var/lib/postgresql/media-md5-backfill-20261016/full.sqlite --workers 2 --mbps 4 --batch-size 50 --limit 1500000
# 重试失败记录；同一时间仅允许一个进程，通过 PG advisory lock 保证。
sudo -u postgres python3 /var/lib/postgresql/media-md5-backfill-20261016/backfill.py --apply --retry-failures --state /var/lib/postgresql/media-md5-backfill-20261016/full.sqlite --limit 1500000
```

每 10000 个主键范围扫描，默认每次获取 50 个候选。每条更新前检查精确 ID 的数量，再通过 MD5 为空、ready、原 URL/path 未变的条件更新；同一事务同步媒体指纹表，并校验两表 MD5 一致。已有不同指纹、删除标记冲突会回滚，记录为失败，不覆盖。仅新增缺失指纹行时使用已有 pHash，缺失 pHash 不在本脚本计算。

下载有连接/读取/总时限、单文件大小上限，检查 HTTP 状态、内容类型和已知文件大小。大小不符、地址失效、无 HTTP 原文件的记录写入本地 SQLite 重试清单。日志只含 ID、摘要和错误类型。`SIGTERM`/`SIGINT` 可停止，重跑可续传。

检查进度：

```sh
sudo -u postgres python3 -c "import sqlite3; c=sqlite3.connect('/var/lib/postgresql/media-md5-backfill-20261016/full.sqlite'); print(c.execute('select status,count(*) from audit group by status').fetchall()); print(c.execute('select * from checkpoint').fetchall())"
```

修复范围：媒体原文件 MD5 + 媒体向量指纹表。历史采集事件指纹、资料聚合去重签名和缺失 pHash 不在此脚本内，不使用 URL 跨租户批量关联。现有扫图结果缓存最多等待原 TTL 生效；本脚本不会清空 Redis。新增采集提交仍通过现有链路同步事件和资料指纹。

停止任务可使用 `systemctl stop youban-media-md5-backfill-20261016`。代码回滚使用上线前镜像 SHA；回填为补全数据通常无需回滚。如确需撤销，先停任务，依据 SQLite 中 `status=applied` 的精确 ID/摘要逐条审核，在事务中仅清理仍匹配的值，禁止全表置空；新插入指纹行需单独判断，不能直接批量删除。

验证：Go 原文件 MD5 测试、采集链路相关测试、`go build ./...`；Python 流式下载、大小校验、HTTP 错误和日志脱敏测试；线上 pilot ID 4000001–4000005 已成功补全且两表一致。
