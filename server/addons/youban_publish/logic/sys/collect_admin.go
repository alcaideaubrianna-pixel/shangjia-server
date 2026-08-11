package sys

import (
	"context"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) CollectSourceList(ctx context.Context, in *sysin.CollectSourceListInp) (list []*sysin.CollectSourceModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.CollectSourceListInp{}
	}
	mod := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("tenant_id", account.TenantId).
		Where("account_id", account.Id).
		WhereNull("deleted_at")
	if in.SourceType != "" {
		mod = mod.Where("source_type", strings.TrimSpace(in.SourceType))
	}
	if in.Status > 0 {
		mod = mod.Where("status", in.Status)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(title LIKE ? OR source_username LIKE ? OR source_chat_id LIKE ?)", like, like, like)
	}
	if totalCount, err = mod.Count(); err != nil {
		return nil, 0, gerror.Wrap(err, "统计采集源失败")
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取采集源失败")
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		item.HistoryCollectEnabled, item.HistoryCollectMode, item.HistoryCollectDays = sysin.NormalizeCollectHistoryConfig(
			item.SourceType,
			item.HistoryCollectEnabled,
			item.HistoryCollectMode,
			item.HistoryCollectDays,
		)
	}
	if err = s.fillCollectSourceRules(ctx, list); err != nil {
		return nil, 0, err
	}
	if err = s.fillCollectSourceChannelTitles(ctx, list); err != nil {
		return nil, 0, err
	}
	return
}

func (s *sSysPublish) CollectSourceSave(ctx context.Context, in *sysin.CollectSourceSaveInp) (id int64, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return 0, err
	}
	if in == nil {
		return 0, gerror.New("采集源参数不能为空")
	}
	if err = s.ensureTenantVipFeature(ctx, account.TenantId, sysin.TenantVipFeatureCollectSource); err != nil {
		return 0, err
	}
	if err = in.Filter(ctx); err != nil {
		return 0, err
	}
	if in.SourceType == sysin.CollectSourceTypeBot {
		var botExists bool
		botExists, err = g.DB().Model(publishBotTable).Safe().Ctx(ctx).
			Where("id", in.BotId).
			Where("tenant_id", account.TenantId).
			Where("status", 1).
			WhereNull("deleted_at").
			Exist()
		if err != nil {
			return 0, gerror.Wrap(err, "校验Bot采集机器人失败")
		}
		if !botExists {
			return 0, gerror.New("Bot采集机器人不存在、已停用或不属于当前账号")
		}
	}
	now := gtime.Now()
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		data := g.Map{
			"tenant_id":               account.TenantId,
			"account_id":              account.Id,
			"source_type":             in.SourceType,
			"title":                   in.Title,
			"source_chat_id":          in.SourceChatId,
			"source_username":         in.SourceUsername,
			"tg_account_id":           in.TgAccountId,
			"bot_id":                  in.BotId,
			"bot_collect_scope":       in.BotCollectScope,
			"follow_account_id":       in.FollowAccountId,
			"collect_enabled":         in.CollectEnabled,
			"history_collect_enabled": in.HistoryCollectEnabled,
			"history_collect_mode":    in.HistoryCollectMode,
			"history_collect_days":    in.HistoryCollectDays,
			"status":                  in.Status,
			"remark":                  in.Remark,
			"updated_by":              account.Id,
			"updated_at":              now,
		}
		if in.Id > 0 {
			if _, err = tx.Model(pdao.YoubanPublishCollectSource.Table()).Ctx(ctx).
				Where("id", in.Id).
				Where("tenant_id", account.TenantId).
				Where("account_id", account.Id).
				WhereNull("deleted_at").
				Data(data).
				Update(); err != nil {
				return gerror.Wrap(err, "更新采集源失败")
			}
			id = in.Id
		} else {
			data["created_by"] = account.Id
			data["created_at"] = now
			newId, err := tx.Model(pdao.YoubanPublishCollectSource.Table()).Ctx(ctx).Data(data).InsertAndGetId()
			if err != nil {
				return gerror.Wrap(err, "创建采集源失败")
			}
			id = newId
		}
		return s.saveCollectSourceRules(ctx, tx, account.TenantId, id, in.RuleIds)
	})
	if err == nil && id > 0 && in.HistoryCollectEnabled == 1 {
		s.maybeCreateCollectHistoryTask(context.Background(), id, account.TenantId, account.Id)
	}
	if err == nil {
		s.refreshCollectEventRulesCache(ctx)
		s.refreshCollectSourceCache(ctx)
		s.refreshAccountCollectSupervisor()
	}
	return id, err
}

func (s *sSysPublish) CollectSourceDelete(ctx context.Context, in *sysin.IdsInp) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择采集源")
	}
	ids := uniqueIds(in.Ids)
	for _, sourceId := range ids {
		if err = s.cancelCollectSourceRuntime(ctx, sourceId, account.TenantId, account.Id); err != nil {
			return gerror.Wrap(err, "取消采集源任务失败")
		}
		if err = s.clearCollectSourceDedupe(ctx, sourceId, account.TenantId, account.Id); err != nil {
			return gerror.Wrap(err, "清理采集源去重数据失败")
		}
	}
	err = pdao.YoubanPublishCollectSource.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, deleteErr := tx.Model(pdao.YoubanPublishCollectSourceRule.Table()).Ctx(ctx).
			WhereIn("source_id", ids).
			Where("tenant_id", account.TenantId).
			Delete(); deleteErr != nil {
			return gerror.Wrap(deleteErr, "删除采集源规则绑定失败")
		}
		if _, deleteErr := tx.Model(pdao.YoubanPublishCollectSource.Table()).Ctx(ctx).
			WhereIn("id", ids).
			Where("tenant_id", account.TenantId).
			Where("account_id", account.Id).
			Unscoped().
			Delete(); deleteErr != nil {
			return gerror.Wrap(deleteErr, "物理删除采集源失败")
		}
		return nil
	})
	if err == nil {
		s.refreshCollectEventRulesCache(ctx)
		s.refreshCollectSourceCache(ctx)
		s.refreshAccountCollectSupervisor()
	}
	return gerror.Wrap(err, "删除采集源失败")
}

func (s *sSysPublish) CollectSourceStatus(ctx context.Context, in *sysin.CollectStatusInp) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || in.Id <= 0 {
		return gerror.New("采集源ID不能为空")
	}
	if in.Enabled == 1 {
		if err = s.ensureTenantVipFeature(ctx, account.TenantId, sysin.TenantVipFeatureCollectSource); err != nil {
			return err
		}
	}
	data := g.Map{"updated_at": gtime.Now(), "updated_by": account.Id}
	if in.Enabled == 1 {
		data["collect_enabled"] = 1
	} else {
		data["collect_enabled"] = 0
	}
	if in.Status > 0 {
		data["status"] = in.Status
	}
	_, err = pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("id", in.Id).
		Where("tenant_id", account.TenantId).
		Where("account_id", account.Id).
		WhereNull("deleted_at").
		Data(data).
		Update()
	if err == nil {
		s.refreshCollectSourceCache(ctx)
		s.refreshAccountCollectSupervisor()
		if in.Enabled == 1 {
			go s.retryCollectSourceAfterEnabled(context.Background(), in.Id, account.TenantId, account.Id)
		}
	}
	return gerror.Wrap(err, "更新采集源状态失败")
}

func (s *sSysPublish) fillCollectSourceRules(ctx context.Context, list []*sysin.CollectSourceModel) error {
	sourceIds := make([]int64, 0, len(list))
	for _, item := range list {
		if item != nil {
			sourceIds = append(sourceIds, item.Id)
		}
	}
	if len(sourceIds) == 0 {
		return nil
	}
	var rows []struct {
		SourceId int64 `json:"sourceId"`
		RuleId   int64 `json:"ruleId"`
	}
	if err := pdao.YoubanPublishCollectSourceRule.Ctx(ctx).WhereIn("source_id", sourceIds).Where("status", 1).OrderAsc("sort").Scan(&rows); err != nil {
		return gerror.Wrap(err, "读取采集源规则失败")
	}
	ruleMap := map[int64][]int64{}
	for _, row := range rows {
		ruleMap[row.SourceId] = append(ruleMap[row.SourceId], row.RuleId)
	}
	for _, item := range list {
		item.RuleIds = ruleMap[item.Id]
	}
	return nil
}

func (s *sSysPublish) fillCollectSourceChannelTitles(ctx context.Context, list []*sysin.CollectSourceModel) error {
	tgAccountIds := make([]int64, 0, len(list))
	tenantIds := make([]int64, 0, len(list))
	channelIds := make([]string, 0, len(list))
	wanted := map[string]struct{}{}
	for _, item := range list {
		if item == nil || item.SourceType != sysin.CollectSourceTypeAccount || item.TgAccountId <= 0 || strings.TrimSpace(item.SourceChatId) == "" {
			continue
		}
		tenantIds = append(tenantIds, item.TenantId)
		tgAccountIds = append(tgAccountIds, item.TgAccountId)
		for _, channelId := range tgChannelCacheLookupIds(item.SourceChatId) {
			channelIds = append(channelIds, channelId)
			wanted[collectSourceChannelKey(item.TgAccountId, channelId)] = struct{}{}
		}
	}
	tenantIds = uniqueIds(tenantIds)
	tgAccountIds = uniqueIds(tgAccountIds)
	channelIds = uniqueStrings(channelIds)
	if len(tenantIds) == 0 || len(tgAccountIds) == 0 || len(channelIds) == 0 {
		return nil
	}
	var rows []struct {
		TgAccountId     int64  `json:"tgAccountId"`
		ChannelId       string `json:"channelId"`
		ChannelTitle    string `json:"channelTitle"`
		ChannelUsername string `json:"channelUsername"`
	}
	if err := g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
		Fields("tg_account_id,channel_id,channel_title,channel_username").
		WhereIn("tenant_id", tenantIds).
		WhereIn("tg_account_id", tgAccountIds).
		WhereIn("channel_id", channelIds).
		Scan(&rows); err != nil {
		return gerror.Wrap(err, "读取采集源频道缓存失败")
	}
	cacheMap := make(map[string]collectSourceChannelCacheValue, len(rows))
	for _, row := range rows {
		key := collectSourceChannelKey(row.TgAccountId, row.ChannelId)
		if _, ok := wanted[key]; !ok {
			continue
		}
		cacheMap[key] = collectSourceChannelCacheValue{Title: row.ChannelTitle, Username: row.ChannelUsername}
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		if cache, ok := collectSourceChannelCache(item.TgAccountId, item.SourceChatId, cacheMap); ok {
			if strings.TrimSpace(cache.Title) != "" {
				item.Title = cache.Title
			}
			if strings.TrimSpace(cache.Username) != "" {
				item.SourceUsername = cache.Username
			}
		}
	}
	return nil
}

func collectSourceChannelKey(tgAccountId int64, channelId string) string {
	return strconv.FormatInt(tgAccountId, 10) + ":" + strings.TrimSpace(channelId)
}

type collectSourceChannelCacheValue struct {
	Title    string
	Username string
}

func collectSourceChannelCache(tgAccountId int64, channelId string, cacheMap map[string]collectSourceChannelCacheValue) (collectSourceChannelCacheValue, bool) {
	for _, lookupId := range tgChannelCacheLookupIds(channelId) {
		if cache, ok := cacheMap[collectSourceChannelKey(tgAccountId, lookupId)]; ok {
			return cache, true
		}
	}
	return collectSourceChannelCacheValue{}, false
}

func (s *sSysPublish) saveCollectSourceRules(ctx context.Context, tx gdb.TX, tenantId int64, sourceId int64, ruleIds []int64) error {
	if _, err := tx.Model(pdao.YoubanPublishCollectSourceRule.Table()).Ctx(ctx).Where("source_id", sourceId).Delete(); err != nil {
		return gerror.Wrap(err, "清理采集源规则失败")
	}
	now := gtime.Now()
	for index, ruleId := range uniqueIds(ruleIds) {
		if ruleId <= 0 {
			continue
		}
		if _, err := tx.Model(pdao.YoubanPublishCollectSourceRule.Table()).Ctx(ctx).Data(g.Map{
			"tenant_id":  tenantId,
			"source_id":  sourceId,
			"rule_id":    ruleId,
			"sort":       index + 1,
			"status":     1,
			"created_at": now,
			"updated_at": now,
		}).Insert(); err != nil {
			return gerror.Wrap(err, "保存采集源规则失败")
		}
	}
	return nil
}
