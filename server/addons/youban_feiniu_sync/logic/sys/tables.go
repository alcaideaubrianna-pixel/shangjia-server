package sys

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_feiniu_sync/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/cron"
	"hotgo/internal/model/entity"
)

var (
	syncExistingConfigCronsMu   sync.Mutex
	syncExistingConfigCronsDone bool
)

func ensureTables(ctx context.Context) error {
	return ensureTablesWithCron(ctx, true)
}

func ensureTablesWithCron(ctx context.Context, syncCrons bool) error {
	installSQL := mysqlInstallSQL
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		installSQL = pgsqlInstallSQL
	}
	for _, sql := range splitSQL(installSQL) {
		sql = strings.TrimSpace(sql)
		if sql == "" {
			continue
		}
		if _, err := g.DB().Exec(ctx, sql); err != nil {
			if isIgnorableSQLError(err) {
				continue
			}
			return gerror.Wrap(err, "初始化 FeiNiu 同步表失败")
		}
	}
	if err := ensureDefaultCron(ctx); err != nil {
		return err
	}
	if syncCrons {
		return syncExistingConfigCronsIfNeeded(ctx)
	}
	return nil
}

func ensureDefaultCron(ctx context.Context) error {
	columns := dao.SysCron.Columns()
	now := gtime.Now()
	row, err := dao.SysCron.Ctx(ctx).Where(columns.Name, "youbanFeiniuSync").Where(columns.Params, "").One()
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "does not exist") {
			return nil
		}
		return gerror.Wrap(err, "读取 FeiNiu 同步定时任务失败")
	}

	data := g.Map{
		columns.GroupId:   1,
		columns.Title:     "FeiNiu资料自动同步",
		columns.Name:      "youbanFeiniuSync",
		columns.Params:    "",
		columns.Pattern:   "@every 1m",
		columns.Policy:    consts.CronPolicySingle,
		columns.Count:     0,
		columns.Sort:      20,
		columns.Remark:    "旧版全局轮询任务，已停用",
		columns.Status:    consts.StatusDisable,
		columns.UpdatedAt: now,
	}
	if row.IsEmpty() {
		data[columns.CreatedAt] = now
		_, err = dao.SysCron.Ctx(ctx).Data(data).Insert()
	} else {
		_, err = dao.SysCron.Ctx(ctx).Where(columns.Id, row[columns.Id].Int64()).Data(data).Update()
	}
	if err != nil {
		return gerror.Wrap(err, "初始化 FeiNiu 同步定时任务失败")
	}

	var cronRow *entity.SysCron
	if err = dao.SysCron.Ctx(ctx).Where(columns.Name, "youbanFeiniuSync").Where(columns.Params, "").Scan(&cronRow); err != nil {
		return gerror.Wrap(err, "读取 FeiNiu 同步定时任务失败")
	}
	return cron.RefreshStatus(cronRow)
}

func syncExistingConfigCronsIfNeeded(ctx context.Context) error {
	syncExistingConfigCronsMu.Lock()
	defer syncExistingConfigCronsMu.Unlock()
	if syncExistingConfigCronsDone {
		return nil
	}
	if err := syncExistingConfigCrons(ctx); err != nil {
		return err
	}
	syncExistingConfigCronsDone = true
	return nil
}

func syncExistingConfigCrons(ctx context.Context) error {
	var configs []gdb.Record
	if err := g.DB().Model(configTable).Safe().Ctx(ctx).WhereNull("deleted_at").OrderAsc("id").Scan(&configs); err != nil {
		return gerror.Wrap(err, "同步 FeiNiu 配置定时任务失败")
	}
	for _, cfg := range configs {
		if err := syncConfigCron(ctx, cfg, false); err != nil {
			return err
		}
	}
	return nil
}

func syncConfigCron(ctx context.Context, cfg gdb.Record, runNow bool) error {
	columns := dao.SysCron.Columns()
	now := gtime.Now()
	configId := cfg["id"].Int64()
	if configId <= 0 {
		return gerror.New("同步配置ID不能为空")
	}
	params := fmt.Sprintf("%d", configId)
	pattern := syncConfigCronPattern(cfg["sync_interval_minutes"].Int())
	status := consts.StatusDisable
	if cfg["status"].Int() == sysin.SyncStatusEnabled && cfg["auto_sync_enabled"].Int() == sysin.SyncStatusEnabled {
		status = consts.StatusEnabled
	}
	data := g.Map{
		columns.GroupId:   1,
		columns.Title:     fmt.Sprintf("FeiNiu资料自动同步[%s]", cfg["name"].String()),
		columns.Name:      "youbanFeiniuSync",
		columns.Params:    params,
		columns.Pattern:   pattern,
		columns.Policy:    consts.CronPolicySingle,
		columns.Count:     0,
		columns.Sort:      20,
		columns.Remark:    "FeiNiu 配置自动同步",
		columns.Status:    status,
		columns.UpdatedAt: now,
	}
	row, err := dao.SysCron.Ctx(ctx).Where(columns.Name, "youbanFeiniuSync").Where(columns.Params, params).One()
	if err != nil {
		return gerror.Wrap(err, "读取 FeiNiu 配置定时任务失败")
	}
	if row.IsEmpty() {
		data[columns.CreatedAt] = now
		_, err = dao.SysCron.Ctx(ctx).Data(data).Insert()
	} else {
		_, err = dao.SysCron.Ctx(ctx).Where(columns.Id, row[columns.Id].Int64()).Data(data).Update()
	}
	if err != nil {
		return gerror.Wrap(err, "初始化 FeiNiu 配置定时任务失败")
	}

	var cronRow *entity.SysCron
	if err = dao.SysCron.Ctx(ctx).Where(columns.Name, "youbanFeiniuSync").Where(columns.Params, params).Scan(&cronRow); err != nil {
		return gerror.Wrap(err, "读取 FeiNiu 配置定时任务失败")
	}
	if err = cron.RefreshStatus(cronRow); err != nil {
		return gerror.Wrap(err, "刷新 FeiNiu 配置定时任务失败")
	}
	if runNow && status == consts.StatusEnabled {
		return cron.Once(ctx, cronRow)
	}
	return nil
}

func disableConfigCron(ctx context.Context, configId int64) error {
	columns := dao.SysCron.Columns()
	params := fmt.Sprintf("%d", configId)
	row, err := dao.SysCron.Ctx(ctx).Where(columns.Name, "youbanFeiniuSync").Where(columns.Params, params).One()
	if err != nil {
		return gerror.Wrap(err, "读取 FeiNiu 配置定时任务失败")
	}
	if row.IsEmpty() {
		return nil
	}
	_, err = dao.SysCron.Ctx(ctx).Where(columns.Id, row[columns.Id].Int64()).Data(g.Map{columns.Status: consts.StatusDisable, columns.UpdatedAt: gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "停用 FeiNiu 配置定时任务失败")
	}
	var cronRow *entity.SysCron
	if err = dao.SysCron.Ctx(ctx).Where(columns.Id, row[columns.Id].Int64()).Scan(&cronRow); err != nil {
		return gerror.Wrap(err, "读取 FeiNiu 配置定时任务失败")
	}
	return cron.RefreshStatus(cronRow)
}

func syncConfigCronPattern(minutes int) string {
	if minutes <= 0 {
		minutes = 10
	}
	return fmt.Sprintf("@every %dm", minutes)
}

func splitSQL(content string) []string {
	var list []string
	var builder strings.Builder
	var quote rune
	for _, r := range content {
		builder.WriteRune(r)
		switch r {
		case '\'', '"', '`':
			if quote == 0 {
				quote = r
			} else if quote == r {
				quote = 0
			}
		case ';':
			if quote == 0 {
				list = append(list, builder.String())
				builder.Reset()
			}
		}
	}
	if tail := strings.TrimSpace(builder.String()); tail != "" {
		list = append(list, tail)
	}
	return list
}

func isIgnorableSQLError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") || strings.Contains(msg, "duplicate key") || strings.Contains(msg, "already exists")
}

const mysqlInstallSQL = `
CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_config (
  id bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  name varchar(128) NOT NULL DEFAULT '',
  db_type varchar(32) NOT NULL DEFAULT 'mysql',
  db_host varchar(255) NOT NULL DEFAULT '',
  db_port int(11) NOT NULL DEFAULT '3306',
  db_name varchar(128) NOT NULL DEFAULT '',
  db_user varchar(128) NOT NULL DEFAULT '',
  db_password varchar(1024) NOT NULL DEFAULT '',
  target_tenant_id bigint(20) NOT NULL DEFAULT '0',
  target_parent_account_id bigint(20) NOT NULL DEFAULT '0',
  auto_create_account tinyint(1) NOT NULL DEFAULT '1',
  sync_media tinyint(1) NOT NULL DEFAULT '1',
  sync_verify_media tinyint(1) NOT NULL DEFAULT '1',
  auto_sync_enabled tinyint(1) NOT NULL DEFAULT '1',
  sync_interval_minutes int(11) NOT NULL DEFAULT '10',
  batch_size int(11) NOT NULL DEFAULT '100',
  last_source_note_id bigint(20) NOT NULL DEFAULT '0',
  status tinyint(1) NOT NULL DEFAULT '1',
  last_run_at datetime DEFAULT NULL,
  last_success_at datetime DEFAULT NULL,
  last_error text,
  created_at datetime DEFAULT NULL,
  updated_at datetime DEFAULT NULL,
  deleted_at datetime DEFAULT NULL,
  PRIMARY KEY (id),
  KEY idx_yfs_config_status (status,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='FeiNiu同步配置';
ALTER TABLE hg_youban_feiniu_sync_config ADD COLUMN auto_sync_enabled tinyint(1) NOT NULL DEFAULT '1';
ALTER TABLE hg_youban_feiniu_sync_config ADD COLUMN sync_interval_minutes int(11) NOT NULL DEFAULT '10';
ALTER TABLE hg_youban_feiniu_sync_config ADD COLUMN last_source_note_id bigint(20) NOT NULL DEFAULT '0';
CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_channel_map (
  id bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  config_id bigint(20) NOT NULL DEFAULT '0',
  feiniu_channel_id bigint(20) NOT NULL DEFAULT '0',
  feiniu_tg_chat_id bigint(20) NOT NULL DEFAULT '0',
  feiniu_channel_title varchar(255) NOT NULL DEFAULT '',
  feiniu_username varchar(255) NOT NULL DEFAULT '',
  youban_tenant_id bigint(20) NOT NULL DEFAULT '0',
  youban_account_id bigint(20) NOT NULL DEFAULT '0',
  youban_account_username varchar(128) NOT NULL DEFAULT '',
  last_source_update_time datetime DEFAULT NULL,
  last_source_note_id bigint(20) NOT NULL DEFAULT '0',
  sync_status varchar(32) NOT NULL DEFAULT 'pending',
  error_message text,
  created_at datetime DEFAULT NULL,
  updated_at datetime DEFAULT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_yfs_channel (config_id,feiniu_channel_id),
  KEY idx_yfs_channel_chat (config_id,feiniu_tg_chat_id),
  KEY idx_yfs_channel_account (youban_tenant_id,youban_account_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='FeiNiu频道映射';
ALTER TABLE hg_youban_feiniu_sync_channel_map ADD INDEX idx_yfs_channel_chat (config_id,feiniu_tg_chat_id);
CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_profile_map (
  id bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  config_id bigint(20) NOT NULL DEFAULT '0',
  feiniu_note_id bigint(20) NOT NULL DEFAULT '0',
  feiniu_note_uuid varchar(64) NOT NULL DEFAULT '',
  feiniu_note_code varchar(32) NOT NULL DEFAULT '',
  feiniu_source_key varchar(255) NOT NULL DEFAULT '',
  feiniu_channel_id bigint(20) NOT NULL DEFAULT '0',
  feiniu_tg_chat_id bigint(20) NOT NULL DEFAULT '0',
  youban_profile_id bigint(20) NOT NULL DEFAULT '0',
  youban_task_id bigint(20) NOT NULL DEFAULT '0',
  youban_account_id bigint(20) NOT NULL DEFAULT '0',
  source_updated_at datetime DEFAULT NULL,
  content_hash varchar(64) NOT NULL DEFAULT '',
  dedupe_key varchar(255) NOT NULL DEFAULT '',
  duplicate_profile_id bigint(20) NOT NULL DEFAULT '0',
  sync_status varchar(32) NOT NULL DEFAULT 'pending',
  error_message text,
  created_at datetime DEFAULT NULL,
  updated_at datetime DEFAULT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_yfs_profile (config_id,feiniu_note_id),
  KEY idx_yfs_profile_cursor (config_id,source_updated_at,feiniu_note_id),
  KEY idx_yfs_profile_status (config_id,sync_status,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='FeiNiu资料映射';
ALTER TABLE hg_youban_feiniu_sync_profile_map ADD COLUMN dedupe_key varchar(255) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_feiniu_sync_profile_map ADD COLUMN duplicate_profile_id bigint(20) NOT NULL DEFAULT '0';
CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_run (
  id bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  config_id bigint(20) NOT NULL DEFAULT '0',
  run_type varchar(32) NOT NULL DEFAULT 'manual',
  status varchar(32) NOT NULL DEFAULT 'running',
  total_count int(11) NOT NULL DEFAULT '0',
  created_count int(11) NOT NULL DEFAULT '0',
  updated_count int(11) NOT NULL DEFAULT '0',
  skipped_count int(11) NOT NULL DEFAULT '0',
  failed_count int(11) NOT NULL DEFAULT '0',
  started_at datetime DEFAULT NULL,
  finished_at datetime DEFAULT NULL,
  error_message text,
  runtime_log longtext,
  created_at datetime DEFAULT NULL,
  PRIMARY KEY (id),
  KEY idx_yfs_run_config (config_id,id),
  KEY idx_yfs_run_status (status,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='FeiNiu同步运行记录';

CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_daily_stat (
  id bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  stat_date date NOT NULL,
  config_id bigint(20) NOT NULL DEFAULT '0',
  run_count int(11) NOT NULL DEFAULT '0',
  total_count int(11) NOT NULL DEFAULT '0',
  success_count int(11) NOT NULL DEFAULT '0',
  created_count int(11) NOT NULL DEFAULT '0',
  updated_count int(11) NOT NULL DEFAULT '0',
  skipped_count int(11) NOT NULL DEFAULT '0',
  failed_count int(11) NOT NULL DEFAULT '0',
  channel_count int(11) NOT NULL DEFAULT '0',
  profile_count int(11) NOT NULL DEFAULT '0',
  avg_duration_ms bigint(20) NOT NULL DEFAULT '0',
  last_run_id bigint(20) NOT NULL DEFAULT '0',
  last_run_at datetime DEFAULT NULL,
  created_at datetime DEFAULT NULL,
  updated_at datetime DEFAULT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_yfs_daily (config_id,stat_date),
  KEY idx_yfs_daily_date (stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='FeiNiu同步每日统计';
CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_channel_daily_stat (
  id bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  stat_date date NOT NULL,
  config_id bigint(20) NOT NULL DEFAULT '0',
  feiniu_channel_id bigint(20) NOT NULL DEFAULT '0',
  feiniu_tg_chat_id bigint(20) NOT NULL DEFAULT '0',
  feiniu_channel_title varchar(255) NOT NULL DEFAULT '',
  youban_account_id bigint(20) NOT NULL DEFAULT '0',
  youban_account_username varchar(128) NOT NULL DEFAULT '',
  total_count int(11) NOT NULL DEFAULT '0',
  created_count int(11) NOT NULL DEFAULT '0',
  updated_count int(11) NOT NULL DEFAULT '0',
  skipped_count int(11) NOT NULL DEFAULT '0',
  failed_count int(11) NOT NULL DEFAULT '0',
  last_note_id bigint(20) NOT NULL DEFAULT '0',
  last_source_update_time datetime DEFAULT NULL,
  created_at datetime DEFAULT NULL,
  updated_at datetime DEFAULT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_yfs_channel_daily (config_id,stat_date,feiniu_channel_id),
  KEY idx_yfs_channel_daily_rank (config_id,stat_date,total_count)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='FeiNiu同步频道每日统计';
CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_run_item (
  id bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  run_id bigint(20) NOT NULL DEFAULT '0',
  config_id bigint(20) NOT NULL DEFAULT '0',
  feiniu_note_id bigint(20) NOT NULL DEFAULT '0',
  feiniu_note_code varchar(32) NOT NULL DEFAULT '',
  feiniu_channel_id bigint(20) NOT NULL DEFAULT '0',
  feiniu_channel_title varchar(255) NOT NULL DEFAULT '',
  youban_profile_id bigint(20) NOT NULL DEFAULT '0',
  youban_task_id bigint(20) NOT NULL DEFAULT '0',
  action varchar(32) NOT NULL DEFAULT '',
  status varchar(32) NOT NULL DEFAULT '',
  error_message text,
  source_updated_at datetime DEFAULT NULL,
  duration_ms bigint(20) NOT NULL DEFAULT '0',
  created_at datetime DEFAULT NULL,
  PRIMARY KEY (id),
  KEY idx_yfs_run_item_run (run_id,id),
  KEY idx_yfs_run_item_config (config_id,created_at),
  KEY idx_yfs_run_item_status (config_id,status,created_at),
  KEY idx_yfs_run_item_channel (config_id,feiniu_channel_id,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='FeiNiu同步运行明细';
`

const pgsqlInstallSQL = `
CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_config (
  id bigserial PRIMARY KEY,
  name varchar(128) NOT NULL DEFAULT '',
  db_type varchar(32) NOT NULL DEFAULT 'mysql',
  db_host varchar(255) NOT NULL DEFAULT '',
  db_port integer NOT NULL DEFAULT 3306,
  db_name varchar(128) NOT NULL DEFAULT '',
  db_user varchar(128) NOT NULL DEFAULT '',
  db_password varchar(1024) NOT NULL DEFAULT '',
  target_tenant_id bigint NOT NULL DEFAULT 0,
  target_parent_account_id bigint NOT NULL DEFAULT 0,
  auto_create_account smallint NOT NULL DEFAULT 1,
  sync_media smallint NOT NULL DEFAULT 1,
  sync_verify_media smallint NOT NULL DEFAULT 1,
  auto_sync_enabled smallint NOT NULL DEFAULT 1,
  sync_interval_minutes integer NOT NULL DEFAULT 10,
  batch_size integer NOT NULL DEFAULT 100,
  last_source_note_id bigint NOT NULL DEFAULT 0,
  status smallint NOT NULL DEFAULT 1,
  last_run_at timestamp DEFAULT NULL,
  last_success_at timestamp DEFAULT NULL,
  last_error text,
  created_at timestamp DEFAULT NULL,
  updated_at timestamp DEFAULT NULL,
  deleted_at timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS idx_yfs_config_status ON hg_youban_feiniu_sync_config (status,id);
ALTER TABLE hg_youban_feiniu_sync_config ADD COLUMN IF NOT EXISTS auto_sync_enabled smallint NOT NULL DEFAULT 1;
ALTER TABLE hg_youban_feiniu_sync_config ADD COLUMN IF NOT EXISTS sync_interval_minutes integer NOT NULL DEFAULT 10;
ALTER TABLE hg_youban_feiniu_sync_config ADD COLUMN IF NOT EXISTS last_source_note_id bigint NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_channel_map (
  id bigserial PRIMARY KEY,
  config_id bigint NOT NULL DEFAULT 0,
  feiniu_channel_id bigint NOT NULL DEFAULT 0,
  feiniu_tg_chat_id bigint NOT NULL DEFAULT 0,
  feiniu_channel_title varchar(255) NOT NULL DEFAULT '',
  feiniu_username varchar(255) NOT NULL DEFAULT '',
  youban_tenant_id bigint NOT NULL DEFAULT 0,
  youban_account_id bigint NOT NULL DEFAULT 0,
  youban_account_username varchar(128) NOT NULL DEFAULT '',
  last_source_update_time timestamp DEFAULT NULL,
  last_source_note_id bigint NOT NULL DEFAULT 0,
  sync_status varchar(32) NOT NULL DEFAULT 'pending',
  error_message text,
  created_at timestamp DEFAULT NULL,
  updated_at timestamp DEFAULT NULL,
  CONSTRAINT uk_yfs_channel UNIQUE (config_id,feiniu_channel_id)
);
CREATE INDEX IF NOT EXISTS idx_yfs_channel_chat ON hg_youban_feiniu_sync_channel_map (config_id,feiniu_tg_chat_id);
CREATE INDEX IF NOT EXISTS idx_yfs_channel_account ON hg_youban_feiniu_sync_channel_map (youban_tenant_id,youban_account_id);
CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_profile_map (
  id bigserial PRIMARY KEY,
  config_id bigint NOT NULL DEFAULT 0,
  feiniu_note_id bigint NOT NULL DEFAULT 0,
  feiniu_note_uuid varchar(64) NOT NULL DEFAULT '',
  feiniu_note_code varchar(32) NOT NULL DEFAULT '',
  feiniu_source_key varchar(255) NOT NULL DEFAULT '',
  feiniu_channel_id bigint NOT NULL DEFAULT 0,
  feiniu_tg_chat_id bigint NOT NULL DEFAULT 0,
  youban_profile_id bigint NOT NULL DEFAULT 0,
  youban_task_id bigint NOT NULL DEFAULT 0,
  youban_account_id bigint NOT NULL DEFAULT 0,
  source_updated_at timestamp DEFAULT NULL,
  content_hash varchar(64) NOT NULL DEFAULT '',
  dedupe_key varchar(255) NOT NULL DEFAULT '',
  duplicate_profile_id bigint NOT NULL DEFAULT 0,
  sync_status varchar(32) NOT NULL DEFAULT 'pending',
  error_message text,
  created_at timestamp DEFAULT NULL,
  updated_at timestamp DEFAULT NULL,
  CONSTRAINT uk_yfs_profile UNIQUE (config_id,feiniu_note_id)
);
CREATE INDEX IF NOT EXISTS idx_yfs_profile_cursor ON hg_youban_feiniu_sync_profile_map (config_id,source_updated_at,feiniu_note_id);
CREATE INDEX IF NOT EXISTS idx_yfs_profile_status ON hg_youban_feiniu_sync_profile_map (config_id,sync_status,id);
ALTER TABLE hg_youban_feiniu_sync_profile_map ADD COLUMN IF NOT EXISTS dedupe_key varchar(255) NOT NULL DEFAULT '';
ALTER TABLE hg_youban_feiniu_sync_profile_map ADD COLUMN IF NOT EXISTS duplicate_profile_id bigint NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_run (
  id bigserial PRIMARY KEY,
  config_id bigint NOT NULL DEFAULT 0,
  run_type varchar(32) NOT NULL DEFAULT 'manual',
  status varchar(32) NOT NULL DEFAULT 'running',
  total_count integer NOT NULL DEFAULT 0,
  created_count integer NOT NULL DEFAULT 0,
  updated_count integer NOT NULL DEFAULT 0,
  skipped_count integer NOT NULL DEFAULT 0,
  failed_count integer NOT NULL DEFAULT 0,
  started_at timestamp DEFAULT NULL,
  finished_at timestamp DEFAULT NULL,
  error_message text,
  runtime_log text,
  created_at timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS idx_yfs_run_config ON hg_youban_feiniu_sync_run (config_id,id);
CREATE INDEX IF NOT EXISTS idx_yfs_run_status ON hg_youban_feiniu_sync_run (status,id);

CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_daily_stat (
  id bigserial PRIMARY KEY,
  stat_date date NOT NULL,
  config_id bigint NOT NULL DEFAULT 0,
  run_count integer NOT NULL DEFAULT 0,
  total_count integer NOT NULL DEFAULT 0,
  success_count integer NOT NULL DEFAULT 0,
  created_count integer NOT NULL DEFAULT 0,
  updated_count integer NOT NULL DEFAULT 0,
  skipped_count integer NOT NULL DEFAULT 0,
  failed_count integer NOT NULL DEFAULT 0,
  channel_count integer NOT NULL DEFAULT 0,
  profile_count integer NOT NULL DEFAULT 0,
  avg_duration_ms bigint NOT NULL DEFAULT 0,
  last_run_id bigint NOT NULL DEFAULT 0,
  last_run_at timestamp DEFAULT NULL,
  created_at timestamp DEFAULT NULL,
  updated_at timestamp DEFAULT NULL,
  CONSTRAINT uk_yfs_daily UNIQUE (config_id,stat_date)
);
CREATE INDEX IF NOT EXISTS idx_yfs_daily_date ON hg_youban_feiniu_sync_daily_stat (stat_date);
CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_channel_daily_stat (
  id bigserial PRIMARY KEY,
  stat_date date NOT NULL,
  config_id bigint NOT NULL DEFAULT 0,
  feiniu_channel_id bigint NOT NULL DEFAULT 0,
  feiniu_tg_chat_id bigint NOT NULL DEFAULT 0,
  feiniu_channel_title varchar(255) NOT NULL DEFAULT '',
  youban_account_id bigint NOT NULL DEFAULT 0,
  youban_account_username varchar(128) NOT NULL DEFAULT '',
  total_count integer NOT NULL DEFAULT 0,
  created_count integer NOT NULL DEFAULT 0,
  updated_count integer NOT NULL DEFAULT 0,
  skipped_count integer NOT NULL DEFAULT 0,
  failed_count integer NOT NULL DEFAULT 0,
  last_note_id bigint NOT NULL DEFAULT 0,
  last_source_update_time timestamp DEFAULT NULL,
  created_at timestamp DEFAULT NULL,
  updated_at timestamp DEFAULT NULL,
  CONSTRAINT uk_yfs_channel_daily UNIQUE (config_id,stat_date,feiniu_channel_id)
);
CREATE INDEX IF NOT EXISTS idx_yfs_channel_daily_rank ON hg_youban_feiniu_sync_channel_daily_stat (config_id,stat_date,total_count);
CREATE TABLE IF NOT EXISTS hg_youban_feiniu_sync_run_item (
  id bigserial PRIMARY KEY,
  run_id bigint NOT NULL DEFAULT 0,
  config_id bigint NOT NULL DEFAULT 0,
  feiniu_note_id bigint NOT NULL DEFAULT 0,
  feiniu_note_code varchar(32) NOT NULL DEFAULT '',
  feiniu_channel_id bigint NOT NULL DEFAULT 0,
  feiniu_channel_title varchar(255) NOT NULL DEFAULT '',
  youban_profile_id bigint NOT NULL DEFAULT 0,
  youban_task_id bigint NOT NULL DEFAULT 0,
  action varchar(32) NOT NULL DEFAULT '',
  status varchar(32) NOT NULL DEFAULT '',
  error_message text,
  source_updated_at timestamp DEFAULT NULL,
  duration_ms bigint NOT NULL DEFAULT 0,
  created_at timestamp DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS idx_yfs_run_item_run ON hg_youban_feiniu_sync_run_item (run_id,id);
CREATE INDEX IF NOT EXISTS idx_yfs_run_item_config ON hg_youban_feiniu_sync_run_item (config_id,created_at);
CREATE INDEX IF NOT EXISTS idx_yfs_run_item_status ON hg_youban_feiniu_sync_run_item (config_id,status,created_at);
CREATE INDEX IF NOT EXISTS idx_yfs_run_item_channel ON hg_youban_feiniu_sync_run_item (config_id,feiniu_channel_id,created_at);
`
