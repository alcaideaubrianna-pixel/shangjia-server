#!/usr/bin/env bash
# 只读核验真实角色运行后的任务集合、频道顺序、发送审计和资料投影；不修改任务状态。
set -euo pipefail
local_db_name="${YOUBAN_LOCAL_DB_NAME:-youban_hotgo}"
run_id="${1:?用法：verify-runtime-round.sh run_id profiles channels}"
profiles="${2:?缺少资料数量}"
channels="${3:?缺少频道数量}"
audit_mode="${4:-response}"
if ! [[ "$run_id" =~ ^[a-zA-Z0-9_-]+$ && "$profiles" =~ ^[1-9][0-9]*$ && "$channels" =~ ^[1-9][0-9]*$ ]]; then
  echo 'run_id只允许字母数字下划线连字符；数量必须为正整数' >&2
  exit 1
fi
audit_filter="AND a.source='response'"
if [[ "$audit_mode" == "any" ]]; then
  audit_filter=""
elif [[ "$audit_mode" != "response" ]]; then
  echo '证据模式必须为response或any；响应丢失场景使用any验证Webhook证据' >&2
  exit 1
fi
result="$(docker exec youban-postgres psql -U postgres -d "$local_db_name" -X -v ON_ERROR_STOP=1 -Atc "
WITH jobs AS (
 SELECT j.* FROM hg_youban_publish_tg_job j
 WHERE j.tenant_id=999 AND j.account_id=999
 AND j.operation_no LIKE 'profile_cycle:local:runtime:$run_id:%'
), ordered AS (
 SELECT id,sent_at,lag(sent_at) OVER(PARTITION BY target_chat_id ORDER BY created_at,id) previous_sent
 FROM jobs
), evidence AS (
 SELECT j.id,count(DISTINCT a.message_id) actual,
 greatest(1,s.expected_display_media_count)+s.expected_verify_media_count expected
 FROM jobs j JOIN hg_youban_publish_profile_state s ON s.profile_id=j.profile_id AND s.tenant_id=999 AND s.account_id=999
 LEFT JOIN hg_youban_publish_tg_attempt_message a ON a.job_id=j.id $audit_filter
 GROUP BY j.id,s.expected_display_media_count,s.expected_verify_media_count
), coverage AS (
 SELECT profile_id,count(DISTINCT target_chat_id) count FROM jobs GROUP BY profile_id
)
SELECT json_build_object(
 'run_id','$run_id','audit_mode','$audit_mode','expected_jobs',$profiles*$channels,
 'jobs',(SELECT count(*) FROM jobs),
 'sent',(SELECT count(*) FROM jobs WHERE status='sent'),
 'profiles',(SELECT count(DISTINCT profile_id) FROM jobs),
 'channels',(SELECT count(DISTINCT target_chat_id) FROM jobs),
 'incomplete_profiles',(SELECT count(*) FROM coverage WHERE count<>$channels),
 'order_violations',(SELECT count(*) FROM ordered WHERE sent_at<previous_sent),
 'evidence_mismatches',(SELECT count(*) FROM evidence WHERE actual<>expected),
 'projection_pending',(SELECT count(*) FROM hg_youban_publish_profile_state s WHERE s.tenant_id=999 AND s.account_id=999 AND s.publish_operation_no LIKE 'profile_cycle:local:runtime:$run_id:%' AND COALESCE(s.publish_task_status,'')<>''))")"
printf '%s\n' "$result"
python3 -c '
import json,sys
r=json.loads(sys.argv[1]); profiles=int(sys.argv[2]); channels=int(sys.argv[3])
ok=(r["jobs"]==r["sent"]==r["expected_jobs"] and r["profiles"]==profiles and r["channels"]==channels
    and all(r[k]==0 for k in ["incomplete_profiles","order_violations","evidence_mismatches","projection_pending"]))
sys.exit(0 if ok else 1)
' "$result" "$profiles" "$channels"
