-- Explicit migration; do not execute from an HTTP request or runtime startup.
-- Install before deploying the count reader, then run tools/collect_channel_counts.
BEGIN;
SET LOCAL lock_timeout = '3s';
SET LOCAL statement_timeout = '30s';

CREATE TABLE IF NOT EXISTS hg_youban_publish_collect_channel_count (
 id bigserial PRIMARY KEY,
 tenant_id bigint NOT NULL, account_id bigint NOT NULL, rule_id bigint NOT NULL,
 channel_id varchar(64) NOT NULL, profile_count bigint NOT NULL DEFAULT 0 CHECK(profile_count>=0),
 ready boolean NOT NULL DEFAULT false,
 created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE(tenant_id,account_id,rule_id,channel_id)
);
ALTER TABLE hg_youban_publish_collect_channel_count ADD COLUMN IF NOT EXISTS last_seen_at timestamp;
-- References deduplicate both chat-ID aliases and profiles shared by multiple rules.
CREATE TABLE IF NOT EXISTS hg_youban_publish_collect_channel_count_member (
 id bigserial PRIMARY KEY,
 tenant_id bigint NOT NULL, account_id bigint NOT NULL, rule_id bigint NOT NULL,
 channel_id varchar(64) NOT NULL, profile_id bigint NOT NULL,
 reference_count bigint NOT NULL CHECK(reference_count>0),
 UNIQUE(tenant_id,account_id,rule_id,channel_id,profile_id)
);
DROP INDEX IF EXISTS idx_ybp_collect_channel_count_recent;
CREATE INDEX IF NOT EXISTS idx_ybp_collect_channel_count_seen
 ON hg_youban_publish_collect_channel_count(tenant_id,account_id,rule_id,last_seen_at DESC) WHERE ready AND profile_count>0;
ALTER TABLE hg_youban_publish_collect_channel_profile
 ADD COLUMN IF NOT EXISTS count_projected boolean NOT NULL DEFAULT false,
 ADD COLUMN IF NOT EXISTS count_included boolean NOT NULL DEFAULT false;
CREATE INDEX IF NOT EXISTS idx_ybp_collect_channel_count_pending_scope
 ON hg_youban_publish_collect_channel_profile(tenant_id,account_id,channel_id,rule_id)
 WHERE NOT count_projected;
CREATE INDEX IF NOT EXISTS idx_ybp_collect_channel_profile_profile
 ON hg_youban_publish_collect_channel_profile(profile_id);

CREATE OR REPLACE FUNCTION ybp_collect_count_chat(chat text) RETURNS text
 LANGUAGE sql IMMUTABLE STRICT AS $$ SELECT CASE WHEN chat LIKE '-100%' THEN substring(chat,5) ELSE chat END $$;
CREATE INDEX IF NOT EXISTS idx_ybp_collect_channel_count_pending_order
 ON hg_youban_publish_collect_channel_profile(tenant_id,account_id,ybp_collect_count_chat(channel_id),rule_id,id)
 WHERE NOT count_projected;
DROP INDEX IF EXISTS idx_ybp_collect_channel_count_backfill;
DROP INDEX IF EXISTS idx_ybp_collect_channel_count_unprojected;

CREATE OR REPLACE FUNCTION ybp_collect_count_adjust(t bigint,a bigint,r bigint,c text,p bigint,delta integer)
 RETURNS void LANGUAGE plpgsql AS $$
DECLARE refs bigint;
BEGIN
 INSERT INTO hg_youban_publish_collect_channel_count(tenant_id,account_id,rule_id,channel_id)
 VALUES(t,a,r,c) ON CONFLICT(tenant_id,account_id,rule_id,channel_id) DO NOTHING;
 IF delta=1 THEN
  INSERT INTO hg_youban_publish_collect_channel_count_member
   (tenant_id,account_id,rule_id,channel_id,profile_id,reference_count)
  VALUES(t,a,r,c,p,1)
  ON CONFLICT(tenant_id,account_id,rule_id,channel_id,profile_id)
  DO UPDATE SET reference_count=hg_youban_publish_collect_channel_count_member.reference_count+1
  RETURNING reference_count INTO refs;
  IF refs=1 THEN
   UPDATE hg_youban_publish_collect_channel_count SET profile_count=profile_count+1,updated_at=CURRENT_TIMESTAMP
    WHERE tenant_id=t AND account_id=a AND rule_id=r AND channel_id=c;
  END IF;
 ELSE
  SELECT reference_count INTO refs FROM hg_youban_publish_collect_channel_count_member
   WHERE tenant_id=t AND account_id=a AND rule_id=r AND channel_id=c AND profile_id=p FOR UPDATE;
  IF refs IS NULL THEN RAISE EXCEPTION 'collection count membership missing'; END IF;
  IF refs=1 THEN
   DELETE FROM hg_youban_publish_collect_channel_count_member
    WHERE tenant_id=t AND account_id=a AND rule_id=r AND channel_id=c AND profile_id=p;
   UPDATE hg_youban_publish_collect_channel_count SET profile_count=profile_count-1,updated_at=CURRENT_TIMESTAMP
    WHERE tenant_id=t AND account_id=a AND rule_id=r AND channel_id=c;
  ELSE
   UPDATE hg_youban_publish_collect_channel_count_member SET reference_count=reference_count-1
    WHERE tenant_id=t AND account_id=a AND rule_id=r AND channel_id=c AND profile_id=p;
  END IF;
 END IF;
END $$;

CREATE OR REPLACE FUNCTION ybp_collect_count_before() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE key text;
BEGIN
 -- Lock the channel, including its all-rule count, before changing memberships.
 FOR key IN
  SELECT DISTINCT concat(x.t,':',x.a,':',ybp_collect_count_chat(x.c))
  FROM (SELECT OLD.tenant_id t,OLD.account_id a,OLD.channel_id c WHERE TG_OP<>'INSERT'
        UNION ALL SELECT NEW.tenant_id,NEW.account_id,NEW.channel_id WHERE TG_OP<>'DELETE') x
  ORDER BY 1
 LOOP
  PERFORM pg_advisory_xact_lock(hashtextextended('ybp:collect-count:'||key,0));
 END LOOP;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 NEW.count_projected:=true;
 NEW.count_included:=NEW.deleted_at IS NULL AND NEW.tenant_id>0 AND NEW.account_id>0
  AND NEW.rule_id>0 AND NEW.profile_id>0 AND coalesce(NEW.channel_id,'')<>''
  AND EXISTS(SELECT 1 FROM hg_content_profile p JOIN hg_youban_publish_profile_state ps ON ps.profile_id=p.id
    WHERE p.id=NEW.profile_id AND p.deleted_at IS NULL AND ps.tenant_id=NEW.tenant_id
     AND ps.account_id=NEW.account_id AND ps.deleted_at IS NULL);
 RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION ybp_collect_count_after() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE old_in boolean:=false; new_in boolean:=false; unchanged boolean:=false; scope record; r bigint;
BEGIN
 IF TG_OP<>'INSERT' THEN old_in:=OLD.count_included; END IF;
 IF TG_OP<>'DELETE' THEN new_in:=NEW.count_included; END IF;
 IF TG_OP='UPDATE' THEN
  unchanged:=OLD.tenant_id=NEW.tenant_id AND OLD.account_id=NEW.account_id AND OLD.rule_id=NEW.rule_id
   AND ybp_collect_count_chat(OLD.channel_id)=ybp_collect_count_chat(NEW.channel_id) AND OLD.profile_id=NEW.profile_id;
 END IF;
 IF NOT(unchanged AND old_in=new_in) THEN
  IF old_in THEN
   FOREACH r IN ARRAY ARRAY[OLD.rule_id,0::bigint] LOOP
    PERFORM ybp_collect_count_adjust(OLD.tenant_id,OLD.account_id,r,ybp_collect_count_chat(OLD.channel_id),OLD.profile_id,-1);
   END LOOP;
  END IF;
  IF new_in THEN
   FOREACH r IN ARRAY ARRAY[NEW.rule_id,0::bigint] LOOP
    PERFORM ybp_collect_count_adjust(NEW.tenant_id,NEW.account_id,r,ybp_collect_count_chat(NEW.channel_id),NEW.profile_id,1);
   END LOOP;
  END IF;
 END IF;
 FOR scope IN
  SELECT DISTINCT x.t,x.a,unnest(ARRAY[x.r,0::bigint]) r,ybp_collect_count_chat(x.c) c
  FROM (SELECT OLD.tenant_id t,OLD.account_id a,OLD.rule_id r,OLD.channel_id c WHERE TG_OP<>'INSERT'
        UNION ALL SELECT NEW.tenant_id,NEW.account_id,NEW.rule_id,NEW.channel_id WHERE TG_OP<>'DELETE') x
  WHERE x.t>0 AND x.a>0 AND x.r>0 AND coalesce(x.c,'')<>''
 LOOP
  INSERT INTO hg_youban_publish_collect_channel_count(tenant_id,account_id,rule_id,channel_id)
   VALUES(scope.t,scope.a,scope.r,scope.c) ON CONFLICT(tenant_id,account_id,rule_id,channel_id) DO NOTHING;
  IF new_in AND scope.t=NEW.tenant_id AND scope.a=NEW.account_id AND scope.c=ybp_collect_count_chat(NEW.channel_id)
   AND scope.r IN(0,NEW.rule_id) THEN
   UPDATE hg_youban_publish_collect_channel_count SET last_seen_at=greatest(last_seen_at,NEW.updated_at)
    WHERE tenant_id=scope.t AND account_id=scope.a AND rule_id=scope.r AND channel_id=scope.c
     AND (last_seen_at IS NULL OR last_seen_at<NEW.updated_at);
  END IF;
  -- Bulk backfill checks completion once per scope, not once per relation.
  IF current_setting('youban.collect_count_backfill',true) IS DISTINCT FROM 'on' THEN
  UPDATE hg_youban_publish_collect_channel_count s SET ready=true,updated_at=CURRENT_TIMESTAMP
   WHERE s.tenant_id=scope.t AND s.account_id=scope.a AND s.rule_id=scope.r AND s.channel_id=scope.c AND NOT s.ready
   AND NOT EXISTS(SELECT 1 FROM hg_youban_publish_collect_channel_profile c
    WHERE c.tenant_id=scope.t AND c.account_id=scope.a AND (scope.r=0 OR c.rule_id=scope.r)
     AND c.channel_id IN(scope.c,'-100'||scope.c) AND NOT c.count_projected);
  END IF;
 END LOOP;
 RETURN NULL;
END $$;
CREATE OR REPLACE FUNCTION ybp_collect_count_finish_backfill() RETURNS integer LANGUAGE plpgsql AS $$
DECLARE scope record; changed integer; total integer:=0;
BEGIN
 FOR scope IN SELECT tenant_id,account_id,rule_id,channel_id FROM hg_youban_publish_collect_channel_count
  WHERE NOT ready ORDER BY concat(tenant_id,':',account_id,':',channel_id),rule_id
 LOOP
  PERFORM pg_advisory_xact_lock(hashtextextended('ybp:collect-count:'||concat(scope.tenant_id,':',scope.account_id,':',scope.channel_id),0));
  UPDATE hg_youban_publish_collect_channel_count s SET ready=true,updated_at=CURRENT_TIMESTAMP
   WHERE s.tenant_id=scope.tenant_id AND s.account_id=scope.account_id AND s.rule_id=scope.rule_id AND s.channel_id=scope.channel_id AND NOT s.ready
   AND NOT EXISTS(SELECT 1 FROM hg_youban_publish_collect_channel_profile c
    WHERE c.tenant_id=scope.tenant_id AND c.account_id=scope.account_id AND (scope.rule_id=0 OR c.rule_id=scope.rule_id)
     AND c.channel_id IN(scope.channel_id,'-100'||scope.channel_id) AND NOT c.count_projected);
  GET DIAGNOSTICS changed=ROW_COUNT;total:=total+changed;
 END LOOP;
 RETURN total;
END $$;

DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='hg_youban_publish_collect_channel_profile'::regclass AND tgname='ybp_collect_count_before') THEN
  CREATE TRIGGER ybp_collect_count_before BEFORE INSERT OR UPDATE OR DELETE
   ON hg_youban_publish_collect_channel_profile FOR EACH ROW EXECUTE FUNCTION ybp_collect_count_before();
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='hg_youban_publish_collect_channel_profile'::regclass AND tgname='ybp_collect_count_after') THEN
  CREATE TRIGGER ybp_collect_count_after AFTER INSERT OR UPDATE OR DELETE
   ON hg_youban_publish_collect_channel_profile FOR EACH ROW EXECUTE FUNCTION ybp_collect_count_after();
 END IF;
END $$;

CREATE OR REPLACE FUNCTION ybp_collect_count_profile_changed() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE row_id bigint; profile_ids bigint[];
BEGIN
 IF TG_OP='UPDATE' THEN
  IF TG_TABLE_NAME='hg_content_profile' THEN
   IF OLD.deleted_at IS NOT DISTINCT FROM NEW.deleted_at THEN RETURN NULL; END IF;
  ELSE
   IF OLD.profile_id=NEW.profile_id AND OLD.tenant_id=NEW.tenant_id AND OLD.account_id=NEW.account_id
    AND OLD.deleted_at IS NOT DISTINCT FROM NEW.deleted_at THEN RETURN NULL; END IF;
  END IF;
 END IF;
 IF TG_TABLE_NAME='hg_content_profile' THEN
  IF TG_OP='INSERT' THEN profile_ids:=ARRAY[NEW.id];
  ELSIF TG_OP='DELETE' THEN profile_ids:=ARRAY[OLD.id];
  ELSE profile_ids:=ARRAY[OLD.id,NEW.id]; END IF;
 ELSE
  IF TG_OP='INSERT' THEN profile_ids:=ARRAY[NEW.profile_id];
  ELSIF TG_OP='DELETE' THEN profile_ids:=ARRAY[OLD.profile_id];
  ELSE profile_ids:=ARRAY[OLD.profile_id,NEW.profile_id]; END IF;
 END IF;
 FOR row_id IN SELECT id FROM hg_youban_publish_collect_channel_profile WHERE profile_id=ANY(profile_ids)
  ORDER BY tenant_id,account_id,ybp_collect_count_chat(channel_id),rule_id,id
 LOOP
  UPDATE hg_youban_publish_collect_channel_profile SET count_projected=true WHERE id=row_id;
 END LOOP;
 RETURN NULL;
END $$;
-- Reruns replace function bodies without dropping triggers on busy tables.
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='hg_content_profile'::regclass AND tgname='ybp_collect_count_profile_changed') THEN
  CREATE TRIGGER ybp_collect_count_profile_changed AFTER INSERT OR DELETE OR UPDATE OF deleted_at
   ON hg_content_profile FOR EACH ROW EXECUTE FUNCTION ybp_collect_count_profile_changed();
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='hg_youban_publish_profile_state'::regclass AND tgname='ybp_collect_count_owner_changed') THEN
  CREATE TRIGGER ybp_collect_count_owner_changed AFTER INSERT OR DELETE OR UPDATE OF profile_id,tenant_id,account_id,deleted_at
   ON hg_youban_publish_profile_state FOR EACH ROW EXECUTE FUNCTION ybp_collect_count_profile_changed();
 END IF;
END $$;
-- Reruns preserve source activity timestamps; migration time is not last-seen time.
UPDATE hg_youban_publish_collect_channel_count s SET last_seen_at=f.last_seen_at
FROM (SELECT c.tenant_id,c.account_id,r.rule_id,ybp_collect_count_chat(c.channel_id) channel_id,max(c.updated_at) last_seen_at
 FROM hg_youban_publish_collect_channel_profile c
 CROSS JOIN LATERAL unnest(ARRAY[c.rule_id,0::bigint]) r(rule_id)
 WHERE c.count_included GROUP BY c.tenant_id,c.account_id,r.rule_id,ybp_collect_count_chat(c.channel_id)) f
WHERE s.tenant_id=f.tenant_id AND s.account_id=f.account_id AND s.rule_id=f.rule_id AND s.channel_id=f.channel_id
 AND (s.last_seen_at IS NULL OR s.last_seen_at<f.last_seen_at);
COMMIT;
