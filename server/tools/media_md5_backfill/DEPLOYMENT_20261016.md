# 2026-10-16 MD5 修复上线记录

生产为阿里云 SAE 新加坡；PostgreSQL 仍在腾讯云 `b-xiaohuiji-sg`，仅通过 SSH server2 管理，本机不直连生产 DB。

代码以线上 `30d0784` 为基线，仅追加本次 MD5 修复。2026-10-16 22:05（上海时间）七个 SAE 角色更新至 `8f47cc848c2a62fed604a828ea58369791b72f7a`，镜像标签 `8f47cc8`。GitHub Actions run `38909825903` 构建成功，镜像摘要 `sha256:88b5cf517f00f1d0f0b5cee21ba0e899188a16421cf06cce06009ab502e38438`。

实例核对：web 2、account 1、background 1、collector 1、media 2、publish 2、scheduler 1，镜像均一致，实例均 Healthy/Running。两个 API 域名 `/readyz` 返回上述完整代码 SHA。账号按停止旧实例、修改配置、启动单实例的顺序更新。

新增链路实测：21:57:30 后更新、最新 2000 个采集媒体 ID 范围内，25 条账号采集及 6 条 Bot 采集已 ready，MD5 全部非空；pending/downloading 状态不计为成功。

DB 回填目录 `/var/lib/postgresql/media-md5-backfill-20261016`，服务 `youban-media-md5-backfill-20261016.service` 已开机自启。2 个下载线程，合计限速 4 MiB/s，256 KiB 分块流式哈希，不缓存完整媒体、不落盘媒体文件。MemoryMax=512 MiB、MemorySwapMax=0、LimitFSIZE=256 MiB。

22:05 抽查：pilot 成功 5 条，recent 成功 1480 条、ConnectionError 1 条；旧区段 HTTP 404 3900 条、ReadTimeout 2 条，均仅记录失败清单。累计实际补全 1485 条，最近 100 条计算摘要、媒体表、媒体指纹表全部一致。进程峰值约 35 MiB，整个任务目录约 364 KiB；回填仍在持续，数字会变化。

脚本和断点的变更不会触发重新发布资料。每条数据库写入均使用精确 ID、空 MD5、未删除、ready 和源地址未变条件，并在事务中同步、核对媒体指纹表；冲突回滚。

当前近期区段完成后自动续跑旧区段；失败清单可用 `--retry-failures` 单独重试。每份 state 使用首次运行的 ID 高水位，后续补扫须使用新 state。SHA 和工单校验完成，不表示 130 万条历史数据已经全部修复。

回滚：代码切回 `30d0784`，按同样角色顺序发布；停止回填使用 `sudo systemctl stop youban-media-md5-backfill-20261016`。已补全的 MD5 通常无需回滚；如需撤销，先停任务，再依据 SQLite 审计中的精确 ID/摘要逐条审核，禁止全表置空。
