-- 本地真实角色故障注入：让一份夹具资料的一张图片模拟处理进程中断。
-- 先不启动 media-worker，确认该 job 等待而其他频道推进；再启动 media-worker 验证自动恢复和唤醒。
-- 仅允许操作 tenant/account 999 且 source_type=local_regression 的资料，不伪造成功状态。
BEGIN;
CREATE TEMP TABLE local_media_recovery_case ON COMMIT DROP AS
SELECT j.id AS job_id,j.profile_id,j.channel_id,
       (SELECT min(m.id) FROM hg_youban_publish_media m
        WHERE m.account_id=999 AND m.profile_id=j.profile_id AND m.deleted_at IS NULL) AS media_id
FROM hg_youban_publish_tg_job j
JOIN hg_content_profile p ON p.id=j.profile_id
WHERE j.tenant_id=999 AND j.account_id=999 AND j.status='pending' AND j.dispatch_status='idle'
  AND p.source_type='local_regression' AND p.image_count>0
ORDER BY p.id DESC,j.id LIMIT 1;
DO $$ BEGIN
  IF NOT EXISTS(SELECT 1 FROM local_media_recovery_case WHERE media_id>0) THEN
    RAISE EXCEPTION '没有可注入故障的本地媒体任务';
  END IF;
END $$;
DO $$ BEGIN
  UPDATE hg_youban_publish_tg_job j
  SET operation_no='profile:local:media-recovery:'||j.id,priority=1
  FROM local_media_recovery_case c
  WHERE j.id=c.job_id AND j.status='pending' AND j.dispatch_status='idle';
  IF NOT FOUND THEN
    RAISE EXCEPTION '本地任务已被 Worker 抢占，请重试故障注入';
  END IF;
END $$;
UPDATE hg_youban_publish_media m
SET processing_status='processing',processing_error='',updated_at=LOCALTIMESTAMP-interval '12 minutes'
FROM local_media_recovery_case c WHERE m.id=c.media_id AND m.account_id=999;
UPDATE hg_youban_publish_tg_outbox o
SET status='pending',available_at=LOCALTIMESTAMP,lease_owner='',lease_expires_at=NULL,delivered_at=NULL
FROM local_media_recovery_case c WHERE o.job_id=c.job_id;
SELECT * FROM local_media_recovery_case;
COMMIT;
