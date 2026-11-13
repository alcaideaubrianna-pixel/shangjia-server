#!/usr/bin/env bash
# 轻量复现频道 231 的循环恢复链路，不启动压力场景和采集媒体 worker。
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT_DIR"
db="${YOUBAN_LOCAL_DB_NAME:-youban_tgfake_test}"
case "$db" in
  railway|youban_hotgo|youban_production|postgres)
    echo "拒绝使用共享数据库执行频道 231 本地回放: $db" >&2
    exit 1
    ;;
esac

export YOUBAN_LOCAL_DB_NAME="$db"
export YOUBAN_LOCAL_DB_MAX_OPEN="${YOUBAN_LOCAL_DB_MAX_OPEN:-4}"
export YOUBAN_LOCAL_DB_MAX_IDLE="${YOUBAN_LOCAL_DB_MAX_IDLE:-2}"
export YOUBAN_LOCAL_PUBLISH_REPLICAS=1
export YOUBAN_LOCAL_SKIP_PROFILE_FINGERPRINT_BACKFILL=1

# 先停止本地角色释放数据库连接，再重建隔离测试库。
docker compose -f tools/local-regression/docker-compose.yml down --remove-orphans >/dev/null 2>&1 || true
./tools/local-regression/reset-tgfake-db.sh

# 使用 231 作为本地目标频道 ID，复用少量真实结构的资料映射；不导入线上正文或历史任务。
docker exec -i youban-postgres psql -U postgres -d "$db" -v ON_ERROR_STOP=1 <<'SQL'
-- 频道 231 回放必须绑定本地账户和真实 Bot 主键。
-- 旧夹具曾把 tg_account_id 写成 70、Job 的 bot_id 留为 0，导致账户分片和 Bot
-- 发送可用性检查把回放任务排除，表现为循环任务没有生成或生成后不发送。
INSERT INTO hg_youban_publish_bot
    (tenant_id,bot_name,bot_username,bot_token,status,created_at,updated_at)
SELECT 999,'local-hook-bot','local_hook_bot','LOCAL:TEST:TOKEN',1,NOW(),NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM hg_youban_publish_bot
    WHERE tenant_id=999 AND bot_username='local_hook_bot'
);
INSERT INTO hg_youban_publish_channel
    (id,tenant_id,tg_account_id,channel_title,target_chat_id,publish_direction,
     cycle_publish_enabled,cycle_publish_days,cycle_publish_mode,cycle_next_run_at,bot_id_json,
     cycle_active_run_id,status,created_at,updated_at)
VALUES
    (231,999,999,'local-channel-231','-100999000231','up',1,3,'time',NOW()-interval '1 minute',
     (SELECT json_build_array(id)::text FROM hg_youban_publish_bot
      WHERE tenant_id=999 AND bot_username='local_hook_bot' ORDER BY id LIMIT 1),
     0,1,NOW(),NOW())
ON CONFLICT (id) DO UPDATE SET
    tenant_id=EXCLUDED.tenant_id,
    tg_account_id=999,
    bot_id_json=EXCLUDED.bot_id_json,
    cycle_publish_enabled=1,
    cycle_publish_mode='time',
    cycle_publish_days=3,
    cycle_next_run_at=NOW()-interval '1 minute',
    cycle_active_run_id=0,
    status=1,
    updated_at=NOW();

DELETE FROM hg_youban_publish_profile_channel WHERE tenant_id=999 AND account_id=999 AND channel_id=231;
INSERT INTO hg_youban_publish_profile_channel
    (tenant_id,account_id,profile_id,channel_id,is_manual,created_at,updated_at)
SELECT 999,999,p.id,231,0,NOW(),NOW()
FROM hg_content_profile p
WHERE p.source_type='local_regression'
ORDER BY p.id
LIMIT 20;

DELETE FROM hg_youban_publish_channel_profile WHERE channel_id=231;
INSERT INTO hg_youban_publish_tg_job
    (tenant_id,account_id,profile_id,channel_id,target_chat_id,bot_id,operation_no,status,
     priority,dispatch_status,cycle_enabled,cycle_days,push_mode,sent_at,created_at,updated_at)
SELECT 999,999,p.profile_id,231,'-100999000231',
       (SELECT id FROM hg_youban_publish_bot
        WHERE tenant_id=999 AND bot_username='local_hook_bot' ORDER BY id LIMIT 1),
       'cycle_seed:231:'||p.profile_id,'sent',90,'done',1,3,'cycle',
       NOW()-interval '4 days',NOW()-interval '4 days',NOW()
FROM hg_youban_publish_profile_channel p
WHERE p.channel_id=231
  AND NOT EXISTS (SELECT 1 FROM hg_youban_publish_tg_job j WHERE j.channel_id=231 AND j.profile_id=p.profile_id AND j.operation_no='cycle_seed:231:'||p.profile_id);
INSERT INTO hg_youban_publish_channel_profile
    (tenant_id,account_id,channel_id,profile_id,last_job_id,status,next_cycle_at,cycle_generation,created_at,updated_at)
SELECT 999,999,231,p.profile_id,j.id,'active',NOW()-interval '1 second',0,NOW(),NOW()
FROM hg_youban_publish_profile_channel p
JOIN hg_youban_publish_tg_job j ON j.channel_id=231 AND j.profile_id=p.profile_id AND j.operation_no='cycle_seed:231:'||p.profile_id
WHERE p.channel_id=231;
SQL

docker compose -f tools/local-regression/docker-compose.yml up -d --no-build \
  tgfake publish-worker background-worker scheduler web
docker compose -f tools/local-regression/docker-compose.yml ps --all
echo "频道 231 轻量回放已启动：DB=${db}，publish worker=1，连接池=${YOUBAN_LOCAL_DB_MAX_OPEN}"
