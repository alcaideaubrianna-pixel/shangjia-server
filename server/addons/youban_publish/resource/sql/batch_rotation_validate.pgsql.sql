-- psql ON_ERROR_STOP=1; fail deployment if index missing/invalid/not ready.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid
    WHERE c.oid=to_regclass('idx_ybp_batch_rotation')
      AND i.indrelid='hg_youban_publish_channel_profile'::regclass
      AND i.indisvalid AND i.indisready
  ) THEN
    RAISE EXCEPTION 'idx_ybp_batch_rotation missing, invalid or not ready';
  END IF;
END $$;
SELECT i.indisvalid,i.indisready,pg_get_indexdef(i.indexrelid)
FROM pg_index i WHERE i.indexrelid=to_regclass('idx_ybp_batch_rotation');
