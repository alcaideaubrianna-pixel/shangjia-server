-- Explicit admission backfill for ONE authorized tenant/account/channel.
-- psql vars: tenant_id, account_id, channel_id, after_id, upper_id, batch_size.
-- Operator must verify this channel's manual/default authorization for scope.
-- Rows with inactive mappings are deliberately left untouched.
BEGIN;
WITH page AS MATERIALIZED (
  SELECT id,status,visibility,deleted_at FROM hg_content_profile
  WHERE id>:after_id AND id<=:upper_id ORDER BY id
  LIMIT LEAST(GREATEST(:batch_size,1),1000)
)
INSERT INTO hg_youban_publish_channel_profile
  (tenant_id,account_id,channel_id,profile_id,status,batch_order)
SELECT :tenant_id,:account_id,:channel_id,p.id,'active',nextval('hg_ybp_batch_order_seq')
FROM page p
WHERE p.status=1 AND p.visibility='public' AND p.deleted_at IS NULL
  AND EXISTS (SELECT 1 FROM hg_youban_publish_profile_state ps
    WHERE ps.profile_id=p.id AND ps.tenant_id=:tenant_id
      AND ps.account_id=:account_id AND ps.deleted_at IS NULL)
ORDER BY p.id
ON CONFLICT (channel_id,profile_id) DO NOTHING;
COMMIT;
SELECT id AS next_after_id FROM hg_content_profile
WHERE id>:after_id AND id<=:upper_id ORDER BY id
LIMIT LEAST(GREATEST(:batch_size,1),1000);
