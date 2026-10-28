package sys

import (
	"context"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/consts"
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
	if err = fillCollectSourceSuccessTotals(ctx, list, account.TenantId, account.Id); err != nil {
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
		bot, botErr := s.getBotById(ctx, in.BotId, account.TenantId)
		if botErr != nil {
			return 0, botErr
		}
		if bot.BotType == consts.PublishBotTypeTwoWay {
			return 0, gerror.New("双向机器人不能用于频道采集，请选择普通机器人")
		}
	}
	now := gtime.Now()
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if uniqueErr := ensureUniqueActiveCollectChannelSource(ctx, tx, account.TenantId, account.Id, in.Id, in.SourceChatId, in.CollectEnabled, in.Status); uniqueErr != nil {
			return uniqueErr
		}
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
		s.refreshCollectEventRulesCache(ctx, account.TenantId, account.Id)
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
	now := gtime.Now()
	err = pdao.YoubanPublishCollectSource.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var boundRuleIds []int64
		if queryErr := tx.Model(pdao.YoubanPublishCollectSourceRule.Table()).Ctx(ctx).
			Fields("rule_id").WhereIn("source_id", ids).Where("tenant_id", account.TenantId).Scan(&boundRuleIds); queryErr != nil {
			return gerror.Wrap(queryErr, "读取采集源专属规则失败")
		}
		if _, updateErr := tx.Model(pdao.YoubanPublishCollectSource.Table()).Ctx(ctx).
			WhereIn("id", ids).
			Where("tenant_id", account.TenantId).
			Where("account_id", account.Id).
			WhereNull("deleted_at").
			Data(g.Map{"collect_enabled": 0, "deleted_at": now, "updated_by": account.Id, "updated_at": now}).Update(); updateErr != nil {
			return gerror.Wrap(updateErr, "标记采集源删除失败")
		}
		if _, deleteErr := tx.Model(pdao.YoubanPublishCollectSourceRule.Table()).Ctx(ctx).
			WhereIn("source_id", ids).
			Where("tenant_id", account.TenantId).
			Delete(); deleteErr != nil {
			return gerror.Wrap(deleteErr, "删除采集源规则绑定失败")
		}
		boundRuleIds = uniqueIds(boundRuleIds)
		if len(boundRuleIds) > 0 {
			if _, updateErr := tx.Model(pdao.YoubanPublishCollectRule.Table()).Ctx(ctx).
				WhereIn("id", boundRuleIds).Where("tenant_id", account.TenantId).Where("account_id", account.Id).
				Where("global_enabled", 0).
				Where("NOT EXISTS (SELECT 1 FROM " + pdao.YoubanPublishCollectSourceRule.Table() + " sr WHERE sr.rule_id=" + pdao.YoubanPublishCollectRule.Table() + ".id)").
				Data(g.Map{"deleted_at": now, "deleted_by": account.Id, "updated_by": account.Id, "updated_at": now}).Update(); updateErr != nil {
				return gerror.Wrap(updateErr, "清理采集源专属规则失败")
			}
		}
		return nil
	})
	if err == nil {
		s.refreshCollectEventRulesCache(ctx, account.TenantId, account.Id)
		s.refreshCollectSourceCache(ctx)
		s.refreshAccountCollectSupervisor()
		for _, sourceId := range ids {
			if enqueueErr := s.enqueueCollectSourceDelete(ctx, collectSourceDeleteQueuePayload{SourceId: sourceId, TenantId: account.TenantId, AccountId: account.Id}); enqueueErr != nil {
				return gerror.Wrap(enqueueErr, "投递采集源删除清理任务失败")
			}
		}
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
	if err := pdao.YoubanPublishCollectSourceRule.Ctx(ctx).WhereIn("source_id", sourceIds).OrderAsc("sort").Scan(&rows); err != nil {
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
		if item == nil || strings.TrimSpace(item.SourceChatId) == "" {
			continue
		}
		tenantIds = append(tenantIds, item.TenantId)
		if item.SourceType == sysin.CollectSourceTypeAccount && item.TgAccountId > 0 {
			tgAccountIds = append(tgAccountIds, item.TgAccountId)
		}
		for _, channelId := range tgChannelCacheLookupIds(item.SourceChatId) {
			channelIds = append(channelIds, channelId)
			if item.TgAccountId > 0 {
				wanted[collectSourceChannelKey(item.TgAccountId, channelId)] = struct{}{}
			}
		}
	}
	tenantIds = uniqueIds(tenantIds)
	tgAccountIds = uniqueIds(tgAccountIds)
	channelIds = uniqueStrings(channelIds)
	if len(tenantIds) == 0 || len(channelIds) == 0 {
		return nil
	}
	var rows []struct {
		TgAccountId     int64  `json:"tgAccountId"`
		ChannelId       string `json:"channelId"`
		ChannelTitle    string `json:"channelTitle"`
		ChannelUsername string `json:"channelUsername"`
	}
	if len(tgAccountIds) > 0 {
		if err := g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
			Fields("tg_account_id,channel_id,channel_title,channel_username").
			WhereIn("tenant_id", tenantIds).
			WhereIn("tg_account_id", tgAccountIds).
			WhereIn("channel_id", channelIds).
			Scan(&rows); err != nil {
			return gerror.Wrap(err, "读取采集源频道缓存失败")
		}
	}
	cacheMap := make(map[string]collectSourceChannelCacheValue, len(rows))
	for _, row := range rows {
		key := collectSourceChannelKey(row.TgAccountId, row.ChannelId)
		if _, ok := wanted[key]; !ok {
			continue
		}
		cacheMap[key] = collectSourceChannelCacheValue{Title: row.ChannelTitle, Username: row.ChannelUsername}
	}
	sharedCache, err := collectSourceSharedChannelCache(ctx, tenantIds, channelIds)
	if err != nil {
		return err
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		cache, ok := collectSourceChannelCache(item.TgAccountId, item.SourceChatId, cacheMap)
		if !ok {
			cache, ok = collectSourceSharedChannel(item.SourceChatId, sharedCache)
		}
		if ok {
			if strings.TrimSpace(cache.Title) != "" && collectSourceTitleNeedsEnrichment(item.Title, item.SourceChatId) {
				item.Title = cache.Title
			}
			if strings.TrimSpace(cache.Username) != "" {
				item.SourceUsername = cache.Username
			}
		}
	}
	return nil
}

func collectSourceTitleNeedsEnrichment(title, channelID string) bool {
	title = strings.TrimSpace(title)
	return title == "" || canonicalCollectSourceChatID(title) == canonicalCollectSourceChatID(channelID)
}

func collectSourceSharedChannelCache(ctx context.Context, tenantIDs []int64, channelIDs []string) (map[string]collectSourceChannelCacheValue, error) {
	result := make(map[string]collectSourceChannelCacheValue)
	var canonicalRows []struct {
		ChannelId       string `json:"channelId"`
		ChannelTitle    string `json:"channelTitle"`
		ChannelUsername string `json:"channelUsername"`
	}
	if err := pdao.YoubanPublishCollectChannel.Ctx(ctx).
		Fields("telegram_chat_id AS channel_id,title AS channel_title,username AS channel_username").
		WhereIn("tenant_id", tenantIDs).WhereIn("telegram_chat_id", channelIDs).WhereNull("deleted_at").
		Scan(&canonicalRows); err != nil {
		return nil, gerror.Wrap(err, "读取通用采集频道失败")
	}
	for _, row := range canonicalRows {
		mergeCollectSourceSharedChannel(result, row.ChannelId, row.ChannelTitle, row.ChannelUsername)
	}
	var botRows []struct {
		ChannelId       string `json:"channelId"`
		ChannelTitle    string `json:"channelTitle"`
		ChannelUsername string `json:"channelUsername"`
	}
	if err := g.DB().Model(publishBotChannelCacheTable).Safe().Ctx(ctx).
		Fields("chat_id AS channel_id,chat_title AS channel_title,chat_username AS channel_username").
		WhereIn("tenant_id", tenantIDs).WhereIn("chat_id", channelIDs).
		Scan(&botRows); err != nil {
		return nil, gerror.Wrap(err, "读取BOT采集频道缓存失败")
	}
	for _, row := range botRows {
		mergeCollectSourceSharedChannel(result, row.ChannelId, row.ChannelTitle, row.ChannelUsername)
	}
	return result, nil
}

func mergeCollectSourceSharedChannel(cache map[string]collectSourceChannelCacheValue, channelID, title, username string) {
	key := canonicalCollectSourceChatID(channelID)
	current := cache[key]
	if strings.TrimSpace(current.Title) == "" {
		current.Title = strings.TrimSpace(title)
	}
	if strings.TrimSpace(current.Username) == "" {
		current.Username = strings.TrimSpace(username)
	}
	cache[key] = current
}

func collectSourceSharedChannel(channelID string, cache map[string]collectSourceChannelCacheValue) (collectSourceChannelCacheValue, bool) {
	value, ok := cache[canonicalCollectSourceChatID(channelID)]
	return value, ok
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
	ruleIds = uniqueIds(ruleIds)
	if len(ruleIds) != 1 || ruleIds[0] <= 0 {
		return gerror.New("每个采集源必须绑定一个专属规则")
	}
	accountId, err := tx.Model(pdao.YoubanPublishCollectSource.Table()).Ctx(ctx).
		Fields("account_id").Where("id", sourceId).Where("tenant_id", tenantId).Value()
	if err != nil {
		return gerror.Wrap(err, "读取采集源账号失败")
	}
	ruleCols := pdao.YoubanPublishCollectRule.Columns()
	rule, err := tx.Model(pdao.YoubanPublishCollectRule.Table()).Ctx(ctx).
		Fields(ruleCols.Id).Where(ruleCols.Id, ruleIds[0]).Where(ruleCols.TenantId, tenantId).Where(ruleCols.AccountId, accountId.Int64()).
		Where(ruleCols.GlobalEnabled, 0).WhereNull(ruleCols.DeletedAt).LockUpdate().One()
	if err != nil {
		return gerror.Wrap(err, "校验采集源专属规则失败")
	}
	if rule.IsEmpty() {
		return gerror.New("采集源只能绑定当前账号的专属规则")
	}
	// A rule is source-specific. Older clients could submit an existing rule ID
	// from the same account, which silently made multiple sources share one rule.
	// Clone the rule (including its items) on conflict so existing clients remain
	// compatible while each source gets an independent rule going forward.
	otherSourceValue, err := tx.Model(pdao.YoubanPublishCollectSourceRule.Table()).Ctx(ctx).
		Fields("source_id").Where("rule_id", ruleIds[0]).
		Where("source_id !=", sourceId).Value()
	if err != nil {
		return gerror.Wrap(err, "检查采集规则是否已被其他来源使用失败")
	}
	otherSourceId := otherSourceValue.Int64()
	if otherSourceId > 0 {
		clonedRuleId, cloneErr := cloneCollectRuleTx(ctx, tx, tenantId, accountId.Int64(), ruleIds[0], sourceId)
		if cloneErr != nil {
			return cloneErr
		}
		ruleIds[0] = clonedRuleId
	}
	if _, err := tx.Model(pdao.YoubanPublishCollectSourceRule.Table()).Ctx(ctx).Where("source_id", sourceId).Delete(); err != nil {
		return gerror.Wrap(err, "清理采集源规则失败")
	}
	now := gtime.Now()
	for index, ruleId := range ruleIds {
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

func cloneCollectRuleTx(ctx context.Context, tx gdb.TX, tenantId, accountId, ruleId, sourceId int64) (int64, error) {
	rule, err := tx.Model(pdao.YoubanPublishCollectRule.Table()).Ctx(ctx).
		Where("id", ruleId).Where("tenant_id", tenantId).Where("account_id", accountId).
		WhereNull("deleted_at").One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取待复制采集规则失败")
	}
	if rule.IsEmpty() {
		return 0, gerror.New("待复制采集规则不存在")
	}
	data := g.Map{}
	for key, value := range rule {
		if key == "id" || key == "created_at" || key == "updated_at" || key == "deleted_at" || key == "deleted_by" {
			continue
		}
		data[key] = value
	}
	data["name"] = rule["name"].String() + " · 来源 " + strconv.FormatInt(sourceId, 10)
	data["tenant_id"] = tenantId
	data["account_id"] = accountId
	data["global_enabled"] = 0
	data["status"] = 1
	now := gtime.Now()
	data["created_by"] = accountId
	data["updated_by"] = accountId
	data["created_at"] = now
	data["updated_at"] = now
	newRuleId, err := tx.Model(pdao.YoubanPublishCollectRule.Table()).Ctx(ctx).Data(data).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "复制采集规则失败")
	}
	items, err := tx.Model(collectRuleItemTable).Ctx(ctx).
		Where("rule_id", ruleId).OrderAsc("sort").OrderAsc("id").All()
	if err != nil {
		return 0, gerror.Wrap(err, "读取采集规则项失败")
	}
	for _, item := range items {
		itemData := g.Map{}
		for key, value := range item {
			if key == "id" || key == "created_at" || key == "updated_at" {
				continue
			}
			itemData[key] = value
		}
		itemData["rule_id"] = newRuleId
		itemData["tenant_id"] = tenantId
		itemData["account_id"] = accountId
		itemData["created_at"] = now
		itemData["updated_at"] = now
		if _, err = tx.Model(collectRuleItemTable).Ctx(ctx).Data(itemData).Insert(); err != nil {
			return 0, gerror.Wrap(err, "复制采集规则项失败")
		}
	}
	return newRuleId, nil
}
