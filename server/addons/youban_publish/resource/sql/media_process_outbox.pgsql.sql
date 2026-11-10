-- Explicit migration, before deploying producers/media workers. No runtime DDL.
BEGIN;
SET LOCAL lock_timeout = '3s';
SET LOCAL statement_timeout = '30s';
CREATE TABLE IF NOT EXISTS hg_youban_publish_media_process_outbox (
 media_id bigint PRIMARY KEY,
 generation bigint NOT NULL DEFAULT 1,
 status varchar(16) NOT NULL DEFAULT 'pending',
 available_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 lease_token varchar(64) NOT NULL DEFAULT '',
 task_id varchar(128) NOT NULL DEFAULT '',
 attempt_count integer NOT NULL DEFAULT 0,
 bootstrap boolean NOT NULL DEFAULT false,
 last_error text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX IF NOT EXISTS idx_ybp_media_process_outbox_due
 ON hg_youban_publish_media_process_outbox(available_at,media_id)
 WHERE status IN ('pending','leased');

CREATE OR REPLACE FUNCTION ybp_request_media_process(p_media_id bigint,p_delay_ms bigint DEFAULT 0)
RETURNS void LANGUAGE plpgsql AS $$
BEGIN
 IF p_media_id<=0 THEN RETURN; END IF;
 INSERT INTO hg_youban_publish_media_process_outbox AS o(media_id,available_at)
 VALUES(p_media_id,clock_timestamp()+greatest(p_delay_ms,0)*interval '1 millisecond')
 ON CONFLICT(media_id) DO UPDATE SET
 generation=CASE WHEN (o.status IN ('sent','blocked') OR (o.bootstrap AND o.status='leased')) THEN o.generation+1 ELSE o.generation END,
 status=CASE WHEN (o.status IN ('sent','blocked') OR (o.bootstrap AND o.status='leased')) THEN 'pending' ELSE o.status END,
 available_at=CASE WHEN (o.status IN ('sent','blocked') OR (o.bootstrap AND o.status='leased')) THEN EXCLUDED.available_at
                   WHEN o.status='pending' THEN least(o.available_at,EXCLUDED.available_at) ELSE o.available_at END,
 attempt_count=CASE WHEN (o.status IN ('sent','blocked') OR (o.bootstrap AND o.status='leased')) THEN 0 ELSE o.attempt_count END,
 bootstrap=false,
 task_id=CASE WHEN (o.status IN ('sent','blocked') OR (o.bootstrap AND o.status='leased')) THEN '' ELSE o.task_id END,
 last_error=CASE WHEN (o.status IN ('sent','blocked') OR (o.bootstrap AND o.status='leased')) THEN '' ELSE o.last_error END,
 updated_at=clock_timestamp();
END $$;

CREATE OR REPLACE FUNCTION ybp_media_process_outbox_on_write()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.deleted_at IS NULL AND NEW.processing_status='uploaded' THEN
  IF TG_OP='INSERT' THEN
   PERFORM ybp_request_media_process(NEW.id,0);
  ELSIF OLD.processing_status IS DISTINCT FROM NEW.processing_status
     OR OLD.storage_path IS DISTINCT FROM NEW.storage_path
     OR OLD.file_url IS DISTINCT FROM NEW.file_url
     OR OLD.attachment_id IS DISTINCT FROM NEW.attachment_id THEN
   PERFORM ybp_request_media_process(NEW.id,0);
  END IF;
 END IF;
 RETURN NEW;
END $$;
-- The migration is atomic: concurrent media writes see either the old or new trigger.
DROP TRIGGER IF EXISTS ybp_media_process_outbox_write ON hg_youban_publish_media;
CREATE TRIGGER ybp_media_process_outbox_write
 AFTER INSERT OR UPDATE OF processing_status,storage_path,file_url,attachment_id
 ON hg_youban_publish_media FOR EACH ROW EXECUTE FUNCTION ybp_media_process_outbox_on_write();
COMMIT;
