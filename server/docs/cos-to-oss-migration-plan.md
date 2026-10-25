# COS → OSS 多轻量云节点迁移

核对时间：2026-10-25。此文是执行方案，不代表线上迁移已开始。

## 已确认事实

- 当前数据库 COS 地址指向香港桶 `awdaffwaffawf-1480411854`；目标是新加坡 OSS `xiaohuiji-tg-os`。
- 第二个旧桶、每台轻量云的所属地域和剩余流量尚未确认，不应据节点名称直接开跑。
- 腾讯云官方 FAQ：轻量云访问同地域 COS 默认走内网；跨地域走公网。必须在每个节点解析源桶默认域名并验证请求，不能通过 CDN 域名下载后声称 COS 内网免费。
- COS 内网下载流量免费，但 LIST/HEAD/GET 等请求费仍存在；低频/归档对象还可能有取回费用。
- 轻量云上传到 OSS 使用公网，消耗该实例套餐出站流量，不能假设七八台的流量包可合并共享。超过套餐部分另计费。
- 腾讯云节点无法直接使用阿里云 OSS 内网地址，迁移目标使用 `oss-ap-southeast-1.aliyuncs.com` 公网地址。SAE 日常下载继续用 OSS 内网地址。

参考：
- https://cloud.tencent.com/document/product/1207/44569/
- https://cloud.tencent.com/document/product/1207/56847
- https://cloud.tencent.com/document/product/436/53863
- https://rclone.org/s3/
- https://rclone.org/filtering/#hash-filter-deterministically-select-a-subset-of-files

## 选用 rclone

rclone 同时支持 TencentCOS 和 Alibaba OSS。运行在轻量云上，由节点下载源对象再上传目标对象；不使用 OSS 远程拉取 COS 的迁移服务，因此可以明确控制公网发送节点。

配置文件由已有凭据在受保护文件中生成，权限 0600，不提交仓库，不打印。源权限只授予指定桶 LIST/GET；目标授予指定前缀 LIST/HEAD/PUT，不授予删除。

配置示意，凭据占位符须用受保护渠道填入：

```ini
[cos_old]
type = s3
provider = TencentCOS
access_key_id = <COS读取凭据>
secret_access_key = <COS读取凭据>
endpoint = cos.ap-hongkong.myqcloud.com
region = ap-hongkong

[oss_new]
type = s3
provider = Alibaba
access_key_id = <OSS写入凭据>
secret_access_key = <OSS写入凭据>
endpoint = oss-ap-southeast-1.aliyuncs.com
acl = private
```

## 执行步骤

1. 按源桶地域分组节点；每台通过默认桶域名验证内网地址和小对象读取，确认剩余套餐流量、到期时间、带宽及磁盘。对当前香港桶优先用香港轻量云。
2. 列出两个源桶和目标清单，按完整对象 Key 比较。两源桶同 Key 内容不同的必须单独处理，不能让任意先到者覆盖。
3. 先对缺失对象做少量迁移和读取校验，再扩并发。原路径不变，不改业务数据库，不自动重新上架。
4. 采用 `rclone copy`，绝不使用 `sync` 或 `move`。`--ignore-existing` 跳过目标已有对象，但不证明已有对象完整；同 Key 大小不一致需记录异常，不能自动覆盖。
5. 八台节点分别使用 `--hash-filter 0/8` 至 `7/8`。分片总数固定，失败分片在可用节点续跑相同编号，不能直接改变分母。不要同时使用 `--files-from*`，它会使 hash-filter 失效；若按缺失清单执行，应先将清单分成八份，再使用对应 files-from-raw。
6. 两个源桶按核对后的顺序分别运行；存在冲突 Key 的桶不能直接并发合并到同一目标目录。
7. 每节点开始 2 个传输、4 个校验，限速 5MiB/s；根据实际带宽、流量余量调整。100GiB 限额只是单次运行保护线，不代表套餐剩余额度；rclone 超限退出后不能无人值守无限重启。
8. 完成后按清单校验对象数量、大小及可用校验和，抽样读图和视频，记录缺失、冲突、重试。保留 COS 源数据直到验收。

单节点示例（第一分片，默认演练；确认后移除 `--dry-run`）：

```bash
umask 077
rclone copy \
  cos_old:awdaffwaffawf-1480411854 \
  oss_new:xiaohuiji-tg-os \
  --config /etc/youban-migration/rclone.conf \
  --hash-filter 0/8 \
  --ignore-existing --disable copy \
  --transfers 2 --checkers 4 \
  --bwlimit 5M --max-transfer 100G --cutoff-mode soft \
  --retries 3 --low-level-retries 3 \
  --contimeout 10s --timeout 60s \
  --use-json-log --log-level INFO \
  --log-file /var/log/youban-migration-shard-0.jsonl \
  --stats 30s --dry-run
```

现有对象判断不是跨进程原子写保护；同 Key 必须由唯一分片处理，不在业务正写入的临时目录做迁移。必要时先用截止时间固定历史对象清单。

## 本地验证

使用 rclone v1.70.3、128 个临时文件验证八分片：无交集、合并覆盖全部文件；实际 copy 后目标 128 个文件齐全，预先存在的文件内容保持不变。该验证不等于已验证线上凭据、网络路由或套餐抵扣。

2026-10-25 测试现有 SSH 配置：`youban-hk` 拒绝连接、`Tencent-SG` 认证失败、`b-xiaohuiji-应用 2-sg` 超时；未向上述节点部署迁移进程。
