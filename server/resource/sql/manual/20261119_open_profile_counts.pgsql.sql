-- Run with ON_ERROR_STOP. Install before backfill. Readers must stay on the
-- source query until the bounded backfill and source/result verification finish.
BEGIN;
SET LOCAL lock_timeout = '2s';

CREATE TABLE public.hg_open_profile_count_state (
    singleton boolean PRIMARY KEY CHECK (singleton),
    ready boolean NOT NULL
);
INSERT INTO public.hg_open_profile_count_state VALUES (true, false);

CREATE TABLE public.hg_open_profile_candidate (
    profile_id bigint PRIMARY KEY,
    tenant_id bigint NOT NULL,
    account_id bigint NOT NULL,
    province text NOT NULL,
    city text NOT NULL,
    source_created_at timestamp,
    source_note_id bigint
);

CREATE INDEX idx_open_profile_candidate_latest
ON public.hg_open_profile_candidate
    (tenant_id, source_created_at DESC, source_note_id DESC, profile_id DESC);

CREATE INDEX idx_open_profile_candidate_province_latest
ON public.hg_open_profile_candidate
    (tenant_id, province, source_created_at DESC, source_note_id DESC, profile_id DESC);

CREATE INDEX idx_open_profile_candidate_area_latest
ON public.hg_open_profile_candidate
    (tenant_id, province, city, source_created_at DESC, source_note_id DESC, profile_id DESC);

-- Account status is joined at read time. Account toggles do not rewrite profiles.
-- Shards keep different profile writers from all locking the same counter row.
CREATE TABLE public.hg_open_profile_count (
    tenant_id bigint NOT NULL,
    account_id bigint NOT NULL,
    level smallint NOT NULL,
    province text NOT NULL,
    city text NOT NULL,
    shard smallint NOT NULL,
    total bigint NOT NULL CHECK (total >= 0),
    PRIMARY KEY (tenant_id, level, province, city, account_id, shard)
);

CREATE FUNCTION public.open_profile_refresh(p_id bigint) RETURNS void
LANGUAGE plpgsql VOLATILE SET search_path = pg_catalog, public AS $$
DECLARE
    before_row public.hg_open_profile_candidate%ROWTYPE;
    after_row public.hg_open_profile_candidate%ROWTYPE;
    delta record;
BEGIN
    IF current_setting('transaction_isolation') <> 'read committed' THEN
        RAISE EXCEPTION 'open_profile_refresh requires READ COMMITTED';
    END IF;

    -- Lock the profile independently of whether it currently has a candidate.
    PERFORM pg_advisory_xact_lock(hashtextextended('open_profile:' || p_id, 0));
    SELECT * INTO before_row FROM public.hg_open_profile_candidate WHERE profile_id = p_id;
    SELECT p.id, s.tenant_id, s.account_id, COALESCE(p.province, ''), COALESCE(p.city, ''),
           p.source_created_at, p.source_note_id
    INTO after_row
    FROM public.hg_content_profile p
    JOIN public.hg_youban_publish_profile_state s ON s.profile_id = p.id
    WHERE p.id = p_id AND p.deleted_at IS NULL AND p.status = 1 AND p.visibility = 'public'
      AND s.deleted_at IS NULL
      AND (
          EXISTS (SELECT 1 FROM public.hg_content_media m
                  WHERE m.profile_id = p.id AND m.status = 1 AND m.media_type = 'image'
                    AND COALESCE(m.display_storage_path, '') <> '')
          OR EXISTS (SELECT 1 FROM public.hg_youban_publish_media m
                     WHERE m.profile_id = p.id AND m.status = 1 AND m.media_type = 'image'
                       AND m.deleted_at IS NULL
                       AND COALESCE(NULLIF(m.edited_storage_path, ''), NULLIF(m.storage_path, ''),
                                    NULLIF(m.edited_file_url, ''), NULLIF(m.file_url, '')) <> '')
      );

    IF before_row IS NOT DISTINCT FROM after_row THEN
        RETURN;
    END IF;

    IF after_row.profile_id IS NULL THEN
        DELETE FROM public.hg_open_profile_candidate WHERE profile_id = p_id;
    ELSE
        INSERT INTO public.hg_open_profile_candidate VALUES (after_row.*)
        ON CONFLICT (profile_id) DO UPDATE SET
            tenant_id = EXCLUDED.tenant_id, account_id = EXCLUDED.account_id,
            province = EXCLUDED.province, city = EXCLUDED.city,
            source_created_at = EXCLUDED.source_created_at, source_note_id = EXCLUDED.source_note_id;
    END IF;

    FOR delta IN
        SELECT v.tenant_id, v.account_id, g.level,
               CASE WHEN g.level >= 1 THEN v.province ELSE '' END AS province,
               CASE WHEN g.level = 2 THEN v.city ELSE '' END AS city,
               sum(v.change) AS change
        FROM (VALUES
            (before_row.profile_id, before_row.tenant_id, before_row.account_id, before_row.province, before_row.city, -1),
            (after_row.profile_id, after_row.tenant_id, after_row.account_id, after_row.province, after_row.city, 1)
        ) AS v(profile_id, tenant_id, account_id, province, city, change)
        CROSS JOIN generate_series(0, 2) AS g(level)
        WHERE v.profile_id IS NOT NULL
        GROUP BY 1, 2, 3, 4, 5
        HAVING sum(v.change) <> 0
        ORDER BY 1, 3, 4, 5, 2
    LOOP
        IF delta.change < 0 THEN
            UPDATE public.hg_open_profile_count
            SET total = total + delta.change
            WHERE tenant_id = delta.tenant_id AND account_id = delta.account_id
              AND level = delta.level AND province = delta.province AND city = delta.city
              AND shard = mod(p_id, 64);
            IF NOT FOUND THEN
                RAISE EXCEPTION 'missing open profile counter for profile %', p_id;
            END IF;
        ELSE
            INSERT INTO public.hg_open_profile_count
            VALUES (delta.tenant_id, delta.account_id, delta.level, delta.province,
                    delta.city, mod(p_id, 64), delta.change)
            ON CONFLICT (tenant_id, level, province, city, account_id, shard)
            DO UPDATE SET total = hg_open_profile_count.total + EXCLUDED.total;
        END IF;
    END LOOP;
END;
$$;

CREATE FUNCTION public.open_profile_source_changed() RETURNS trigger
LANGUAGE plpgsql VOLATILE SET search_path = pg_catalog, public AS $$
DECLARE old_id bigint; new_id bigint; affected_id bigint;
BEGIN
    IF TG_OP <> 'INSERT' THEN
        IF TG_TABLE_NAME = 'hg_content_profile' THEN old_id := OLD.id;
        ELSE old_id := OLD.profile_id; END IF;
    END IF;
    IF TG_OP <> 'DELETE' THEN
        IF TG_TABLE_NAME = 'hg_content_profile' THEN new_id := NEW.id;
        ELSE new_id := NEW.profile_id; END IF;
    END IF;
    FOR affected_id IN SELECT DISTINCT id FROM unnest(ARRAY[old_id, new_id]) ids(id)
                       WHERE id > 0 ORDER BY id
    LOOP
        PERFORM public.open_profile_refresh(affected_id);
    END LOOP;
    RETURN NULL;
END;
$$;

CREATE TRIGGER open_profile_count_source AFTER INSERT OR DELETE OR
UPDATE OF id, status, visibility, deleted_at, province, city, source_created_at, source_note_id
ON public.hg_content_profile FOR EACH ROW EXECUTE FUNCTION public.open_profile_source_changed();

CREATE TRIGGER open_profile_count_source AFTER INSERT OR DELETE OR
UPDATE OF profile_id, tenant_id, account_id, deleted_at
ON public.hg_youban_publish_profile_state FOR EACH ROW EXECUTE FUNCTION public.open_profile_source_changed();

CREATE TRIGGER open_profile_count_source AFTER INSERT OR DELETE OR
UPDATE OF profile_id, status, media_type, display_storage_path
ON public.hg_content_media FOR EACH ROW EXECUTE FUNCTION public.open_profile_source_changed();

CREATE TRIGGER open_profile_count_source AFTER INSERT OR DELETE OR
UPDATE OF profile_id, status, deleted_at, media_type, edited_storage_path, storage_path, edited_file_url, file_url
ON public.hg_youban_publish_media FOR EACH ROW EXECUTE FUNCTION public.open_profile_source_changed();

CREATE FUNCTION public.open_profile_backfill(after_id bigint, batch_size integer)
RETURNS bigint LANGUAGE plpgsql VOLATILE SET search_path = pg_catalog, public AS $$
DECLARE p_id bigint; last_id bigint;
BEGIN
    IF after_id IS NULL OR after_id < 0 OR batch_size IS NULL OR batch_size NOT BETWEEN 1 AND 1000 THEN
        RAISE EXCEPTION 'require after_id >= 0 and batch_size between 1 and 1000';
    END IF;
    FOR p_id IN SELECT id FROM public.hg_content_profile
                WHERE id > after_id ORDER BY id LIMIT batch_size
    LOOP
        PERFORM public.open_profile_refresh(p_id);
        last_id := p_id;
    END LOOP;
    RETURN last_id;
END;
$$;
-- NULL province/city means no filter. A city-only filter uses the original
-- query because city counts are keyed by both province and city.
CREATE FUNCTION public.open_profile_exact_count(
    target_tenant bigint, require_enabled boolean, target_province text,
    target_city text, excluded_profile_ids bigint[]
) RETURNS bigint
LANGUAGE sql STABLE SET search_path = pg_catalog, public AS $$
    SELECT CASE WHEN target_province IS NULL AND target_city IS NOT NULL THEN NULL
    ELSE
        (SELECT COALESCE(sum(c.total), 0)::bigint
         FROM public.hg_open_profile_count c
         WHERE c.tenant_id = target_tenant
           AND c.level = CASE WHEN target_city IS NOT NULL THEN 2
                              WHEN target_province IS NOT NULL THEN 1 ELSE 0 END
           AND c.province = COALESCE(target_province, '')
           AND c.city = COALESCE(target_city, '')
           AND (NOT require_enabled OR EXISTS (
                SELECT 1 FROM public.hg_youban_publish_account a
                WHERE a.id = c.account_id AND a.tenant_id = c.tenant_id
                  AND a.status = 1 AND a.deleted_at IS NULL
           )))
        -
        (SELECT count(*)
         FROM public.hg_open_profile_candidate p
         WHERE p.profile_id = ANY(excluded_profile_ids)
           AND p.tenant_id = target_tenant
           AND (target_province IS NULL OR p.province = target_province)
           AND (target_city IS NULL OR p.city = target_city)
           AND (NOT require_enabled OR EXISTS (
                SELECT 1 FROM public.hg_youban_publish_account a
                WHERE a.id = p.account_id AND a.tenant_id = p.tenant_id
                  AND a.status = 1 AND a.deleted_at IS NULL
           )))
    END;
$$;
-- Run explicitly after the bounded backfill finishes. Compare the full source,
-- projection and counters in one snapshot before permitting application reads.
CREATE FUNCTION public.open_profile_verify_counts() RETURNS boolean
LANGUAGE plpgsql VOLATILE SET search_path = pg_catalog, public AS $$
DECLARE matches boolean;
BEGIN
    WITH source_rows AS MATERIALIZED (
        SELECT p.id AS profile_id, s.tenant_id, s.account_id,
               COALESCE(p.province, '')::text AS province, COALESCE(p.city, '')::text AS city,
               p.source_created_at, p.source_note_id
        FROM public.hg_content_profile p
        JOIN public.hg_youban_publish_profile_state s ON s.profile_id = p.id
        WHERE p.deleted_at IS NULL AND p.status = 1 AND p.visibility = 'public'
          AND s.deleted_at IS NULL
          AND (
              EXISTS (SELECT 1 FROM public.hg_content_media m
                      WHERE m.profile_id = p.id AND m.status = 1 AND m.media_type = 'image'
                        AND COALESCE(m.display_storage_path, '') <> '')
              OR EXISTS (SELECT 1 FROM public.hg_youban_publish_media m
                         WHERE m.profile_id = p.id AND m.status = 1 AND m.media_type = 'image'
                           AND m.deleted_at IS NULL
                           AND COALESCE(NULLIF(m.edited_storage_path, ''), NULLIF(m.storage_path, ''),
                                        NULLIF(m.edited_file_url, ''), NULLIF(m.file_url, '')) <> '')
          )
    ), expected_counts AS (
        SELECT p.tenant_id, p.account_id, g.level,
               CASE WHEN g.level >= 1 THEN p.province ELSE '' END AS province,
               CASE WHEN g.level = 2 THEN p.city ELSE '' END AS city,
               mod(p.profile_id, 64) AS shard, count(*) AS total
        FROM source_rows p CROSS JOIN generate_series(0, 2) AS g(level)
        GROUP BY 1, 2, 3, 4, 5, 6
    )
    SELECT NOT EXISTS (
        (SELECT * FROM source_rows EXCEPT ALL SELECT * FROM public.hg_open_profile_candidate)
        UNION ALL
        (SELECT * FROM public.hg_open_profile_candidate EXCEPT ALL SELECT * FROM source_rows)
    ) AND NOT EXISTS (
        SELECT 1 FROM expected_counts e
        FULL JOIN public.hg_open_profile_count c
            USING (tenant_id, account_id, level, province, city, shard)
        WHERE COALESCE(e.total, 0) <> COALESCE(c.total, 0)
    ) INTO matches;

    UPDATE public.hg_open_profile_count_state SET ready = matches WHERE singleton;
    RETURN matches;
END;
$$;

-- Call open_profile_backfill in short autocommit batches until it returns NULL,
-- then run open_profile_verify_counts() off peak. A successful verification
-- enables reads when content.openProfileCountsEnabled is true (the default).
-- To revert reads immediately, set ready=false or disable the application flag.
COMMIT;
