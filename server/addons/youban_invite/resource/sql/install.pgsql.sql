CREATE TABLE IF NOT EXISTS "hg_yb_invite_config" (
  "id" BIGSERIAL PRIMARY KEY,
  "enabled" smallint NOT NULL DEFAULT 1,
  "base_url" varchar(500) NOT NULL DEFAULT '',
  "level1_min" integer NOT NULL DEFAULT 1,
  "level1_max" integer NOT NULL DEFAULT 5,
  "level1_rate" numeric(8,4) NOT NULL DEFAULT 2.0000,
  "level2_min" integer NOT NULL DEFAULT 6,
  "level2_rate" numeric(8,4) NOT NULL DEFAULT 3.0000,
  "manual_audit" smallint NOT NULL DEFAULT 0,
  "remark" varchar(500) NOT NULL DEFAULT '',
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS "hg_yb_invite_rebate" (
  "id" BIGSERIAL PRIMARY KEY,
  "inviter_id" bigint NOT NULL DEFAULT 0,
  "invitee_id" bigint NOT NULL DEFAULT 0,
  "invite_code" varchar(64) NOT NULL DEFAULT '',
  "order_id" bigint NOT NULL DEFAULT 0,
  "order_sn" varchar(64) NOT NULL DEFAULT '',
  "trade_type" varchar(64) NOT NULL DEFAULT 'member_vip',
  "order_amount" numeric(10,2) NOT NULL DEFAULT 0.00,
  "rebate_rate" numeric(8,4) NOT NULL DEFAULT 0.0000,
  "rebate_amount" numeric(10,2) NOT NULL DEFAULT 0.00,
  "settle_status" varchar(32) NOT NULL DEFAULT 'settled',
  "settled_at" timestamp DEFAULT NULL,
  "remark" varchar(500) NOT NULL DEFAULT '',
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS "uk_yb_invite_rebate_order" ON "hg_yb_invite_rebate" ("trade_type", "order_sn");
CREATE INDEX IF NOT EXISTS "idx_yb_invite_rebate_inviter" ON "hg_yb_invite_rebate" ("inviter_id");
CREATE INDEX IF NOT EXISTS "idx_yb_invite_rebate_invitee" ON "hg_yb_invite_rebate" ("invitee_id");

INSERT INTO "hg_yb_invite_config" ("id", "enabled", "base_url", "level1_min", "level1_max", "level1_rate", "level2_min", "level2_rate", "manual_audit", "remark", "created_at", "updated_at")
SELECT 1, 1, 'https://yuebanby.com', 1, 5, 2.0000, 6, 3.0000, 0, '默认邀请返现配置', NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM "hg_yb_invite_config" WHERE "id" = 1);
