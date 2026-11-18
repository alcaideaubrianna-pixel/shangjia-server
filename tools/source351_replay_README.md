# Source351 本地有界重放

`source351_replay.py` 仅使用 Python 3.8+ 标准库（macOS/Linux），固定
tenant78/account615/source351。无需 Go、DB、Redis 或 Telegram 客户端。
这是现有原始消息重放接口的串行调用器，不是新批量入库引擎。
每批默认且最多 200 条，默认每次只处理 1 批；`--max-batches` 有明确上限。

## 导出约定

### 兼容 collect_backlog_export.py 的无 header 导出

导出完成（`summary.json` 含 `finishedAt`）后，先离线转换，不会调用生产接口：

```sh
python3 tools/source351_prepare.py \
  tmp/collector351-backlog-20261118/manifest.jsonl \
  --output tmp/collector351-backlog-20261118/prepared.jsonl
```

自动读取同目录 `summary.json`（可用 `--summary` 指定）。必须核对 summary 的
tenant78/account615/source351，**每一行必须显式含 tenantId78/sourceId351**；缺字段或跨 scope
立即失败，不从默认值补租户/来源。accountId 从已验证的 summary 补入，行中若提供则必须615。
仅允许 summary.channels 内且不在推送排除列表的频道；将数字 chat 别名转为 `-100` 形式。
本地 SQLite 按原始 ID 排序，仅合并 ID/chat/message/scope 相同的重复项；同 ID 内容冲突即失败。
校验原始行数等于 summary.rawMessages，拒绝未完成或准备期间变化的 summary。
prepared.jsonl 含 header、全 scope 行字段和期望 rule338，不覆盖已有输出。

输出包含 prepared 的 `sha256`、输入/去重数量以及每个实际频道的 `chatArguments`。
使用这个 sha256 对 prepared.jsonl 运行 replay，显式追加输出列出的频道参数；
`--chat=-100频道ID` 现在可省略 `:338`，仍逐频道显式指定，不自动扩展授权范围。
转换只预处理原始事实，不做消息配对、本地匹配或新的批量入库；summary 仍是受控导出的证据，
不是独立服务器授权证明。prepare 不修改原始导出文件或其他 agent 的导出脚本。

由主 agent 在受控数据库导出流程中核对 source 归属、同 Bot 原始来源、账号频道审批、
未处理条件和导出截止游标；本脚本不执行 SSH 或生产导出。
JSONL 首行必须包含以下三个 scope 字段，允许附加导出证据字段：

```jsonl
{"sourceId":351,"tenantId":78,"accountId":615,"exportedAt":"2026-11-18T12:00:00+08:00","approvalChecked":true,"publishChannelsExcluded":true}
{"id":123456,"tenantId":78,"accountId":615,"sourceId":351,"messageId":123,"chatId":"-1004411799859"}
{"id":123460,"tenantId":78,"accountId":615,"sourceId":351,"messageId":124,"chatId":"-1004411799859"}
```

示例 ID 是占位值，不能当生产数据执行。`id` 是 `hg_tg_collector_event.id`，
不是 publish collect_event ID、update ID、资料 ID 或 Telegram messageId。
兼容此前约定的 `collectorEventId` 字段。整份文件按原始 ID 严格升序，禁止重复和空行。
ID/messageId 使用 JSON 整数，chatId 使用字符串。可选 `originSourceId`、`ruleId`，
提供后会与 preview 逐项比对；不要求原始 source 都等于目标351，但服务端必须核对同租户同 Bot。

默认允许 `-1004411799859:338`。增加频道必须逐个传 `--chat=CHAT_ID:RULE_ID`，
且仍须通过服务端审批检查。以下五个推送目标始终拒绝，发现即整份 manifest 校验失败，
不会静默跳过：

```text
-1003438550143
-1003763589169
-1003763752459
-1004429787816
-1004449446193
```

manifest header 是导出者的声明，并非数字签名或服务器授权凭证。
必须由导出者记录并交付 SHA256；脚本强制匹配 `--manifest-sha256`，
复制本地快照后先验证全文件，再允许任何网络操作，防止中途替换/后半段非法记录。
已有导出若没有 header，先在本地生成带 header 的新文件，再对最终文件计算摘要。

## 使用

从仓库根目录运行。先由导出者提供可信摘要：

```sh
shasum -a 256 /path/to/source351.jsonl
python3 tools/source351_replay.py /path/to/source351.jsonl \
  --manifest-sha256 '<导出者记录的64位摘要>' --dry-run
```

默认就是离线 dry-run：验证整份文件并报告数量，不读凭据、不请求接口、不写 checkpoint。
`--dryrun` 也是别名。dry-run 不证明服务端授权或当前规则仍有效。

联网阶段凭据仅从 `YOUBAN_AI_OPS_TOKEN` 读取，使用 `X-AI-Ops-Token`，
请通过已有受保护的环境注入机制提供；不要把 token 放命令参数、manifest 或日志。
可用 `YOUBAN_AI_OPS_BASE_URL` 配置完整 AI ops 前缀，默认：
`https://api.xiaohuiji.cc/api/youban_publish/ai-ops`。
只允许 HTTPS，不跟随重定向，不输出服务器响应正文或凭据。

以下命令供主 agent 在获得执行授权后使用，本次实现未运行生产命令：

```sh
# 仅预演一批；会在服务端生成15分钟有效的审计计划，但不提交。
python3 tools/source351_replay.py /path/to/source351.jsonl \
  --manifest-sha256 '<摘要>' --preview --batch-size 200 --max-batches 1

# 提交一批，并保存本地 checkpoint。请放在受保护、持久的仓库外目录。
python3 tools/source351_replay.py /path/to/source351.jsonl \
  --manifest-sha256 '<摘要>' --apply \
  --checkpoint /path/to/private/source351.checkpoint.json \
  --batch-size 200 --max-batches 1

# 更多已批准频道：同一 checkpoint 后续必须保持完全相同的频道参数。
# 在上述命令后追加 --chat=-100XXXXXXXXXX:EXPECTED_RULE_ID
```

source replay 异步 apply 正常返回 running。自动串行批量时沿用已有 checkpoint、
manifest SHA 和显式频道参数，追加以下等待参数，例如：

```sh
python3 tools/source351_replay.py /path/to/prepared.jsonl \
  --manifest-sha256 '<已有摘要>' --apply \
  --checkpoint /path/to/existing.checkpoint.json \
  --batch-size 200 --max-batches 14 --wait-seconds 330 --poll-interval 2 \
  --chat=-100XXXXXXXXXX:338
```

`--wait-seconds` 范围0..600，默认0保留立即退出行为；`--poll-interval` 范围1..30，默认2秒。
等待预算按每个计划计算，启动恢复和本次新 apply 均使用相同参数；
只对已知 running 的原 planId 查询 status，每次重新验证 planId/action/candidates。
completed 才允许推进 cursor、进入下一批。running 超时、status HTTP失败或返回不一致时停止，
保留 checkpoint，不再 apply、不生成替代 plan；下一次仍先查询原计划。
pending/failed/unknown 不等待，不自动重放。HTTP请求另受 `--timeout` 限制。
不要为启用等待清除已有 checkpoint；原 cursor（包括已确认的13195166）继续保留。

每次运行沿用相同 manifest、摘要、endpoint、频道列表、checkpoint；重复执行同一命令即可续跑。
同一 checkpoint 使用文件锁阻止并发，临时文件权限0600、原子替换并 fsync。
checkpoint 绑定 manifest SHA、scope、endpoint、允许频道；cursor 是已确认批次的最后原始 ID，
不是 OFFSET，也不是 messageId。保留每批 planId、精确原始 ID、失败 ID 和 queued 数量审计。
不要为同一 manifest 使用多个 checkpoint/多台机器并发提交；本地锁无法协调不同文件或主机。
checkpoint 是受信本地状态文件，不应手工改 cursor 或删除来强行重跑。

## 接口与失败恢复边界

调用的专用路由均位于上述 AI ops 前缀下：

| 接口 | 方法 | 内容 |
|---|---|---|
| `/collector/source/replay/plan` | POST | `sourceId:351, collectorEventIds:[原始ID], limit:本批数量` |
| `/collector/source/replay/apply` | POST | 当前 preview 的 `planId, confirmToken` |
| `/collector/source/replay/status` | GET | `planId` |

只传原始 ID，不使用自动范围查询、模糊时间筛选、eventIds 兼容兜底。
preview 必须返回完整 `collectorEventIds` 和 `replayPreview`，数量、集合、chat/message、
ruleSourceId351、每频道期望规则全部一致才提交。旧部署若缺这些字段直接拒绝，绝不降级绕过校验。
服务端现有逻辑校验 source 启用、租户权益、采集开关、原始来源同租户同 Bot、账号频道 approved
关系及专属规则，并在 apply 再检查。preview 不返回 source 的 tenant/account，
请求中也没有可强制绑定这两个值的参数，因此预期78/615依赖受控导出证据；脚本不能独立证明
数据库 source 当前仍归属78/615。上线执行前须由主 agent 用只读证据重新确认。

apply 前先持久保存 planId；确认令牌只保留内存，不输出或写 checkpoint。
apply HTTP 超时、断连、业务报错、进程中断后，不重新 apply，也不创建替代计划：
下次运行首先查询已保存 planId。status 查询失败保留状态并退出，可以安全重跑查询。
running 可按上述等待参数轮询，到期仍停止；failed、unknown、pending（包括过期）全部停止，等待人工对账；
不得凭“过期”推断没有入队，也不得删除 checkpoint 重放。崩溃发生在保存计划后、发送前，
也会保守停在 pending，需要人工确认，这牺牲自动恢复以避免未知结果重复提交。

只有 `status=completed` 且 failed 为空才推进整批 cursor。
`completed` 即使 HTTP 成功也可能含 failed IDs：脚本保留精确失败子集、停止后续批次，
审核后用相同命令加 `--retry-failed`，为这些 ID 重新 preview/apply；成功子集不再提交。
`failed` 状态可能存在部分副作用且无完整失败列表，不能自动重试。
API 原始消息重放仍依赖现有幂等持久化和队列去重，本脚本不清去重、不更改审批、
不启动消费者、不直接发 TG、不直接写生产 Redis。

**plan completed、queued、cursor 推进或 manifestExhausted 都不代表采集资料、媒体、发布任务
或 Telegram 发送完成。** `applied` 在此是新入队数量，已存在/处理中可能被 skipped；
服务端状态接口没有结构化 skipped 明细。最终业务验收仍要沿原始事件 → 采集事件 →
资料/媒体 → tg_job → 发送确认逐层核对。脚本始终输出 `businessComplete:false`。

本地验证（模拟 API，不访问网络）：

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tools -p 'test_source351_*.py' -v
```
