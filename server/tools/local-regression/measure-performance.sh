#!/usr/bin/env bash
# 只读采样本地真实 Worker；本脚本不创建任务、不修改队列、不连接线上。
set -euo pipefail

duration="${1:-120}"
interval="${2:-10}"
if ! [[ "$duration" =~ ^[1-9][0-9]*$ && "$interval" =~ ^[1-9][0-9]*$ ]]; then
  echo '参数必须为正整数：measure-performance.sh [持续秒数] [采样间隔秒数]' >&2
  exit 1
fi

output="${YOUBAN_LOCAL_PERF_OUTPUT:-/tmp/youban-performance-$(date +%Y%m%d-%H%M%S)}"
mkdir -p "$output"
sql() {
  docker exec youban-postgres psql -U postgres -d youban_hotgo -X -v ON_ERROR_STOP=1 -Atc "$1"
}
started="$(sql 'SELECT clock_timestamp()::text')"
printf '%s\n' "$started" > "$output/started-at.txt"

# 资料发送次数按 job 统计，媒体组的一组消息只算一次资料推送。
sample() {
  sql "SELECT json_build_object('at',clock_timestamp(),'states',(
    SELECT json_agg(s) FROM (
      SELECT status,dispatch_status,count(*) AS count
      FROM hg_youban_publish_tg_job
      WHERE account_id=999 AND operation_no LIKE '%:local:%'
      GROUP BY status,dispatch_status
    ) s),'db_connections',(
      SELECT count(*) FROM pg_stat_activity WHERE datname=current_database()
    ),'advisory_waiters',(
      SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event='advisory'
    ))" >> "$output/samples.jsonl"
  docker stats --no-stream --format '{{json .}}' >> "$output/resources.jsonl"
}

elapsed=0
while (( elapsed < duration )); do
  sample
  remaining=$((duration - elapsed))
  delay="$interval"
  if (( delay > remaining )); then delay="$remaining"; fi
  sleep "$delay"
  elapsed=$((elapsed + delay))
done
sample

# 只统计采样窗口中新完成的 job，排除上轮遗留成功数据。
sql "WITH completed AS (
  SELECT channel_id,sent_at,created_at,
    extract(epoch FROM sent_at-lag(sent_at) OVER(PARTITION BY channel_id ORDER BY sent_at,id)) AS gap
  FROM hg_youban_publish_tg_job
  WHERE account_id=999 AND operation_no LIKE '%:local:%' AND status='sent'
    AND sent_at >= '$started'::timestamptz AT TIME ZONE 'Asia/Shanghai'
), per_channel AS (
  SELECT channel_id,count(*) AS count FROM completed GROUP BY channel_id
) SELECT json_build_object(
  'started_at','$started','ended_at',clock_timestamp(),
  'completed',count(*),'channels',count(DISTINCT channel_id),
  'total_per_minute',count(*)*60.0 / greatest(1,extract(epoch FROM clock_timestamp()-'$started'::timestamptz)),
  'channel_gap_p50_seconds',percentile_cont(0.50) WITHIN GROUP(ORDER BY gap),
  'channel_gap_p95_seconds',percentile_cont(0.95) WITHIN GROUP(ORDER BY gap),
  'channel_gap_p99_seconds',percentile_cont(0.99) WITHIN GROUP(ORDER BY gap),
  'created_to_sent_p95_seconds',percentile_cont(0.95) WITHIN GROUP(ORDER BY extract(epoch FROM sent_at-created_at)),
  'per_channel',(SELECT json_agg(per_channel) FROM per_channel)
) FROM completed" > "$output/result.json"

cat "$output/result.json"
printf '采样记录：%s\n' "$output"
