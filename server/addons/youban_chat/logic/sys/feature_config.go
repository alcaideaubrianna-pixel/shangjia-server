package sys

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_chat/model/input/sysin"
	"hotgo/internal/consts"
)

const telegramFeatureCacheTTL = 30 * time.Second
const telegramFeatureDefaultSyncTTL = 5 * time.Minute

func (s *sSysChat) AdminFeatureList(ctx context.Context, in *sysin.AdminChatFeatureListInp) (list []*sysin.AdminChatFeatureModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.AdminChatFeatureListInp{}
	}
	if err = s.ensureTelegramFeatureTable(ctx); err != nil {
		return nil, 0, err
	}
	if err = s.syncTelegramFeatureDefaults(ctx); err != nil {
		return nil, 0, err
	}
	mod := g.DB().Model(chatFeatureTable).Ctx(ctx).WhereNull("deleted_at")
	if in.Status > 0 {
		mod = mod.Where("status", in.Status)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(feature_key LIKE ? OR name LIKE ? OR command LIKE ? OR description LIKE ?)", like, like, like, like)
	}
	var rows []*chatFeatureRow
	if err = mod.Page(in.Page, in.PerPage).OrderAsc("sort").OrderAsc("id").ScanAndCount(&rows, &totalCount, false); err != nil {
		if isMissingTableError(err) {
			return []*sysin.AdminChatFeatureModel{}, 0, nil
		}
		return nil, 0, gerror.Wrap(err, "获取功能配置列表失败")
	}
	list = make([]*sysin.AdminChatFeatureModel, 0, len(rows))
	for _, row := range rows {
		item := &sysin.AdminChatFeatureModel{Id: row.Id, FeatureKey: row.FeatureKey, Name: row.Name, Command: row.Command, Description: row.Description, ConfigJson: row.ConfigJson, Sort: row.Sort, Status: row.Status}
		if row.CreatedAt != nil {
			item.CreatedAt = row.CreatedAt.String()
		}
		if row.UpdatedAt != nil {
			item.UpdatedAt = row.UpdatedAt.String()
		}
		list = append(list, item)
	}
	return
}

func (s *sSysChat) AdminSaveFeature(ctx context.Context, in *sysin.AdminChatFeatureSaveInp) (err error) {
	if in == nil {
		return gerror.New("功能配置不能为空")
	}
	if err = s.ensureTelegramFeatureTable(ctx); err != nil {
		return err
	}
	if err = s.syncTelegramFeatureDefaults(ctx); err != nil {
		return err
	}
	featureKey := strings.TrimSpace(in.FeatureKey)
	if featureKey == "" {
		return gerror.New("功能Key不能为空")
	}
	if strings.TrimSpace(in.ConfigJson) != "" && !json.Valid([]byte(strings.TrimSpace(in.ConfigJson))) {
		return gerror.New("配置JSON格式不正确")
	}
	status := in.Status
	if status == 0 {
		status = 1
	}
	data := g.Map{
		"name":        strings.TrimSpace(in.Name),
		"command":     strings.TrimPrefix(strings.TrimSpace(in.Command), "/"),
		"description": strings.TrimSpace(in.Description),
		"config_json": strings.TrimSpace(in.ConfigJson),
		"sort":        in.Sort,
		"status":      status,
		"updated_at":  gtime.Now(),
	}
	if in.Id > 0 {
		_, err = g.DB().Model(chatFeatureTable).Ctx(ctx).Where("id", in.Id).Where("feature_key", featureKey).Data(data).Update()
	} else {
		data["feature_key"] = featureKey
		data["created_at"] = gtime.Now()
		_, err = g.DB().Model(chatFeatureTable).Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存功能配置失败")
	}
	s.clearTelegramFeatureCache()
	if err = s.syncAllTelegramBotMenus(ctx); err != nil {
		g.Log().Warningf(ctx, "同步Telegram菜单失败 err:%+v", err)
	}
	return nil
}

func (s *sSysChat) syncTelegramFeatureDefaults(ctx context.Context) error {
	if err := s.ensureTelegramFeatureTable(ctx); err != nil {
		return err
	}
	if s.telegramFeatureDefaultsSynced() {
		return nil
	}
	changed := false
	for index, feature := range telegramFeatures {
		if feature == nil || strings.TrimSpace(feature.Key()) == "" {
			continue
		}
		var row *chatFeatureRow
		err := g.DB().Model(chatFeatureTable).Ctx(ctx).
			Fields("id").
			Where("feature_key", feature.Key()).
			WhereNull("deleted_at").
			Scan(&row)
		if err != nil {
			return gerror.Wrap(err, "同步功能默认配置失败")
		}
		if row != nil {
			continue
		}
		_, err = g.DB().Model(chatFeatureTable).Ctx(ctx).Data(g.Map{
			"feature_key": feature.Key(),
			"name":        feature.Description(),
			"command":     strings.TrimPrefix(feature.Command(), "/"),
			"description": feature.Description(),
			"config_json": "{}",
			"sort":        (index + 1) * 10,
			"status":      1,
			"created_at":  gtime.Now(),
			"updated_at":  gtime.Now(),
		}).Insert()
		if err != nil {
			return gerror.Wrap(err, "写入功能默认配置失败")
		}
		changed = true
	}
	if changed {
		s.clearTelegramFeatureCache()
	}
	s.markTelegramFeatureDefaultsSynced()
	return nil
}

func (s *sSysChat) telegramFeatureDefaultsSynced() bool {
	s.telegramFeatureMu.RLock()
	defer s.telegramFeatureMu.RUnlock()
	return !s.telegramFeatureDefaultsAt.IsZero() && time.Since(s.telegramFeatureDefaultsAt) < telegramFeatureDefaultSyncTTL
}

func (s *sSysChat) markTelegramFeatureDefaultsSynced() {
	s.telegramFeatureMu.Lock()
	defer s.telegramFeatureMu.Unlock()
	s.telegramFeatureDefaultsAt = time.Now()
}

func (s *sSysChat) telegramFeatureConfig(ctx context.Context, key string) (*chatFeatureRow, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, false
	}
	configs, err := s.telegramFeatureConfigs(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "读取Telegram功能配置失败 err:%+v", err)
		return nil, true
	}
	row, ok := configs[key]
	if !ok || row == nil {
		return nil, true
	}
	return row, row.Status == 1
}

func (s *sSysChat) telegramFeatureConfigs(ctx context.Context) (map[string]*chatFeatureRow, error) {
	if err := s.ensureTelegramFeatureTable(ctx); err != nil {
		return nil, err
	}
	if err := s.syncTelegramFeatureDefaults(ctx); err != nil {
		return nil, err
	}
	s.telegramFeatureMu.RLock()
	if s.telegramFeatures != nil && time.Since(s.telegramFeatureAt) < telegramFeatureCacheTTL {
		defer s.telegramFeatureMu.RUnlock()
		return s.telegramFeatures, nil
	}
	s.telegramFeatureMu.RUnlock()

	s.telegramFeatureMu.Lock()
	defer s.telegramFeatureMu.Unlock()
	if s.telegramFeatures != nil && time.Since(s.telegramFeatureAt) < telegramFeatureCacheTTL {
		return s.telegramFeatures, nil
	}
	var rows []*chatFeatureRow
	err := g.DB().Model(chatFeatureTable).Ctx(ctx).
		WhereNull("deleted_at").
		OrderAsc("sort").
		OrderAsc("id").
		Scan(&rows)
	if err != nil {
		if isMissingTableError(err) {
			return map[string]*chatFeatureRow{}, nil
		}
		return nil, gerror.Wrap(err, "读取功能配置失败")
	}
	configs := make(map[string]*chatFeatureRow, len(rows))
	for _, row := range rows {
		if row != nil && strings.TrimSpace(row.FeatureKey) != "" {
			configs[row.FeatureKey] = row
		}
	}
	s.telegramFeatures = configs
	s.telegramFeatureAt = time.Now()
	return configs, nil
}

func (s *sSysChat) clearTelegramFeatureCache() {
	s.telegramFeatureMu.Lock()
	defer s.telegramFeatureMu.Unlock()
	s.telegramFeatures = nil
	s.telegramFeatureAt = time.Time{}
}

func (s *sSysChat) syncAllTelegramBotMenus(ctx context.Context) error {
	bots, err := s.enabledBots(ctx)
	if err != nil {
		return err
	}
	for _, bot := range bots {
		if bot == nil || strings.TrimSpace(bot.BotToken) == "" {
			continue
		}
		if err = s.syncTelegramBotMenu(ctx, bot.BotToken); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysChat) ensureTelegramFeatureTable(ctx context.Context) error {
	sqlList := []string{`
CREATE TABLE IF NOT EXISTS hg_youban_chat_feature (
  id bigserial PRIMARY KEY,
  feature_key varchar(64) NOT NULL DEFAULT '',
  name varchar(128) NOT NULL DEFAULT '',
  command varchar(64) NOT NULL DEFAULT '',
  description varchar(255) NOT NULL DEFAULT '',
  config_json text,
  sort integer NOT NULL DEFAULT 0,
  status smallint NOT NULL DEFAULT 1,
  created_at timestamp,
  updated_at timestamp,
  deleted_at timestamp
);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_ybcf_feature_key ON hg_youban_chat_feature (feature_key);`,
		`CREATE INDEX IF NOT EXISTS idx_ybcf_status_sort ON hg_youban_chat_feature (status, sort);`,
	}
	if strings.ToLower(g.DB().GetConfig().Type) != consts.DBPgsql {
		sqlList = []string{`
CREATE TABLE IF NOT EXISTS hg_youban_chat_feature (
  id bigint(20) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  feature_key varchar(64) NOT NULL DEFAULT '' COMMENT '功能Key',
  name varchar(128) NOT NULL DEFAULT '' COMMENT '功能名称',
  command varchar(64) NOT NULL DEFAULT '' COMMENT 'Telegram命令',
  description varchar(255) NOT NULL DEFAULT '' COMMENT '功能描述',
  config_json text COMMENT '配置JSON',
  sort int(11) NOT NULL DEFAULT '0' COMMENT '排序',
  status tinyint(4) NOT NULL DEFAULT '1' COMMENT '状态',
  created_at datetime DEFAULT NULL COMMENT '创建时间',
  updated_at datetime DEFAULT NULL COMMENT '更新时间',
  deleted_at datetime DEFAULT NULL COMMENT '删除时间',
  PRIMARY KEY (id),
  UNIQUE KEY uk_ybcf_feature_key (feature_key),
  KEY idx_ybcf_status_sort (status, sort)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴聊天_Telegram功能配置';
`}
	}
	for _, item := range sqlList {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, err := g.DB().Exec(ctx, item); err != nil {
			return gerror.Wrap(err, "初始化功能配置表失败")
		}
	}
	return nil
}
