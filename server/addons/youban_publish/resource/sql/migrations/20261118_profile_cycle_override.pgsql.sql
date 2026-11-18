-- Explicit deployment migration; never execute from startup.
-- Prerequisite: apply ../cleanup_intent_context.pgsql.sql first. The existing
-- TG Outbox must have payload and operation_no before enabling the save API.
-- Also apply ../batch_rotation.pgsql.sql first: disabling uses its batch_order
-- column and hg_ybp_batch_order_seq to rejoin the inherited batch tail.
-- Fresh installs already add these columns later in install.pgsql.sql;
-- finish the entire install before serving requests.
-- Then run 20261118_profile_cycle_override_outbox_index.pgsql.sql separately,
-- outside any transaction, before enabling the existing Outbox recovery hook.
-- Also run 20261118_profile_cycle_override_batch_index.pgsql.sql standalone
-- before enabling the inherited-batch query. Existing mapping rows stay false;
-- no historical data backfill is required.
ALTER TABLE hg_youban_publish_profile_state
  ADD COLUMN IF NOT EXISTS cycle_override_enabled boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS cycle_override_days integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS cycle_override_version bigint NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_publish_channel_profile
  ADD COLUMN IF NOT EXISTS cycle_override_enabled boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS applied_profile_cycle_version bigint NOT NULL DEFAULT 0;
ALTER TABLE hg_youban_publish_tg_job
  ADD COLUMN IF NOT EXISTS profile_cycle_version bigint NOT NULL DEFAULT 0;
