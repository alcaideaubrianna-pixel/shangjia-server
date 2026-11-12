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

./tools/local-regression/reset-tgfake-db.sh

# 使用 231 作为本地目标频道 ID，复用少量真实结构的资料映射；不导入线上正文或历史任务。
docker exec -i youban-postgres psql -U postgres -d "$db" -v ON_ERROR_STOP=1 <<'SQL'
INSERT INTO hg_youban_publish_channel
    (id,tenant_id,tg_account_id,channel_title,target_chat_id,publish_direction,
     cycle_publish_enabled,cycle_publish_days,cycle_publish_mode,cycle_next_run_at,
     cycle_active_run_id,status,created_at,updated_at)
VALUES
    (231,999,70,'local-channel-231','-100999000231','up',1,3,'time',NOW()-interval '1 minute',0,1,NOW(),NOW())
ON CONFLICT (id) DO UPDATE SET
    tenant_id=EXCLUDED.tenant_id,
    tg_account_id=EXCLUDED.tg_account_id,
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
INSERT INTO hg_youban_publish_channel_profile
    (tenant_id,account_id,channel_id,profile_id,last_job_id,status,next_cycle_at,cycle_generation,created_at,updated_at)
SELECT 999,999,231,p.id,0,'active',NOW()-interval '1 second',0,NOW(),NOW()
FROM hg_youban_publish_profile_channel p
WHERE p.channel_id=231;
SQL

docker compose -f tools/local-regression/docker-compose.yml up -d --no-build \
  tgfake publish-worker background-worker scheduler web
docker compose -f tools/local-regression/docker-compose.yml ps --all
echo "频道 231 轻量回放已启动：DB=$db，publish worker=1，连接池=$YOUBAN_LOCAL_DB_MAX_OPEN"
