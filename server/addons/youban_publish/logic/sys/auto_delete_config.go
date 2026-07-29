package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/entity"
	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
	coredao "hotgo/internal/dao"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/contexts"
)

const (
	autoDeleteDefaultConfigCacheKey = "youban_publish:auto_delete:defaults"
	autoDeleteTenantCacheKeyPrefix  = "youban_publish:auto_delete:tenant"
	autoDeleteConfigCacheTTL        = 10 * time.Minute
	autoDeleteRuleSingleNumberLine  = `single:^编号\s*[:：]\s*[A-Za-z0-9_-]+$`
)

type tenantAutoDeleteConfig struct {
	CustomKeywords []string `json:"customKeywords"`
	CustomRules    []string `json:"customRules"`
	Enabled        int      `json:"enabled"`
	LegacyMigrated bool     `json:"legacyMigrated"`
}

func (s *sSysConfig) AutoDeleteConfigView(ctx context.Context, _ *sysin.AutoDeleteConfigViewInp) (*sysin.AutoDeleteConfigViewModel, error) {
	tenantId, err := s.currentAutoDeleteTenant(ctx)
	if err != nil {
		return nil, err
	}
	res, err := s.AutoDeleteConfigForTenant(ctx, tenantId)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *sSysConfig) AutoDeleteConfigForTenant(ctx context.Context, tenantId int64) (*sysin.AutoDeleteConfigViewModel, error) {
	if tenantId <= 0 {
		return nil, gerror.New("租户ID不能为空")
	}
	defaults, err := s.loadAutoDeleteDefaults(ctx)
	if err != nil {
		return nil, err
	}
	tenant, err := s.loadTenantAutoDeleteConfig(ctx, tenantId, defaults)
	if err != nil {
		return nil, err
	}
	conf := &model.AutoDeleteConfig{
		Enabled:         tenant.Enabled,
		DefaultKeywords: append([]string(nil), defaults.Keywords...),
		CustomKeywords:  append([]string(nil), tenant.CustomKeywords...),
		DefaultRules:    append([]string(nil), defaults.Rules...),
		CustomRules:     append([]string(nil), tenant.CustomRules...),
	}
	conf.Keywords = mergeAutoDeleteStrings(conf.DefaultKeywords, conf.CustomKeywords)
	conf.Rules = mergeAutoDeleteStrings(conf.DefaultRules, conf.CustomRules)
	return &sysin.AutoDeleteConfigViewModel{AutoDeleteConfig: conf}, nil
}

func (s *sSysConfig) AutoDeleteConfigSave(ctx context.Context, in *sysin.AutoDeleteConfigSaveInp) error {
	if in == nil {
		return gerror.New("频道自动删除配置不能为空")
	}
	if err := in.Filter(ctx); err != nil {
		return err
	}
	tenantId, err := s.currentAutoDeleteTenant(ctx)
	if err != nil {
		return err
	}
	columns := pdao.YoubanPublishTenantAutoDeleteConfig.Columns()
	now := gtime.Now()
	data := g.Map{
		columns.TenantId:           tenantId,
		columns.Enabled:            in.Enabled,
		columns.CustomKeywordsJson: mustConfigJSON(in.CustomKeywords),
		columns.CustomRulesJson:    mustConfigJSON(in.CustomRules),
		columns.UpdatedBy:          contexts.GetUserId(ctx),
		columns.UpdatedAt:          now,
		columns.CreatedBy:          contexts.GetUserId(ctx),
		columns.CreatedAt:          now,
	}
	_, err = pdao.YoubanPublishTenantAutoDeleteConfig.Ctx(ctx).
		Data(data).
		OnConflict(columns.TenantId).
		OnDuplicateEx(columns.Id, columns.TenantId, columns.CreatedBy, columns.CreatedAt).
		Save()
	if err != nil {
		return gerror.Wrap(err, "保存消息自动删除配置失败")
	}
	clearTenantAutoDeleteConfigCache(ctx, tenantId)
	return nil
}

func (s *sSysConfig) currentAutoDeleteTenant(ctx context.Context) (int64, error) {
	userId := contexts.GetUserId(ctx)
	if userId <= 0 {
		return 0, gerror.New("请先登录")
	}
	columns := pdao.YoubanPublishAccount.Columns()
	var row struct {
		AccountType string `json:"accountType"`
		TenantId    int64  `json:"tenantId"`
	}
	err := pdao.YoubanPublishAccount.Ctx(ctx).
		Fields(columns.TenantId, columns.AccountType).
		Where(columns.Id, userId).
		Where(columns.Status, 1).
		WhereNull(columns.DeletedAt).
		Scan(&row)
	if err != nil {
		return 0, gerror.Wrap(err, "读取当前账号失败")
	}
	if row.TenantId <= 0 || row.AccountType != sysin.PublishAccountTypeAdmin {
		return 0, gerror.New("当前账号无管理权限")
	}
	return row.TenantId, nil
}

func (s *sSysConfig) loadAutoDeleteDefaults(ctx context.Context) (*model.AutoDeleteConfig, error) {
	cacheVar, cacheErr := cache.Instance().Get(ctx, autoDeleteDefaultConfigCacheKey)
	if cacheErr == nil && !cacheVar.IsNil() {
		var cached model.AutoDeleteConfig
		if cacheErr = cacheVar.Scan(&cached); cacheErr == nil {
			ensureAutoDeleteDefaultRules(&cached)
			return &cached, nil
		}
	}
	conf := defaultAutoDeleteConfig()
	if err := s.scanConfigGroup(ctx, publishConfigGroupAutoDelete, conf); err != nil {
		return nil, err
	}
	if err := loadLegacyAutoDeleteFields(ctx, conf); err != nil {
		return nil, err
	}
	conf.Keywords = mergeAutoDeleteStrings(conf.Keywords)
	ensureAutoDeleteDefaultRules(conf)
	_ = cache.Instance().Set(ctx, autoDeleteDefaultConfigCacheKey, conf, autoDeleteConfigCacheTTL)
	return conf, nil
}

func loadLegacyAutoDeleteFields(ctx context.Context, conf *model.AutoDeleteConfig) error {
	columns := coredao.SysAddonsConfig.Columns()
	var rows []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	err := coredao.SysAddonsConfig.Ctx(ctx).
		Fields(columns.Key, columns.Value).
		Where(columns.AddonName, "youban_publish").
		Where(columns.Group, publishConfigGroupAutoDelete).
		WhereIn(columns.Key, []string{"enabled", "autoDeleteEnabled"}).
		Scan(&rows)
	if err != nil {
		return gerror.Wrap(err, "读取旧消息自动删除配置失败")
	}
	for _, row := range rows {
		switch row.Key {
		case "enabled":
			if conf.Enabled == 0 {
				conf.Enabled, _ = strconv.Atoi(strings.TrimSpace(row.Value))
			}
		case "autoDeleteEnabled":
			conf.Enabled, _ = strconv.Atoi(strings.TrimSpace(row.Value))
		}
	}
	return nil
}

func (s *sSysConfig) loadTenantAutoDeleteConfig(ctx context.Context, tenantId int64, legacy *model.AutoDeleteConfig) (*tenantAutoDeleteConfig, error) {
	key := tenantAutoDeleteConfigCacheKey(tenantId)
	cacheVar, cacheErr := cache.Instance().Get(ctx, key)
	if cacheErr == nil && !cacheVar.IsNil() {
		var cached tenantAutoDeleteConfig
		if cacheErr = cacheVar.Scan(&cached); cacheErr == nil && cached.LegacyMigrated {
			return &cached, nil
		}
	}
	columns := pdao.YoubanPublishTenantAutoDeleteConfig.Columns()
	var row *entity.YoubanPublishTenantAutoDeleteConfig
	if err := pdao.YoubanPublishTenantAutoDeleteConfig.Ctx(ctx).
		Where(columns.TenantId, tenantId).
		Scan(&row); err != nil {
		return nil, gerror.Wrap(err, "读取消息自动删除配置失败")
	}
	if row == nil || row.Id <= 0 {
		return s.migrateLegacyAutoDeleteConfig(ctx, tenantId, legacy)
	}
	conf := tenantAutoDeleteConfig{
		Enabled:        row.Enabled,
		CustomKeywords: decodeAutoDeleteStringJSON(row.CustomKeywordsJson),
		CustomRules:    decodeAutoDeleteStringJSON(row.CustomRulesJson),
		LegacyMigrated: true,
	}
	_ = cache.Instance().Set(ctx, key, conf, autoDeleteConfigCacheTTL)
	return &conf, nil
}

func (s *sSysConfig) migrateLegacyAutoDeleteConfig(ctx context.Context, tenantId int64, legacy *model.AutoDeleteConfig) (*tenantAutoDeleteConfig, error) {
	conf := tenantAutoDeleteConfigFromLegacy(legacy)
	columns := pdao.YoubanPublishTenantAutoDeleteConfig.Columns()
	now := gtime.Now()
	_, err := pdao.YoubanPublishTenantAutoDeleteConfig.Ctx(ctx).Data(g.Map{
		columns.TenantId:           tenantId,
		columns.Enabled:            conf.Enabled,
		columns.CustomKeywordsJson: "[]",
		columns.CustomRulesJson:    "[]",
		columns.CreatedAt:          now,
		columns.UpdatedAt:          now,
	}).OnConflict(columns.TenantId).OnDuplicateEx(columns.Id, columns.TenantId, columns.CreatedAt).Save()
	if err != nil {
		return nil, gerror.Wrap(err, "迁移消息自动删除配置失败")
	}
	_ = cache.Instance().Set(ctx, tenantAutoDeleteConfigCacheKey(tenantId), conf, autoDeleteConfigCacheTTL)
	return conf, nil
}

func tenantAutoDeleteConfigFromLegacy(legacy *model.AutoDeleteConfig) *tenantAutoDeleteConfig {
	conf := &tenantAutoDeleteConfig{
		Enabled:        1,
		LegacyMigrated: true,
	}
	if legacy != nil && legacy.Enabled == 0 {
		conf.Enabled = 0
	}
	return conf
}

func defaultAutoDeleteConfig() *model.AutoDeleteConfig {
	return &model.AutoDeleteConfig{
		Enabled:  1,
		Keywords: []string{},
		Rules:    []string{autoDeleteRuleSingleNumberLine},
	}
}

func ensureAutoDeleteDefaultRules(conf *model.AutoDeleteConfig) {
	if conf == nil {
		return
	}
	conf.Rules = mergeAutoDeleteStrings([]string{autoDeleteRuleSingleNumberLine}, conf.Rules)
}

func mergeAutoDeleteStrings(groups ...[]string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, group := range groups {
		for _, value := range group {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			key := strings.ToLower(value)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, value)
		}
	}
	return result
}

func decodeAutoDeleteStringJSON(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &values); err != nil {
		return []string{}
	}
	return mergeAutoDeleteStrings(values)
}

func tenantAutoDeleteConfigCacheKey(tenantId int64) string {
	return fmt.Sprintf("%s:%d", autoDeleteTenantCacheKeyPrefix, tenantId)
}

func clearTenantAutoDeleteConfigCache(ctx context.Context, tenantId int64) {
	_, _ = cache.Instance().Remove(ctx, tenantAutoDeleteConfigCacheKey(tenantId))
}

func clearAutoDeleteDefaultConfigCache(ctx context.Context) {
	_, _ = cache.Instance().Remove(ctx, autoDeleteDefaultConfigCacheKey)
}
