#!/usr/bin/env bash
# 重建独立 tgfake 回归库；只操作本地 PostgreSQL/Redis，不触碰线上数据。
set -euo pipefail

db="${YOUBAN_LOCAL_DB_NAME:-youban_tgfake_test}"
source_db="${YOUBAN_SOURCE_DB_NAME:-youban_hotgo}"
pg_host="${YOUBAN_DB_HOST:-127.0.0.1}"
pg_user="${YOUBAN_DB_USER:-postgres}"
pg_password="${YOUBAN_DB_PASSWORD:-postgres}"
export PGPASSWORD="$pg_password"

# macOS 的 mktemp 要求 XXXXXX 位于模板末尾，使用 -t 兼容 macOS/Linux。
schema_dump="$(mktemp -t tgfake-schema)"
data_dump="$(mktemp -t tgfake-config)"
trap 'rm -f "$schema_dump" "$data_dump"' EXIT

pg_dump -h "$pg_host" -U "$pg_user" -d "$source_db" --schema-only > "$schema_dump"
sed -i.bak '/transaction_timeout/d' "$schema_dump"
psql -h "$pg_host" -U "$pg_user" -d postgres -v ON_ERROR_STOP=1 \
  -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='$db' AND pid<>pg_backend_pid()" >/dev/null || true
dropdb -h "$pg_host" -U "$pg_user" --if-exists "$db"
createdb -h "$pg_host" -U "$pg_user" "$db"
# 业务表使用 timestamp without time zone；测试库必须和 HotGo 应用统一使用
# Asia/Shanghai，否则 Go 写入时间与 PostgreSQL now() 会相差 8 小时。
psql -h "$pg_host" -U "$pg_user" -d postgres -v ON_ERROR_STOP=1 \
  -c "ALTER DATABASE \"$db\" SET timezone TO 'Asia/Shanghai'" >/dev/null
psql -h "$pg_host" -U "$pg_user" -d "$db" -v ON_ERROR_STOP=1 -f "$schema_dump" >/dev/null
# 线上独立迁移不一定存在于历史 schema dump；本地回放必须补齐资料来源表，
# 否则指纹去重查询会因关系不存在而阻塞整个采集链路。
psql -h "$pg_host" -U "$pg_user" -d "$db" -v ON_ERROR_STOP=1 \
  -f addons/youban_publish/resource/sql/migrations/profile_origin.pgsql.sql >/dev/null

# Worker 启动需要管理员和插件配置；只复制配置表，不复制历史任务。
pg_dump -h "$pg_host" -U "$pg_user" -d "$source_db" --data-only --disable-triggers \
  -t hg_admin_dept -t hg_admin_member -t hg_admin_role -t hg_admin_menu -t hg_admin_role_menu \
  -t hg_admin_member_role -t hg_admin_post -t hg_admin_member_post \
  -t hg_sys_config -t hg_sys_addons_config -t hg_sys_addons_install > "$data_dump"
sed -i.bak '/transaction_timeout/d' "$data_dump"
psql -h "$pg_host" -U "$pg_user" -d "$db" -v ON_ERROR_STOP=1 \
  -c 'TRUNCATE hg_admin_member_role,hg_admin_member_post,hg_admin_post,hg_admin_member,hg_admin_role_menu,hg_admin_menu,hg_admin_role,hg_admin_dept,hg_sys_config,hg_sys_addons_config,hg_sys_addons_install CASCADE' >/dev/null
psql -h "$pg_host" -U "$pg_user" -d "$db" -v ON_ERROR_STOP=1 -f "$data_dump" >/dev/null

psql -h "$pg_host" -U "$pg_user" -d "$db" -v ON_ERROR_STOP=1 \
  -f tools/local-regression/seed_publish_dependencies.sql >/dev/null
psql -h "$pg_host" -U "$pg_user" -d "$db" -v ON_ERROR_STOP=1 \
  -f tools/local-regression/seed_1000_publish.sql >/dev/null

# seed_1000_publish.sql 同时包含旧吞吐压测任务；完整角色场景需要从空队列开始。
psql -h "$pg_host" -U "$pg_user" -d "$db" -v ON_ERROR_STOP=1 <<'SQL' >/dev/null
TRUNCATE hg_youban_publish_tg_attempt_message,hg_youban_publish_tg_delivery_step,
  hg_youban_publish_tg_job_log,hg_youban_publish_tg_attempt,
  hg_youban_publish_tg_message,hg_youban_publish_tg_outbox,
  hg_youban_publish_tg_job CASCADE;
-- 压力脚本会生成 1000×100 条历史任务；真实角色场景按需注入任务，
-- 基础环境只保留少量资料映射，避免账户调度先扫描十万条历史记录。
DELETE FROM hg_youban_publish_profile_channel WHERE account_id=999;
INSERT INTO hg_youban_publish_profile_channel
    (tenant_id,account_id,profile_id,channel_id,is_manual,created_at,updated_at)
SELECT 999,999,p.id,c.id,0,NOW(),NOW()
FROM hg_content_profile p
JOIN hg_youban_publish_channel c ON c.tenant_id=999 AND c.channel_title LIKE 'local-hook-channel-%'
WHERE p.source_type='local_regression' AND p.id<=20;
UPDATE hg_youban_publish_profile_state
SET publish_operation_no='',publish_task_status='',publish_task_updated_at=NULL,
    publish_at=NULL
WHERE tenant_id=999 AND account_id=999;
-- 任务表已清空后，循环状态仍必须有频道资料映射；否则测试会保留已删除的 last_job_id，循环扫描永远找不到队首。
INSERT INTO hg_youban_publish_channel_profile
    (tenant_id,account_id,channel_id,profile_id,last_job_id,status,next_cycle_at,cycle_generation,created_at,updated_at)
SELECT pc.tenant_id,pc.account_id,pc.channel_id,pc.profile_id,0,'active',NOW()-interval '1 second',0,NOW(),NOW()
FROM hg_youban_publish_profile_channel pc
WHERE pc.tenant_id=999 AND pc.account_id=999
ON CONFLICT (channel_id,profile_id) DO UPDATE SET
    account_id=EXCLUDED.account_id,
    last_job_id=0,
    status='active',
    next_cycle_at=EXCLUDED.next_cycle_at,
    updated_at=NOW();
SQL

# 专用 Redis 只存本地回归 Stream/Asynq 数据。
if docker ps --format '{{.Names}}' | rg -q '^youban-runtime-clean-redis$'; then
  docker exec youban-runtime-clean-redis redis-cli -n 15 FLUSHDB >/dev/null
  redis_keys="$(docker exec youban-runtime-clean-redis redis-cli -n 15 DBSIZE | tr -d '\r')"
  if [[ "$redis_keys" != "0" ]]; then
    echo "本地回归 Redis 清理失败：DB15 仍有 ${redis_keys} 个 key" >&2
    exit 1
  fi
fi

echo "tgfake 回归库已重建：$db"
