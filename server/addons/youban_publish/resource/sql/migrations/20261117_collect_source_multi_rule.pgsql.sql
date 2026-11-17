-- Explicit production migration; never run automatically in an application.
-- psql -X -v ON_ERROR_STOP=1 -f <this file>
-- Catalog-only checks: no business-row reads, writes, deletes or index builds.
\set ON_ERROR_STOP on

BEGIN;
SET LOCAL lock_timeout = '2s';
SET LOCAL statement_timeout = '15s';
SET LOCAL idle_in_transaction_session_timeout = '30s';

DO $migration$
DECLARE
    target_table oid := to_regclass('public.hg_youban_publish_collect_source_rule');
    target_namespace oid;
    source_attribute smallint;
    rule_attribute smallint;
    pair_index oid;
    legacy_index oid;
    legacy_definition record;
    owner_constraint record;
    owner_count integer;
    unexpected_single text;
BEGIN
    IF target_table IS NULL THEN
        RAISE EXCEPTION 'Missing public.hg_youban_publish_collect_source_rule; no changes made';
    END IF;
    IF NOT pg_try_advisory_xact_lock(20261117, 35101) THEN
        RAISE EXCEPTION 'Another collect source multi-rule migration is active; retry later';
    END IF;

    -- Short exclusive lock prevents concurrent schema changes between catalog
    -- validation and DROP. A busy table aborts after lock_timeout, not waits.
    LOCK TABLE public.hg_youban_publish_collect_source_rule IN ACCESS EXCLUSIVE MODE;

    SELECT relnamespace INTO target_namespace
    FROM pg_class WHERE oid = target_table;
    SELECT attnum INTO source_attribute
    FROM pg_attribute
    WHERE attrelid = target_table AND attname = 'source_id'
      AND attnum > 0 AND NOT attisdropped;
    SELECT attnum INTO rule_attribute
    FROM pg_attribute
    WHERE attrelid = target_table AND attname = 'rule_id'
      AND attnum > 0 AND NOT attisdropped;
    IF source_attribute IS NULL OR rule_attribute IS NULL THEN
        RAISE EXCEPTION 'Expected source_id/rule_id columns missing; no changes made';
    END IF;

    -- Require an already-built, valid, immediate, unconditional two-column
    -- unique index. Do not create one here: building it would scan the table.
    SELECT i.indexrelid INTO pair_index
    FROM pg_index i
    WHERE i.indrelid = target_table
      AND i.indisunique AND i.indisvalid AND i.indisready AND i.indimmediate
      AND i.indnkeyatts = 2 AND i.indexprs IS NULL AND i.indpred IS NULL
      AND ARRAY(
          SELECT key.attnum
          FROM unnest(i.indkey::smallint[]) WITH ORDINALITY AS key(attnum, position)
          WHERE key.position <= i.indnkeyatts
          ORDER BY key.position
      ) = ARRAY[source_attribute, rule_attribute]::smallint[]
    ORDER BY i.indexrelid
    LIMIT 1;
    IF pair_index IS NULL THEN
        RAISE EXCEPTION 'Missing valid UNIQUE(source_id,rule_id); build/verify it separately before this migration';
    END IF;
    RAISE NOTICE 'Preserving pair uniqueness: %', pg_get_indexdef(pair_index);

    SELECT c.oid INTO legacy_index
    FROM pg_class c
    WHERE c.relnamespace = target_namespace
      AND c.relname = 'uk_ybp_collect_source_single_rule';

    IF legacy_index IS NOT NULL THEN
        SELECT i.* INTO legacy_definition
        FROM pg_index i WHERE i.indexrelid = legacy_index;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'Legacy name exists but is not an index; refusing to drop';
        END IF;
        IF legacy_definition.indrelid <> target_table
           OR NOT legacy_definition.indisunique
           OR NOT legacy_definition.indisvalid
           OR legacy_definition.indnkeyatts <> 1
           OR legacy_definition.indexprs IS NOT NULL
           OR legacy_definition.indpred IS NOT NULL
           OR ARRAY(
               SELECT key.attnum
               FROM unnest(legacy_definition.indkey::smallint[]) WITH ORDINALITY AS key(attnum, position)
               WHERE key.position <= legacy_definition.indnkeyatts
               ORDER BY key.position
           ) <> ARRAY[source_attribute]::smallint[]
        THEN
            RAISE EXCEPTION 'Legacy index has an unexpected owner or definition: %', pg_get_indexdef(legacy_index);
        END IF;
        RAISE NOTICE 'Removing source-only uniqueness: %', pg_get_indexdef(legacy_index);

        SELECT count(*) INTO owner_count
        FROM pg_constraint
        WHERE conrelid = target_table AND conindid = legacy_index
          AND contype IN ('p', 'u', 'x');
        IF owner_count > 1 THEN
            RAISE EXCEPTION 'Legacy index has multiple owning constraints; manual review required';
        END IF;
        IF owner_count = 1 THEN
            SELECT conname, contype INTO owner_constraint
            FROM pg_constraint
            WHERE conrelid = target_table AND conindid = legacy_index
              AND contype IN ('p', 'u', 'x');
            IF owner_constraint.contype <> 'u' THEN
                RAISE EXCEPTION 'Legacy index is not owned by a UNIQUE constraint; refusing to drop';
            END IF;
            RAISE NOTICE 'Dropping owning UNIQUE constraint: %', owner_constraint.conname;
            EXECUTE format(
                'ALTER TABLE public.hg_youban_publish_collect_source_rule DROP CONSTRAINT %I RESTRICT',
                owner_constraint.conname
            );
        ELSE
            EXECUTE format('DROP INDEX %I.%I RESTRICT',
                'public', 'uk_ybp_collect_source_single_rule');
        END IF;
    ELSE
        RAISE NOTICE 'Legacy source-only index already absent';
    END IF;

    -- Unknown source-only uniqueness must not silently leave the defect alive.
    -- Raise rolls back any DROP above. Do not guess which other object to drop.
    SELECT pg_get_indexdef(i.indexrelid) INTO unexpected_single
    FROM pg_index i
    WHERE i.indrelid = target_table AND i.indisunique
      AND i.indnkeyatts = 1 AND i.indexprs IS NULL
      AND ARRAY(
          SELECT key.attnum
          FROM unnest(i.indkey::smallint[]) WITH ORDINALITY AS key(attnum, position)
          WHERE key.position <= i.indnkeyatts
          ORDER BY key.position
      ) = ARRAY[source_attribute]::smallint[]
    LIMIT 1;
    IF unexpected_single IS NOT NULL THEN
        RAISE EXCEPTION 'Another source-only unique index remains; transaction rolled back: %', unexpected_single;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_index
        WHERE indexrelid = pair_index AND indrelid = target_table
          AND indisunique AND indisvalid AND indisready AND indimmediate
    ) THEN
        RAISE EXCEPTION 'Pair unique protection changed; transaction rolled back';
    END IF;
    RAISE NOTICE 'Multi-rule migration verified; relationships and disabled states unchanged';
END;
$migration$;

-- A bounded catalog report, not a scan of collection relationships.
SELECT i.indexrelid::regclass AS index_name,
       pg_get_indexdef(i.indexrelid) AS index_definition,
       c.conname AS owning_constraint,
       c.contype AS constraint_type
FROM pg_index i
LEFT JOIN pg_constraint c
  ON c.conrelid = i.indrelid AND c.conindid = i.indexrelid
 AND c.contype IN ('p', 'u', 'x')
WHERE i.indrelid = 'public.hg_youban_publish_collect_source_rule'::regclass
ORDER BY i.indexrelid
LIMIT 50;

COMMIT;
