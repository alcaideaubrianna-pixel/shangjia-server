-- Run before deploying the source-scope query change. Existing writers remain compatible.
-- Backfill and concurrent indexes are separate steps; do not deploy until validation passes.
BEGIN;
SET LOCAL lock_timeout = '3s';
ALTER TABLE hg_youban_publish_note_index
    ADD COLUMN IF NOT EXISTS is_collected boolean NOT NULL DEFAULT false;

-- Preserve the projection even while old/new web and worker versions coexist.
CREATE OR REPLACE FUNCTION ybp_note_index_set_collected() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    SELECT COALESCE(p.source_type = 'youban_collect', false) INTO NEW.is_collected
    FROM hg_content_profile p WHERE p.id = NEW.profile_id;
    NEW.is_collected := COALESCE(NEW.is_collected, false);
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS ybp_note_index_set_collected ON hg_youban_publish_note_index;
CREATE TRIGGER ybp_note_index_set_collected
BEFORE INSERT OR UPDATE OF profile_id, is_collected ON hg_youban_publish_note_index
FOR EACH ROW EXECUTE FUNCTION ybp_note_index_set_collected();

CREATE OR REPLACE FUNCTION ybp_profile_sync_collected() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    UPDATE hg_youban_publish_note_index
    SET is_collected = COALESCE(NEW.source_type = 'youban_collect', false)
    WHERE profile_id = NEW.id
      AND is_collected IS DISTINCT FROM COALESCE(NEW.source_type = 'youban_collect', false);
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS ybp_profile_sync_collected ON hg_content_profile;
CREATE TRIGGER ybp_profile_sync_collected
AFTER UPDATE OF source_type ON hg_content_profile
FOR EACH ROW WHEN (OLD.source_type IS DISTINCT FROM NEW.source_type)
EXECUTE FUNCTION ybp_profile_sync_collected();
COMMIT;
