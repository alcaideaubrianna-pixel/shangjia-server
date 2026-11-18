-- Explicit MySQL migration; execute once, never from startup.
-- Prerequisite: apply ../cleanup_intent_context.sql first. The existing TG
-- Outbox must have payload and operation_no before enabling the save API.
-- Fresh installs add these columns later in install.sql; finish the full install.
-- MySQL uses a covering index because it does not support partial indexes.
ALTER TABLE `hg_youban_publish_profile_state`
  ADD COLUMN `cycle_override_enabled` boolean NOT NULL DEFAULT false,
  ADD COLUMN `cycle_override_days` int NOT NULL DEFAULT 0,
  ADD COLUMN `cycle_override_version` bigint NOT NULL DEFAULT 0;
ALTER TABLE `hg_youban_publish_channel_profile`
  ADD COLUMN `cycle_override_enabled` boolean NOT NULL DEFAULT false,
  ADD COLUMN `applied_profile_cycle_version` bigint NOT NULL DEFAULT 0;
ALTER TABLE `hg_youban_publish_tg_job`
  ADD COLUMN `profile_cycle_version` bigint NOT NULL DEFAULT 0;
CREATE INDEX `idx_ybp_profile_cycle_override_due`
  ON `hg_youban_publish_tg_outbox` (`event_type`,`status`,`available_at`,`id`);
