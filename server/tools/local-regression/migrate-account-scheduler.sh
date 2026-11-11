#!/usr/bin/env bash
# 按账户切换调度；默认只读。连接参数由PGHOST/PGPORT/PGUSER/PGDATABASE/PGPASSFILE提供，不输出凭据。
set -euo pipefail
tenant="${1:?用法：migrate-account-scheduler.sh tenant_id account_id [legacy|unified_stream] [--apply]}"
account="${2:?缺少account_id}"
engine="${3:-unified_stream}"
mode="${4:---dry-run}"
[[ "$tenant" =~ ^[1-9][0-9]*$ && "$account" =~ ^[1-9][0-9]*$ ]] || { echo '账户和租户必须为正整数' >&2; exit 1; }
[[ "$engine" == legacy || "$engine" == unified_stream ]] || { echo '不支持的引擎' >&2; exit 1; }
[[ "$mode" == --dry-run || "$mode" == --apply ]] || { echo '模式必须为--dry-run或--apply' >&2; exit 1; }
psql -X -v ON_ERROR_STOP=1 -P pager=off -v tenant="$tenant" -v account="$account" <<'SQL'
SELECT id,tenant_id,nickname,status FROM hg_youban_publish_account WHERE tenant_id=:'tenant' AND id=:'account';
SELECT tenant_id,account_id,push_engine,publish_config_json FROM hg_youban_publish_account_setting WHERE tenant_id=:'tenant' AND account_id=:'account' AND deleted_at IS NULL;
SELECT status,dispatch_status,count(*) FROM hg_youban_publish_tg_job WHERE tenant_id=:'tenant' AND account_id=:'account' AND status IN ('pending','sending','failed_retry','unknown') GROUP BY status,dispatch_status;
SELECT count(*) AS overdue_channel_jobs FROM hg_youban_publish_tg_job WHERE tenant_id=:'tenant' AND account_id=:'account' AND cycle_enabled=1 AND next_cycle_at<=NOW() AND status IN ('sent','superseded');
SELECT r.id,r.channel_id,r.status,r.stage FROM hg_youban_publish_cycle_run r JOIN hg_youban_publish_channel c ON c.id=r.channel_id WHERE r.tenant_id=:'tenant' AND c.tg_account_id=:'account' AND r.status='running' ORDER BY r.id LIMIT 20;
SQL
if [[ "$mode" != --apply ]]; then
  echo 'dry-run结束；未修改数据。应用前必须先部署兼容版本、完成索引迁移并保留旧队列消费。'
  exit 0
fi
psql -X -v ON_ERROR_STOP=1 -P pager=off -v tenant="$tenant" -v account="$account" -v engine="$engine" <<'SQL'
BEGIN;
SET LOCAL lock_timeout='3s';
SET LOCAL statement_timeout='10s';
-- 新引擎必须先补齐轻量计划和有限轮转字段，不能在缺少迁移时切换账户。
SELECT :'engine'='legacy' OR (SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND
 ((table_name='hg_youban_publish_channel_profile' AND column_name IN ('next_cycle_at','cycle_generation')) OR
 (table_name='hg_youban_publish_channel' AND column_name='cycle_round_end_id') OR
 (table_name='hg_youban_publish_cycle_run' AND column_name='upper_profile_id')))=4 AS schema_ready \gset
\if :schema_ready
\else
ROLLBACK;
\echo '缺少cycle_plan_runtime.pgsql.sql迁移，拒绝切换。'
\quit 1
\endif
SELECT id,push_engine AS previous_engine FROM hg_youban_publish_account_setting WHERE tenant_id=:'tenant' AND account_id=:'account' AND deleted_at IS NULL FOR UPDATE;
SELECT :'engine'<>'legacy' OR NOT EXISTS(SELECT 1 FROM hg_youban_publish_profile_cycle_run WHERE tenant_id=:'tenant' AND account_id=:'account' AND status='pending') AS rollback_ready \gset
\if :rollback_ready
\else
ROLLBACK;
\echo '仍有待准备的资料循环计划，先停止该账户计划生产并等待收口，拒绝留下无人处理的计划。'
\quit 1
\endif
SELECT count(*)=1 AND NOT EXISTS(SELECT 1 FROM hg_youban_publish_cycle_run r JOIN hg_youban_publish_channel c ON c.id=r.channel_id WHERE r.tenant_id=:'tenant' AND c.tg_account_id=:'account' AND r.status='running') AS valid_scope FROM hg_youban_publish_account_setting s JOIN hg_youban_publish_account a ON a.id=s.account_id AND a.tenant_id=s.tenant_id WHERE s.tenant_id=:'tenant' AND s.account_id=:'account' AND s.deleted_at IS NULL AND a.deleted_at IS NULL AND a.status=1 \gset
\if :valid_scope
UPDATE hg_youban_publish_account_setting SET push_engine=:'engine',updated_at=NOW() WHERE tenant_id=:'tenant' AND account_id=:'account' AND deleted_at IS NULL RETURNING tenant_id,account_id,push_engine;
COMMIT;
\else
ROLLBACK;
\echo '账户或设置不唯一/未启用，或仍有运行中的批次，已回滚。'
\quit 1
\endif
SQL
echo '已修改该账户引擎；等待缓存失效后核验。没有删除、重置或重入任何发送任务。'
