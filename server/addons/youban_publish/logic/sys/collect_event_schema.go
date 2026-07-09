package sys

import (
	"context"
	"strings"
	"sync"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/consts"
)

var collectEventStructuredSchemaMu sync.Mutex
var collectEventStructuredSchemaReady bool

func ensureCollectEventStructuredSchema(ctx context.Context) error {
	collectEventStructuredSchemaMu.Lock()
	defer collectEventStructuredSchemaMu.Unlock()
	if collectEventStructuredSchemaReady {
		return nil
	}
	var err error
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		err = ensureCollectEventStructuredPgsqlSchema(ctx)
	} else {
		err = ensureCollectEventStructuredMysqlSchema(ctx)
	}
	if err == nil {
		collectEventStructuredSchemaReady = true
	}
	return err
}

func ensureCollectEventStructuredPgsqlSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_event_media" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "source_id" bigint NOT NULL DEFAULT 0,
  "source_type" varchar(32) NOT NULL DEFAULT '',
  "event_id" bigint NOT NULL DEFAULT 0,
  "source_chat_id" varchar(128) NOT NULL DEFAULT '',
  "source_message_id" bigint NOT NULL DEFAULT 0,
  "source_grouped_id" varchar(128) NOT NULL DEFAULT '',
  "source_media_key" varchar(255) NOT NULL DEFAULT '',
  "media_type" varchar(32) NOT NULL DEFAULT '',
  "source_ref_type" varchar(32) NOT NULL DEFAULT '',
  "source_file_id" varchar(255) NOT NULL DEFAULT '',
  "source_message_ref" varchar(255) NOT NULL DEFAULT '',
  "backup_channel_id" bigint NOT NULL DEFAULT 0,
  "backup_chat_id" varchar(128) NOT NULL DEFAULT '',
  "backup_message_id" bigint NOT NULL DEFAULT 0,
  "file_url" varchar(1024) NOT NULL DEFAULT '',
  "storage_path" varchar(1024) NOT NULL DEFAULT '',
  "poster_url" varchar(1024) NOT NULL DEFAULT '',
  "meta_json" text,
  "sort_index" integer NOT NULL DEFAULT 0,
  "cache_status" varchar(32) NOT NULL DEFAULT 'pending',
  "error_message" text,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL
)`,
		`ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0`,
		`ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "account_id" bigint NOT NULL DEFAULT 0`,
		`ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_id" bigint NOT NULL DEFAULT 0`,
		`ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_type" varchar(32) NOT NULL DEFAULT ''`,
		`ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_chat_id" varchar(128) NOT NULL DEFAULT ''`,
		`ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "source_grouped_id" varchar(128) NOT NULL DEFAULT ''`,
		`ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "meta_json" text`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_media_event" ON "hg_youban_publish_collect_event_media" ("event_id", "sort_index", "id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_media_owner" ON "hg_youban_publish_collect_event_media" ("tenant_id", "source_id", "cache_status", "id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_media_source" ON "hg_youban_publish_collect_event_media" ("source_chat_id", "source_message_id", "source_media_key")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_media_file" ON "hg_youban_publish_collect_event_media" ("source_file_id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_media_cache" ON "hg_youban_publish_collect_event_media" ("cache_status", "updated_at", "id")`,
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_event_log" (
  "id" BIGSERIAL PRIMARY KEY,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "event_id" bigint NOT NULL DEFAULT 0,
  "dispatch_id" bigint NOT NULL DEFAULT 0,
  "stage" varchar(64) NOT NULL DEFAULT '',
  "status" varchar(32) NOT NULL DEFAULT '',
  "message" text,
  "meta_text" text,
  "created_at" timestamp DEFAULT NULL
)`,
		`ALTER TABLE "hg_youban_publish_collect_event_log" ADD COLUMN IF NOT EXISTS "tenant_id" bigint NOT NULL DEFAULT 0`,
		`ALTER TABLE "hg_youban_publish_collect_event_log" ADD COLUMN IF NOT EXISTS "account_id" bigint NOT NULL DEFAULT 0`,
		`ALTER TABLE "hg_youban_publish_collect_event_log" ADD COLUMN IF NOT EXISTS "dispatch_id" bigint NOT NULL DEFAULT 0`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_log_event" ON "hg_youban_publish_collect_event_log" ("event_id", "id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_log_owner" ON "hg_youban_publish_collect_event_log" ("tenant_id", "account_id", "created_at")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_collect_event_log_stage" ON "hg_youban_publish_collect_event_log" ("event_id", "stage", "status")`,
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			return gerror.Wrap(err, "检查采集事件结构化表失败")
		}
	}
	return nil
}

func ensureCollectEventStructuredMysqlSchema(ctx context.Context) error {
	statements := []string{
		"CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_event_media` (" +
			"`id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键'," +
			"`tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID'," +
			"`account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID'," +
			"`source_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集源ID'," +
			"`source_type` varchar(32) NOT NULL DEFAULT '' COMMENT '采集源类型'," +
			"`event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集事件ID'," +
			"`source_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '来源频道/群聊ID'," +
			"`source_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '来源消息ID'," +
			"`source_grouped_id` varchar(128) NOT NULL DEFAULT '' COMMENT '媒体组ID'," +
			"`source_media_key` varchar(255) NOT NULL DEFAULT '' COMMENT '来源媒体键'," +
			"`media_type` varchar(32) NOT NULL DEFAULT '' COMMENT '媒体类型'," +
			"`source_ref_type` varchar(32) NOT NULL DEFAULT '' COMMENT '来源引用类型'," +
			"`source_file_id` varchar(255) NOT NULL DEFAULT '' COMMENT '来源文件ID'," +
			"`source_message_ref` varchar(255) NOT NULL DEFAULT '' COMMENT '来源消息引用'," +
			"`backup_channel_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '备份频道ID'," +
			"`backup_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT '备份聊天ID'," +
			"`backup_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '备份消息ID'," +
			"`file_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '文件访问地址'," +
			"`storage_path` varchar(1024) NOT NULL DEFAULT '' COMMENT '存储路径'," +
			"`poster_url` varchar(1024) NOT NULL DEFAULT '' COMMENT '封面地址'," +
			"`meta_json` text COMMENT '媒体元数据'," +
			"`sort_index` int(11) NOT NULL DEFAULT '0' COMMENT '排序'," +
			"`cache_status` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '缓存状态'," +
			"`error_message` text COMMENT '错误信息'," +
			"`created_at` datetime DEFAULT NULL COMMENT '创建时间'," +
			"`updated_at` datetime DEFAULT NULL COMMENT '更新时间'," +
			"PRIMARY KEY (`id`)," +
			"KEY `idx_ybp_collect_event_media_event` (`event_id`,`sort_index`,`id`)," +
			"KEY `idx_ybp_collect_event_media_owner` (`tenant_id`,`source_id`,`cache_status`,`id`)," +
			"KEY `idx_ybp_collect_event_media_source` (`source_chat_id`,`source_message_id`,`source_media_key`)," +
			"KEY `idx_ybp_collect_event_media_file` (`source_file_id`)," +
			"KEY `idx_ybp_collect_event_media_cache` (`cache_status`,`updated_at`,`id`)" +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴采集事件媒体'",
		"ALTER TABLE `hg_youban_publish_collect_event_media` ADD COLUMN `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID'",
		"ALTER TABLE `hg_youban_publish_collect_event_media` ADD COLUMN `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID'",
		"ALTER TABLE `hg_youban_publish_collect_event_media` ADD COLUMN `source_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集源ID'",
		"ALTER TABLE `hg_youban_publish_collect_event_media` ADD COLUMN `source_type` varchar(32) NOT NULL DEFAULT '' COMMENT '采集源类型'",
		"ALTER TABLE `hg_youban_publish_collect_event_media` ADD COLUMN `source_grouped_id` varchar(128) NOT NULL DEFAULT '' COMMENT '媒体组ID'",
		"ALTER TABLE `hg_youban_publish_collect_event_media` ADD COLUMN `meta_json` text COMMENT '媒体元数据'",
		"ALTER TABLE `hg_youban_publish_collect_event_media` ADD KEY `idx_ybp_collect_event_media_event` (`event_id`,`sort_index`,`id`)",
		"ALTER TABLE `hg_youban_publish_collect_event_media` ADD KEY `idx_ybp_collect_event_media_owner` (`tenant_id`,`source_id`,`cache_status`,`id`)",
		"ALTER TABLE `hg_youban_publish_collect_event_media` ADD KEY `idx_ybp_collect_event_media_source` (`source_chat_id`,`source_message_id`,`source_media_key`)",
		"ALTER TABLE `hg_youban_publish_collect_event_media` ADD KEY `idx_ybp_collect_event_media_file` (`source_file_id`)",
		"ALTER TABLE `hg_youban_publish_collect_event_media` ADD KEY `idx_ybp_collect_event_media_cache` (`cache_status`,`updated_at`,`id`)",
		"CREATE TABLE IF NOT EXISTS `hg_youban_publish_collect_event_log` (" +
			"`id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键'," +
			"`tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID'," +
			"`account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID'," +
			"`event_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '采集事件ID'," +
			"`dispatch_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '分发ID'," +
			"`stage` varchar(64) NOT NULL DEFAULT '' COMMENT '阶段'," +
			"`status` varchar(32) NOT NULL DEFAULT '' COMMENT '状态'," +
			"`message` text COMMENT '日志内容'," +
			"`meta_text` text COMMENT '上下文文本'," +
			"`created_at` datetime DEFAULT NULL COMMENT '创建时间'," +
			"PRIMARY KEY (`id`)," +
			"KEY `idx_ybp_collect_event_log_event` (`event_id`,`id`)," +
			"KEY `idx_ybp_collect_event_log_owner` (`tenant_id`,`account_id`,`created_at`)," +
			"KEY `idx_ybp_collect_event_log_stage` (`event_id`,`stage`,`status`)" +
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴采集事件日志'",
		"ALTER TABLE `hg_youban_publish_collect_event_log` ADD COLUMN `tenant_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID'",
		"ALTER TABLE `hg_youban_publish_collect_event_log` ADD COLUMN `account_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '账号ID'",
		"ALTER TABLE `hg_youban_publish_collect_event_log` ADD COLUMN `dispatch_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '分发ID'",
		"ALTER TABLE `hg_youban_publish_collect_event_log` ADD KEY `idx_ybp_collect_event_log_event` (`event_id`,`id`)",
		"ALTER TABLE `hg_youban_publish_collect_event_log` ADD KEY `idx_ybp_collect_event_log_owner` (`tenant_id`,`account_id`,`created_at`)",
		"ALTER TABLE `hg_youban_publish_collect_event_log` ADD KEY `idx_ybp_collect_event_log_stage` (`event_id`,`stage`,`status`)",
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil && !isIgnorableCollectEventStructuredSchemaError(err) {
			return gerror.Wrap(err, "检查采集事件结构化表失败")
		}
	}
	return nil
}

func isIgnorableCollectEventStructuredSchemaError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "already exists") ||
		strings.Contains(message, "duplicate column") ||
		strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "duplicate key name")
}
