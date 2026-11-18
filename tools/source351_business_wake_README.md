# Source351 business collect_event 批量唤醒

仅 Python 标准库。固定 tenant78 / owner account615 / source351；存储 account 允许 0、615。
本脚本输入 `id` 是业务 collect_event 主键，**不能传给原 raw replay 脚本**。

处理顺序：先用现有 `source351_replay.py` 完整重放已批准的 raw manifest，并完成其计划对账；
再用本脚本唤醒业务积压；最后显式 drain 并查看报告。不要直接清空 Redis 或人工修改 processed。
raw replay、业务 wake 各用独立 checkpoint，不混用 manifest/ID。

## 离线校验（默认，无网络）

从仓库根目录运行。默认输入为
`tmp/collector351-backlog-20261118-v3-business/business-pending.jsonl`，
默认 summary 是同目录 `business-summary.json`，默认审批文件是
`tmp/collector351-backlog-20261118-v2/summary.json`；可用 `--summary`、`--approval` 显式替换。
先由受控导出者记录 SHA256，运行时传入可信摘要：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 tools/source351_business_wake.py \
  --manifest-sha256 '<导出者记录的64位SHA256>' --dry-run
```

全文件一次读入冻结内存，再校验 SHA256、完整 summary、审批文件 SHA256、scope、
storageAccountCounts/statusCounts、输入/唯一数量、状态及 processedAt=null。
仅允许审批 summary 明确列出的非推送频道（数字和 -100 别名规范化校验）；没有默认额外频道。
按业务 ID 排序；完全相同重复行去重，同 ID 任意字段冲突拒绝，后半段非法也在首次网络前拒绝。
checkpoint 只保留身份字段，避免复制业务正文。当前导出记录 2001 business events、3206 media；
媒体文件不参与唤醒，脚本不声称验证媒体落库。
审批文件是受控导出证据，服务端仍须重新验证权限和身份。

## 批量运行（供主 agent 获授权后执行，本次未执行）

凭据仅从 `YOUBAN_AI_OPS_TOKEN` 注入，使用 `X-AI-Ops-Token`。
默认 endpoint `https://api.xiaohuiji.cc/api/youban_publish/ai-ops`，可设置
`YOUBAN_AI_OPS_BASE_URL` 或 `--base-url`。HTTPS、禁止重定向、响应体限额、错误脱敏复用旧客户端；
没有 DB、Redis 或 Telegram 直连。

```sh
PYTHONDONTWRITEBYTECODE=1 python3 tools/source351_business_wake.py \
  --manifest-sha256 '<同一可信摘要>' --apply \
  --checkpoint /path/to/private/source351-business.checkpoint.json \
  --batch-size 200 --max-batches 11 --wait-seconds 330 --poll-interval 2
```

checkpoint 父目录须存在，使用受保护的持久目录。全程独占文件锁，0600 临时文件，
原子替换及文件/目录 fsync。绑定 wake action、原始 manifest/summary/approval SHA256、scope、频道和 endpoint。
同一 manifest 仅用一个 checkpoint/一个执行者；文件锁不能协调不同主机或不同 checkpoint。

每批调用：

- POST `/repair/plan`：`{action:"wake_collect_events",tenantId:78,accountId:615,sourceId:351,eventIds:[业务ID],limit:数量}`，最多200。
- POST `/repair/apply`：`{planId,confirmToken}`；调用前持久保存 planId/精确业务身份和预演决定。
- GET `/repair/status?planId=...`：只查询原计划。

plan/status 的 action、planId、candidates、eventIds 完整集合与数量必须匹配。
预演 wakePreview 必须完整覆盖全部 ID，每项均校验租户、owner、存储账号、source、chat/message、
非空 `identity` 和合法 decision/status。identity 是服务端不变量 hash，脚本不自行重算；
双方都有 sourceUniqueKey/createdAt 时额外比对。
要求服务端新版本返回有效 ISO8601/RFC3339 日期（Go 使用 `gtime.Time.Time.Format(time.RFC3339)`）。
脚本严格解析 `expiresAt`、要求时区且确认尚未过期；旧版本错误格式
`2006-01-02CST15:04:05Z07:00` 会被拒绝，不做兼容或跳过过期校验。
terminal/blocked 同样校验 scope，明确跳过并仍允许整批 apply，
由服务端确保这些事件不入队。预演 status 可以与导出时不同。令牌仅在内存，不进入 checkpoint/日志。

running 在每计划等待预算内轮询，单次请求还受 `--timeout`（默认45秒）及剩余等待预算限制。
未知 API/旧部署字段不全、status 身份不符、pending/failed/unknown、running 截止均停止。
apply 超时、断连或崩溃后保持 current planId；重跑相同命令先查原 plan，绝不再次 apply 或生成替代计划。
即使崩溃在保存计划后、发送前，也保守停在 pending 等待人工对账。不要删除或手工修改 checkpoint。
completed 才推进 cursor；若计划返回明确 failed IDs 则停止并保留 current，不自动重试。
checkpoint 保留最近 status、停止原因及每批预演决定，便于对账。

## 显式 drain 报告

只有加 `--drain-seconds` 才启用全部已完成 history plans 的进度轮询。
对已经唤醒完的 checkpoint 重跑以下命令不会重新提交计划：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 tools/source351_business_wake.py \
  --manifest-sha256 '<同一可信摘要>' --apply \
  --checkpoint /path/to/private/source351-business.checkpoint.json \
  --batch-size 200 --max-batches 11 --wait-seconds 330 --poll-interval 2 \
  --drain-seconds 600 --drain-interval 10
```

服务端对每计划精确业务 PK（最多200）查 DB，客户端默认每10秒轮询一轮所有 history plans，
不会发全库扫描请求。进度必须含 `total,pending,ignored,dispatched,processed,failed,missing` 七个非负整数，
六分类和等于 total，total 等于计划完整 ID 数量。missing 包括原 scope 不存在或身份变化。
字段未实现/无效直接停止，不降级猜测完成；最新每计划计数与等待/失败原因保留 checkpoint。

`businessComplete=true` 必须同时满足 manifest 全部计划 completed、显式 drain 成功且所有历史计划
pending/failed/missing 均0。该值只表示采集交接终态，**不代表媒体入库成功或 TG sent**。
普通 completed 仅表示唤醒计划完成，未显式 drain 永远输出 businessComplete=false。
failed/missing 不自动重放，保留计数等待人工定位；截止后可用同一 checkpoint 再次显式 drain。

## 本地测试

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tools -p 'test_source351_*.py' -v
```

新测试仅使用临时目录和 mock API，不读取生产凭据或访问网络。
