package sys

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	pdao "hotgo/addons/youban_publish/internal/dao"
	pdo "hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
	"time"
)

const introFeeChannelRuleTable = "hg_youban_publish_intro_fee_channel_rule"
const introFeeChannelRuleCacheTTL = 10 * time.Minute
const introFeeChannelCandidateScanLimit = 2000

func introFeeChannelRuleCacheKey(tenant, account, rule, bot int64, channel string) string {
	return fmt.Sprintf("youban_publish:intro_fee_rule:v2:%d:%d:%d:%d:%s", tenant, account, rule, bot, channel)
}

func (s *sSysPublish) resolveIntroFeeChannelSuffix(ctx context.Context, event gdb.Record, rule gdb.Record) (string, bool, error) {
	chatID := canonicalCollectSourceChatID(event["source_chat_id"].String())
	if chatID == "" {
		return "", false, nil
	}
	botID := event["bot_id"].Int64()
	if botID == 0 {
		botID = rule["bot_id"].Int64()
	}
	accountID := event["account_id"].Int64()
	key := introFeeChannelRuleCacheKey(event["tenant_id"].Int64(), accountID, rule["id"].Int64(), botID, chatID)
	if value, cacheErr := cache.Instance().Get(ctx, key); cacheErr == nil && !value.IsNil() {
		if value.String() == "__none__" {
			return "", false, nil
		}
		return value.String(), true, nil
	}
	mod := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).
		Fields("suffix").Where("tenant_id", event["tenant_id"]).WhereIn("account_id", []int64{accountID, 0}).Where("rule_id", rule["id"]).WhereIn("channel_id", tgChannelCacheLookupIds(chatID)).Where("status", 1).WhereNull("deleted_at")
	if botID > 0 {
		// A rule-scoped suffix may be global (bot_id=0), BOT-scoped, or
		// account-scoped. Keep all applicable scopes and prefer the narrowest
		// one through the ordering below.
		mod = mod.WhereIn("bot_id", []int64{botID, 0})
	} else {
		mod = mod.Where("bot_id", 0)
	}
	row, err := mod.OrderDesc("account_id").OrderDesc("bot_id").OrderDesc("id").Limit(1).One()
	if err != nil {
		return "", false, gerror.Wrap(err, "读取介绍费频道后缀规则失败")
	}
	if row.IsEmpty() && botID > 0 {
		row, err = g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Fields("suffix").Where("tenant_id", event["tenant_id"]).WhereIn("account_id", []int64{accountID, 0}).Where("rule_id", 0).Where("bot_id", botID).WhereIn("channel_id", tgChannelCacheLookupIds(chatID)).Where("status", 1).WhereNull("deleted_at").OrderDesc("account_id").OrderDesc("id").Limit(1).One()
	}
	if row.IsEmpty() {
		_ = cache.Instance().Set(ctx, key, "__none__", time.Minute)
		return "", false, nil
	}
	suffix := strings.TrimSpace(row["suffix"].String())
	if suffix == "" {
		return "", false, nil
	}
	_ = cache.Instance().Set(ctx, key, suffix, introFeeChannelRuleCacheTTL)
	return suffix, true, nil
}

func (s *sSysPublish) IntroFeeChannelRuleList(ctx context.Context, in *sysin.IntroFeeChannelRuleListInp) (list []*sysin.IntroFeeChannelRuleItem, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.IntroFeeChannelRuleListInp{}
	}
	if !s.introFeeRuleOwned(ctx, account.TenantId, account.Id, in.RuleId) {
		return nil, 0, gerror.New("介绍费规则不存在或无权访问")
	}
	mod := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("tenant_id", account.TenantId).Where("account_id", account.Id).Where("rule_id", in.RuleId).Where("bot_id", in.BotId).WhereNull("deleted_at")
	if in.IncludeCandidates {
		return s.introFeeChannelRuleCandidates(ctx, account.TenantId, account.Id, in.RuleId, in.BotId, in.Keyword, in.Page, in.PerPage)
	}
	if kw := strings.TrimSpace(in.Keyword); kw != "" {
		like := "%" + kw + "%"
		mod = mod.Where("(channel_id LIKE ? OR channel_title LIKE ? OR channel_username LIKE ? OR suffix LIKE ?)", like, like, like, like)
	}
	err = mod.Page(in.Page, in.PerPage).OrderDesc("updated_at").ScanAndCount(&list, &totalCount, false)
	s.fillIntroFeeChannelBlacklist(ctx, account.TenantId, account.Id, list)
	return
}

func (s *sSysPublish) fillIntroFeeChannelBlacklist(ctx context.Context, tenantID, accountID int64, list []*sysin.IntroFeeChannelRuleItem) {
	for _, row := range list {
		if row == nil || row.ChannelId == "" {
			continue
		}
		mod := g.DB().Model(publishCollectSourceTable).Ctx(ctx).Where("tenant_id", tenantID).WhereIn("source_chat_id", tgChannelCacheLookupIds(row.ChannelId))
		if accountID > 0 {
			mod = mod.Where("account_id", accountID)
		}
		if row.BotId > 0 {
			mod = mod.Where("bot_id", row.BotId)
		}
		var value int
		if err := mod.Fields("channel_blacklisted").OrderDesc("id").Limit(1).Scan(&value); err == nil {
			row.Blacklisted = value != 0
		}
	}
}

// introFeeChannelRuleCandidates combines configured rules with channels already
// seen by this account/bot. It intentionally reads the compact source/cache
// tables instead of scanning the message or event tables.
func (s *sSysPublish) introFeeChannelRuleCandidates(ctx context.Context, tenantID, accountID, ruleID, botID int64, keyword string, page, perPage int) ([]*sysin.IntroFeeChannelRuleItem, int, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 || perPage > 100 {
		perPage = 20
	}
	configured := make([]*sysin.IntroFeeChannelRuleItem, 0)
	configuredModel := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).
		Where("tenant_id", tenantID).WhereIn("account_id", []int64{accountID, 0}).
		Where("rule_id", ruleID).WhereNull("deleted_at")
	if botID > 0 {
		configuredModel = configuredModel.WhereIn("bot_id", []int64{botID, 0})
	} else {
		configuredModel = configuredModel.Where("bot_id", 0)
	}
	if err := configuredModel.OrderDesc("account_id").OrderDesc("bot_id").OrderDesc("updated_at").Limit(introFeeChannelCandidateScanLimit).Scan(&configured); err != nil {
		return nil, 0, err
	}
	seen := make(map[string]struct{}, len(configured))
	deduplicated := configured[:0]
	for _, row := range configured {
		channelID := canonicalCollectSourceChatID(row.ChannelId)
		if _, exists := seen[channelID]; exists {
			continue
		}
		row.Configured = true
		seen[channelID] = struct{}{}
		deduplicated = append(deduplicated, row)
	}
	configured = deduplicated
	// A rule-scoped dialog must only contain channels belonging to sources
	// bound to this rule. The compact source/source_rule tables are the
	// historical index; do not broaden the result with all account/BOT chats.
	type sourceCandidate struct {
		BotId        int64       `orm:"bot_id"`
		ChatId       string      `orm:"source_chat_id"`
		ChatTitle    string      `orm:"chat_title"`
		ChatUsername string      `orm:"chat_username"`
		LastSeenAt   *gtime.Time `orm:"last_message_at"`
	}
	sourceModel := g.DB().Model(publishCollectSourceTable).Ctx(ctx).
		Where("tenant_id", tenantID).WhereNull("deleted_at")
	if accountID > 0 {
		sourceModel = sourceModel.Where("account_id", accountID)
	}
	if botID > 0 {
		sourceModel = sourceModel.Where("bot_id", botID)
	}
	if ruleID > 0 {
		sourceModel = sourceModel.Where("EXISTS (SELECT 1 FROM "+pdao.YoubanPublishCollectSourceRule.Table()+" sr WHERE sr.source_id="+publishCollectSourceTable+".id AND sr.rule_id=? AND sr.tenant_id=? AND sr.status=1)", ruleID, tenantID)
	}
	var sourceRows []sourceCandidate
	if err := sourceModel.Fields("bot_id,source_chat_id,title AS chat_title,source_username AS chat_username,last_event_at AS last_message_at").OrderDesc("updated_at").Limit(introFeeChannelCandidateScanLimit).Scan(&sourceRows); err != nil {
		return nil, 0, err
	}
	// Aggregate BOT sources have no source_chat_id. Their compact
	// channel/profile index replaces the former collect_event history scan.
	var indexedRows []struct {
		ChatId       string      `orm:"channel_id"`
		ProfileCount int         `orm:"profile_count"`
		LastSeenAt   *gtime.Time `orm:"last_seen_at"`
	}
	if err := g.DB().Model(publishCollectChannelProfileTable).Ctx(ctx).
		Fields("channel_id,COUNT(DISTINCT profile_id) AS profile_count,MAX(updated_at) AS last_seen_at").
		Where("tenant_id", tenantID).Where("account_id", accountID).Where("rule_id", ruleID).WhereNull("deleted_at").
		Group("channel_id").OrderDesc("last_seen_at").Limit(introFeeChannelCandidateScanLimit).Scan(&indexedRows); err != nil {
		return nil, 0, gerror.Wrap(err, "读取频道资料索引失败")
	}
	profileCountByChannel := make(map[string]int, len(indexedRows))
	for _, indexed := range indexedRows {
		id := canonicalCollectSourceChatID(indexed.ChatId)
		profileCountByChannel[id] += indexed.ProfileCount
		sourceRows = append(sourceRows, sourceCandidate{ChatId: id, LastSeenAt: indexed.LastSeenAt})
	}
	scopedChatIDs := make([]string, 0, len(sourceRows)*2)
	boundBotIDs := make([]int64, 0, len(sourceRows))
	aggregateBotSource := false
	for _, source := range sourceRows {
		scopedChatIDs = append(scopedChatIDs, tgChannelCacheLookupIds(source.ChatId)...)
		if source.BotId > 0 {
			boundBotIDs = append(boundBotIDs, source.BotId)
			if strings.TrimSpace(source.ChatId) == "" {
				aggregateBotSource = true
			}
		}
	}
	// Configured rows are always in scope. Aggregate BOT sources intentionally
	// have no source_chat_id, so their configured channel IDs must seed the
	// bounded cache lookup used to recover historical titles and usernames.
	for _, row := range configured {
		if row != nil {
			scopedChatIDs = append(scopedChatIDs, tgChannelCacheLookupIds(row.ChannelId)...)
		}
	}
	scopedChatIDs = uniqueStrings(scopedChatIDs)
	cacheModel := g.DB().Model(publishBotChannelCacheTable).Ctx(ctx).Where("tenant_id", tenantID)
	effectiveBotID := botID
	boundBotIDs = uniqueIds(boundBotIDs)
	if effectiveBotID == 0 && len(boundBotIDs) == 1 {
		effectiveBotID = boundBotIDs[0]
	}
	if botID > 0 {
		cacheModel = cacheModel.Where("bot_id", botID)
	} else if len(boundBotIDs) > 0 {
		cacheModel = cacheModel.WhereIn("bot_id", boundBotIDs)
	}
	cacheModel = cacheModel.WhereGT("message_count", 0).WhereNot("chat_type", "private")
	if aggregateBotSource {
		// Aggregate BOT sources own every channel/group that has sent this BOT
		// a message. The BOT cache is the compact history index for this list.
	} else if len(scopedChatIDs) > 0 {
		cacheModel = cacheModel.WhereIn("chat_id", scopedChatIDs)
	} else if ruleID > 0 {
		// No source is bound to this rule; configured rows above are still
		// returned, but unrelated historical chats must not leak in.
		cacheModel = cacheModel.Where("1=0")
	} else if botID == 0 {
		cacheModel = cacheModel.WhereIn("chat_id", g.DB().Model(publishCollectSourceTable).Ctx(ctx).Fields("source_chat_id").Where("tenant_id", tenantID).Where("account_id", accountID).WhereNull("deleted_at"))
	}
	var candidates []struct {
		ChatId       string      `orm:"chat_id"`
		ChatTitle    string      `orm:"chat_title"`
		ChatUsername string      `orm:"chat_username"`
		LastSeenAt   *gtime.Time `orm:"last_message_at"`
	}
	if err := cacheModel.Fields("chat_id,chat_title,chat_username,last_message_at").OrderDesc("last_message_at").Limit(introFeeChannelCandidateScanLimit).Scan(&candidates); err != nil {
		return nil, 0, err
	}
	var canonicalCandidates []struct {
		ChatId       string      `orm:"chat_id"`
		ChatTitle    string      `orm:"chat_title"`
		ChatUsername string      `orm:"chat_username"`
		LastSeenAt   *gtime.Time `orm:"last_message_at"`
	}
	canonicalModel := g.DB().Model(pdao.YoubanPublishCollectChannel.Table()).Ctx(ctx).
		Where("hg_youban_publish_collect_channel.tenant_id", tenantID).
		Where("hg_youban_publish_collect_channel.status", "active").WhereNull("hg_youban_publish_collect_channel.deleted_at")
	if effectiveBotID > 0 {
		canonicalModel = canonicalModel.InnerJoin(pdao.YoubanPublishCollectBotChannel.Table()+" bc", "bc.channel_id=hg_youban_publish_collect_channel.id AND bc.tenant_id=hg_youban_publish_collect_channel.tenant_id").
			Where("bc.bot_id", effectiveBotID).Where("bc.status", "!=", sysin.CollectBotChannelRemoved).WhereNull("bc.deleted_at")
	} else if accountID > 0 {
		canonicalModel = canonicalModel.InnerJoin(pdao.YoubanPublishCollectAccountChannel.Table()+" ac", "ac.channel_id=hg_youban_publish_collect_channel.id AND ac.tenant_id=hg_youban_publish_collect_channel.tenant_id").
			Where("ac.account_id", accountID).WhereNull("ac.deleted_at")
	}
	if aggregateBotSource {
		// The BOT/channel relation above provides the bounded candidate set.
	} else if len(scopedChatIDs) > 0 {
		canonicalModel = canonicalModel.WhereIn("hg_youban_publish_collect_channel.telegram_chat_id", scopedChatIDs)
	} else if ruleID > 0 {
		canonicalModel = canonicalModel.Where("1=0")
	}
	if err := canonicalModel.
		Fields("telegram_chat_id AS chat_id,title AS chat_title,username AS chat_username,last_seen_at AS last_message_at").
		OrderDesc("last_seen_at").Limit(introFeeChannelCandidateScanLimit).Scan(&canonicalCandidates); err == nil {
		candidates = append(candidates, canonicalCandidates...)
	}
	// Collection sources are the compact historical index. Never scan the event
	// or message tables when opening this management dialog.
	for _, sourceCandidate := range sourceRows {
		id := canonicalCollectSourceChatID(sourceCandidate.ChatId)
		if id == "" {
			continue
		}
		candidates = append(candidates, struct {
			ChatId       string      `orm:"chat_id"`
			ChatTitle    string      `orm:"chat_title"`
			ChatUsername string      `orm:"chat_username"`
			LastSeenAt   *gtime.Time `orm:"last_message_at"`
		}{ChatId: id, ChatTitle: sourceCandidate.ChatTitle, ChatUsername: sourceCandidate.ChatUsername, LastSeenAt: sourceCandidate.LastSeenAt})
	}
	for _, candidate := range candidates {
		id := canonicalCollectSourceChatID(candidate.ChatId)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			for _, configuredRow := range configured {
				if configuredRow != nil && canonicalCollectSourceChatID(configuredRow.ChannelId) == id {
					if configuredRow.LastSeenAt == nil || candidate.LastSeenAt != nil && candidate.LastSeenAt.After(configuredRow.LastSeenAt) {
						configuredRow.LastSeenAt = candidate.LastSeenAt
					}
					if strings.TrimSpace(configuredRow.ChannelTitle) == "" {
						configuredRow.ChannelTitle = candidate.ChatTitle
					}
					if strings.TrimSpace(configuredRow.ChannelUsername) == "" {
						configuredRow.ChannelUsername = candidate.ChatUsername
					}
					break
				}
			}
			continue
		}
		seen[id] = struct{}{}
		configured = append(configured, &sysin.IntroFeeChannelRuleItem{RuleId: ruleID, BotId: botID, ChannelId: id, ChannelTitle: candidate.ChatTitle, ChannelUsername: candidate.ChatUsername, LastSeenAt: candidate.LastSeenAt, Configured: false})
	}
	// Enrich all rows in three bounded queries. The dialog must not issue one
	// query per channel because historical BOT lists can be large.
	lookupIDs := make([]string, 0, len(configured)*2)
	for _, row := range configured {
		if row != nil {
			lookupIDs = append(lookupIDs, tgChannelCacheLookupIds(row.ChannelId)...)
		}
	}
	type channelInfo struct {
		Id             int64  `json:"id"`
		TelegramChatId string `json:"telegramChatId"`
		Status         string `json:"status"`
		Title          string `json:"title"`
		Username       string `json:"username"`
		InviteLink     string `json:"inviteLink"`
	}
	var channels []channelInfo
	channelByID := make(map[string]channelInfo, len(configured))
	channelPKs := make([]int64, 0, len(configured))
	if len(lookupIDs) > 0 {
		_ = pdao.YoubanPublishCollectChannel.Ctx(ctx).
			Fields("id,telegram_chat_id,status,title,username,invite_link").
			Where("tenant_id", tenantID).WhereIn("telegram_chat_id", uniqueStrings(lookupIDs)).WhereNull("deleted_at").Scan(&channels)
		for _, channel := range channels {
			key := canonicalCollectSourceChatID(channel.TelegramChatId)
			current, exists := channelByID[key]
			if !exists || introFeeChannelDisplayScore(channel.Title, channel.Username, channel.InviteLink) > introFeeChannelDisplayScore(current.Title, current.Username, current.InviteLink) {
				channelByID[key] = channel
			}
			channelPKs = append(channelPKs, channel.Id)
		}
	}
	botStatusByChannel := make(map[int64]string, len(channels))
	accountStatusByChannel := make(map[int64]string, len(channels))
	if len(channelPKs) > 0 && effectiveBotID > 0 {
		var relations []struct {
			ChannelId int64  `json:"channelId"`
			Status    string `json:"status"`
		}
		_ = pdao.YoubanPublishCollectBotChannel.Ctx(ctx).Fields("channel_id,status").Where("tenant_id", tenantID).Where("bot_id", effectiveBotID).WhereIn("channel_id", channelPKs).Scan(&relations)
		for _, relation := range relations {
			botStatusByChannel[relation.ChannelId] = relation.Status
		}
	}
	if len(channelPKs) > 0 && accountID > 0 {
		var relations []struct {
			ChannelId int64  `json:"channelId"`
			Status    string `json:"status"`
		}
		_ = pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).Fields("channel_id,status").Where("tenant_id", tenantID).Where("account_id", accountID).WhereIn("channel_id", channelPKs).Scan(&relations)
		for _, relation := range relations {
			accountStatusByChannel[relation.ChannelId] = relation.Status
		}
	}
	for _, row := range configured {
		if row == nil || row.ChannelId == "" {
			continue
		}
		if channel, ok := channelByID[canonicalCollectSourceChatID(row.ChannelId)]; ok {
			if strings.TrimSpace(row.ChannelTitle) == "" {
				row.ChannelTitle = strings.TrimSpace(channel.Title)
			}
			if strings.TrimSpace(row.ChannelUsername) == "" {
				row.ChannelUsername = strings.TrimSpace(channel.Username)
			}
			if strings.TrimSpace(channel.InviteLink) != "" {
				row.ChannelLink = strings.TrimSpace(channel.InviteLink)
			}
			row.ChannelStatus = channel.Status
			row.BotStatus = botStatusByChannel[channel.Id]
			row.AccountStatus = accountStatusByChannel[channel.Id]
			if row.AccountStatus == sysin.CollectAccountChannelDisabled {
				row.Blacklisted = true
			}
		}
		if row.ChannelLink == "" && row.ChannelUsername != "" {
			row.ChannelLink = "https://t.me/" + strings.TrimPrefix(row.ChannelUsername, "@")
		}
		row.ProfileCount = profileCountByChannel[canonicalCollectSourceChatID(row.ChannelId)]
	}
	s.fillIntroFeeChannelBlacklist(ctx, tenantID, accountID, configured)
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		filtered := configured[:0]
		lower := strings.ToLower(keyword)
		for _, row := range configured {
			if strings.Contains(strings.ToLower(row.ChannelId+row.ChannelTitle+row.ChannelUsername+row.Suffix), lower) {
				filtered = append(filtered, row)
			}
		}
		configured = filtered
	}
	sort.SliceStable(configured, func(i, j int) bool {
		left, right := configured[i], configured[j]
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		if left.Configured != right.Configured {
			return left.Configured
		}
		if left.Configured {
			if left.UpdatedAt == nil {
				return false
			}
			if right.UpdatedAt == nil {
				return true
			}
			if !left.UpdatedAt.Equal(right.UpdatedAt) {
				return left.UpdatedAt.After(right.UpdatedAt)
			}
		}
		if left.LastSeenAt == nil {
			return false
		}
		if right.LastSeenAt == nil {
			return true
		}
		return left.LastSeenAt.After(right.LastSeenAt)
	})
	total := len(configured)
	start := (page - 1) * perPage
	if start >= total {
		return []*sysin.IntroFeeChannelRuleItem{}, total, nil
	}
	end := start + perPage
	if end > total {
		end = total
	}
	return configured[start:end], total, nil
}

func introFeeChannelDisplayScore(title, username, inviteLink string) int {
	score := 0
	if strings.TrimSpace(title) != "" {
		score += 4
	}
	if strings.TrimSpace(username) != "" {
		score += 2
	}
	if strings.TrimSpace(inviteLink) != "" {
		score++
	}
	return score
}

func (s *sSysPublish) IntroFeeChannelRuleSave(ctx context.Context, in *sysin.IntroFeeChannelRuleSaveInp) (int64, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return 0, err
	}
	if in == nil || in.RuleId <= 0 || strings.TrimSpace(in.ChannelId) == "" {
		return 0, gerror.New("规则ID和频道ID不能为空")
	}
	if !s.introFeeRuleOwned(ctx, account.TenantId, account.Id, in.RuleId) {
		return 0, gerror.New("介绍费规则不存在或无权修改")
	}
	in.ChannelId = canonicalCollectSourceChatID(in.ChannelId)
	now := gtime.Now()
	data := g.Map{"tenant_id": account.TenantId, "account_id": account.Id, "rule_id": in.RuleId, "bot_id": in.BotId, "channel_id": strings.TrimSpace(in.ChannelId), "suffix": strings.TrimSpace(in.Suffix), "status": in.Status, "updated_at": now}
	if in.Status == 0 {
		data["status"] = 1
	}
	if in.Id > 0 {
		result, updateErr := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).
			Where("id", in.Id).Where("tenant_id", account.TenantId).WhereIn("account_id", []int64{account.Id, 0}).
			Where("rule_id", in.RuleId).WhereNull("deleted_at").Data(data).Update()
		if updateErr != nil {
			return 0, updateErr
		}
		rows, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return 0, rowsErr
		}
		if rows == 0 {
			return 0, gerror.New("频道后缀规则不存在或无权修改")
		}
		_, _ = cache.Instance().Remove(ctx, introFeeChannelRuleCacheKey(account.TenantId, account.Id, in.RuleId, in.BotId, in.ChannelId))
		return in.Id, nil
	}
	if existing, queryErr := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("tenant_id", account.TenantId).WhereIn("account_id", []int64{account.Id, 0}).Where("rule_id", in.RuleId).Where("bot_id", in.BotId).WhereIn("channel_id", tgChannelCacheLookupIds(in.ChannelId)).OrderDesc("account_id").One(); queryErr != nil {
		return 0, queryErr
	} else if !existing.IsEmpty() {
		id := existing["id"].Int64()
		_, err = g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("id", id).Data(g.Map{"account_id": account.Id, "suffix": data["suffix"], "status": data["status"], "deleted_at": nil, "updated_at": now}).Update()
		_, _ = cache.Instance().Remove(ctx, introFeeChannelRuleCacheKey(account.TenantId, account.Id, in.RuleId, in.BotId, in.ChannelId))
		return id, err
	}
	data["created_at"] = now
	id, err := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Data(data).InsertAndGetId()
	return id, err
}

func (s *sSysPublish) BotIntroFeeChannelRuleSave(ctx context.Context, in *sysin.IntroFeeChannelRuleSaveInp, tenantId, accountId int64) (int64, error) {
	if tenantId <= 0 || accountId <= 0 || in == nil {
		return 0, gerror.New("频道后缀参数不完整")
	}
	if !s.introFeeRuleOwned(ctx, tenantId, accountId, in.RuleId) {
		return 0, gerror.New("介绍费规则不存在或无权访问")
	}
	now := gtime.Now()
	status := in.Status
	if status == 0 {
		status = 1
	}
	data := g.Map{"tenant_id": tenantId, "account_id": accountId, "rule_id": in.RuleId, "bot_id": in.BotId, "channel_id": canonicalCollectSourceChatID(in.ChannelId), "suffix": strings.TrimSpace(in.Suffix), "status": status, "updated_at": now}
	if in.Id > 0 {
		result, updateErr := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).
			Where("id", in.Id).Where("tenant_id", tenantId).WhereIn("account_id", []int64{accountId, 0}).
			Where("rule_id", in.RuleId).Data(data).Update()
		if updateErr != nil {
			return 0, updateErr
		}
		rows, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return 0, rowsErr
		}
		if rows == 0 {
			return 0, gerror.New("频道后缀规则不存在或无权修改")
		}
		return in.Id, nil
	}
	var existing struct {
		Id int64 `json:"id"`
	}
	_ = g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Fields("id").Where("tenant_id", tenantId).WhereIn("account_id", []int64{accountId, 0}).Where("rule_id", in.RuleId).Where("bot_id", in.BotId).Where("channel_id", data["channel_id"]).WhereNull("deleted_at").OrderDesc("account_id").Scan(&existing)
	if existing.Id > 0 {
		_, err := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("id", existing.Id).Data(data).Update()
		return existing.Id, err
	}
	id, err := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Data(data).InsertAndGetId()
	return id, err
}

func (s *sSysPublish) BotIntroFeeChannelRuleDelete(ctx context.Context, id, ruleId, tenantId, accountId int64) error {
	if id <= 0 || !s.introFeeRuleOwned(ctx, tenantId, accountId, ruleId) {
		return gerror.New("频道后缀规则不存在或无权删除")
	}
	result, err := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).
		Where("id", id).Where("tenant_id", tenantId).WhereIn("account_id", []int64{accountId, 0}).
		Where("rule_id", ruleId).WhereNull("deleted_at").Data(g.Map{"deleted_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "删除频道后缀规则失败")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "检查频道后缀规则删除结果失败")
	}
	if rows == 0 {
		return gerror.New("频道后缀规则不存在或无权删除")
	}
	return nil
}

func (s *sSysPublish) BotIntroFeeChannelRuleList(ctx context.Context, in *sysin.IntroFeeChannelRuleListInp, tenantId, accountId int64) ([]*sysin.IntroFeeChannelRuleItem, int, error) {
	if tenantId <= 0 || accountId <= 0 {
		return nil, 0, gerror.New("频道规则参数不完整")
	}
	if in == nil {
		in = &sysin.IntroFeeChannelRuleListInp{}
	}
	if !s.introFeeRuleOwned(ctx, tenantId, accountId, in.RuleId) {
		return nil, 0, gerror.New("介绍费规则不存在或无权访问")
	}
	return s.introFeeChannelRuleCandidates(ctx, tenantId, accountId, in.RuleId, in.BotId, in.Keyword, in.Page, in.PerPage)
}

func (s *sSysPublish) introFeeRuleOwned(ctx context.Context, tenantID, accountID, ruleID int64) bool {
	if ruleID <= 0 {
		return false
	}
	ok, err := g.DB().Model("hg_youban_publish_collect_rule").Ctx(ctx).Where("id", ruleID).Where("tenant_id", tenantID).Where("account_id", accountID).WhereNull("deleted_at").Exist()
	return err == nil && ok
}

func (s *sSysPublish) IntroFeeChannelRuleDelete(ctx context.Context, id int64) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if id <= 0 {
		return gerror.New("规则ID无效")
	}
	row, _ := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Fields("rule_id,bot_id,channel_id").Where("id", id).Where("tenant_id", account.TenantId).WhereIn("account_id", []int64{account.Id, 0}).WhereNull("deleted_at").One()
	if row.IsEmpty() {
		return gerror.New("频道后缀规则不存在或无权删除")
	}
	_, err = g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("id", id).Where("tenant_id", account.TenantId).WhereIn("account_id", []int64{account.Id, 0}).WhereNull("deleted_at").Data(g.Map{"deleted_at": gtime.Now()}).Update()
	if !row.IsEmpty() {
		_, _ = cache.Instance().Remove(ctx, introFeeChannelRuleCacheKey(account.TenantId, account.Id, row["rule_id"].Int64(), row["bot_id"].Int64(), row["channel_id"].String()))
	}
	return err
}

func (s *sSysPublish) AdminIntroFeeChannelRuleList(ctx context.Context, in *sysin.AdminIntroFeeChannelRuleListInp) (list []*sysin.IntroFeeChannelRuleItem, totalCount int, err error) {
	admin, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.AdminIntroFeeChannelRuleListInp{}
	}
	if in.AccountId <= 0 || !s.publishAccountBelongsToTenant(ctx, in.AccountId, admin.TenantId) {
		return nil, 0, gerror.New("请选择有效的采集账户")
	}
	mod := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("tenant_id", admin.TenantId).WhereIn("account_id", []int64{in.AccountId, 0}).Where("bot_id", in.BotId).Where("rule_id", in.RuleId).WhereNull("deleted_at")
	if in.IncludeCandidates {
		return s.introFeeChannelRuleCandidates(ctx, admin.TenantId, in.AccountId, in.RuleId, in.BotId, in.Keyword, in.Page, in.PerPage)
	}
	if kw := strings.TrimSpace(in.Keyword); kw != "" {
		like := "%" + kw + "%"
		mod = mod.Where("(channel_id LIKE ? OR channel_title LIKE ? OR channel_username LIKE ? OR suffix LIKE ?)", like, like, like, like)
	}
	err = mod.Page(in.Page, in.PerPage).OrderDesc("updated_at").ScanAndCount(&list, &totalCount, false)
	s.fillIntroFeeChannelBlacklist(ctx, admin.TenantId, 0, list)
	return
}

func (s *sSysPublish) AdminIntroFeeChannelRuleSave(ctx context.Context, in *sysin.AdminIntroFeeChannelRuleSaveInp) (int64, error) {
	admin, err := s.currentAdminAccount(ctx)
	if err != nil {
		return 0, err
	}
	if in == nil || in.BotId <= 0 || in.AccountId <= 0 || strings.TrimSpace(in.ChannelId) == "" {
		return 0, gerror.New("账户、机器人和频道ID不能为空")
	}
	if !s.publishAccountBelongsToTenant(ctx, in.AccountId, admin.TenantId) {
		return 0, gerror.New("采集账户不存在或无权访问")
	}
	channelID := canonicalCollectSourceChatID(in.ChannelId)
	now := gtime.Now()
	data := g.Map{"tenant_id": admin.TenantId, "account_id": in.AccountId, "bot_id": in.BotId, "rule_id": in.RuleId, "channel_id": channelID, "suffix": strings.TrimSpace(in.Suffix), "status": 1, "updated_at": now}
	if in.Id > 0 {
		_, err = g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("id", in.Id).Where("tenant_id", admin.TenantId).Data(data).Update()
		return in.Id, err
	}
	row, _ := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("tenant_id", admin.TenantId).WhereIn("account_id", []int64{in.AccountId, 0}).Where("bot_id", in.BotId).Where("rule_id", in.RuleId).Where("channel_id", channelID).OrderDesc("account_id").One()
	if !row.IsEmpty() {
		id := row["id"].Int64()
		_, err = g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("id", id).Data(g.Map{"account_id": in.AccountId, "suffix": data["suffix"], "status": 1, "deleted_at": nil, "updated_at": now}).Update()
		return id, err
	}
	data["created_at"] = now
	id, err := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Data(data).InsertAndGetId()
	return id, err
}

func (s *sSysPublish) publishAccountBelongsToTenant(ctx context.Context, accountID, tenantID int64) bool {
	ok, err := g.DB().Model("hg_youban_publish_account").Ctx(ctx).Where("id", accountID).Where("tenant_id", tenantID).WhereNull("deleted_at").Exist()
	return err == nil && ok
}

func (s *sSysPublish) AdminIntroFeeChannelRuleDelete(ctx context.Context, id int64) error {
	admin, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if id <= 0 {
		return gerror.New("规则ID无效")
	}
	_, err = g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("id", id).Where("tenant_id", admin.TenantId).WhereNull("deleted_at").Data(g.Map{"deleted_at": gtime.Now()}).Update()
	return err
}

func (s *sSysPublish) IntroFeeChannelBlacklist(ctx context.Context, in *sysin.IntroFeeChannelBlacklistInp) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil {
		return gerror.New("频道状态参数不能为空")
	}
	ruleID, botID, channelID := in.RuleId, in.BotId, canonicalCollectSourceChatID(in.ChannelId)
	if in.Id > 0 {
		row, queryErr := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Fields("rule_id,bot_id,channel_id").Where("id", in.Id).Where("tenant_id", account.TenantId).WhereIn("account_id", []int64{account.Id, 0}).WhereNull("deleted_at").One()
		if queryErr != nil {
			return gerror.Wrap(queryErr, "读取介绍费频道规则失败")
		}
		if row.IsEmpty() {
			return gerror.New("频道规则不存在或无权修改")
		}
		ruleID, botID, channelID = row["rule_id"].Int64(), row["bot_id"].Int64(), row["channel_id"].String()
	}
	if channelID == "" || !s.introFeeRuleOwned(ctx, account.TenantId, account.Id, ruleID) {
		return gerror.New("介绍费规则不存在或无权修改")
	}
	mod := g.DB().Model(publishCollectSourceTable).Ctx(ctx).Where("tenant_id", account.TenantId).Where("account_id", account.Id).WhereIn("source_chat_id", tgChannelCacheLookupIds(channelID))
	if botID > 0 {
		mod = mod.Where("bot_id", botID)
	}
	_, err = mod.Data(g.Map{"channel_blacklisted": introFeeBoolToInt(in.Blacklisted != 0)}).Update()
	if err == nil {
		s.setCollectAccountChannelEnabled(ctx, account.TenantId, account.Id, channelID, in.Blacklisted == 0)
		s.refreshCollectSourceCache(ctx)
	}
	return err
}

func (s *sSysPublish) AdminIntroFeeChannelBlacklist(ctx context.Context, in *sysin.IntroFeeChannelBlacklistInp) error {
	admin, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil {
		return gerror.New("频道状态参数不能为空")
	}
	accountID, botID, channelID := in.AccountId, in.BotId, canonicalCollectSourceChatID(in.ChannelId)
	if in.Id > 0 {
		row, queryErr := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Fields("account_id,bot_id,channel_id").Where("id", in.Id).Where("tenant_id", admin.TenantId).WhereNull("deleted_at").One()
		if queryErr != nil {
			return gerror.Wrap(queryErr, "读取介绍费频道规则失败")
		}
		if row.IsEmpty() {
			return gerror.New("介绍费频道规则不存在")
		}
		accountID, botID, channelID = row["account_id"].Int64(), row["bot_id"].Int64(), row["channel_id"].String()
	}
	if accountID <= 0 || channelID == "" || !s.publishAccountBelongsToTenant(ctx, accountID, admin.TenantId) {
		return gerror.New("采集账户或频道不存在")
	}
	mod := g.DB().Model(publishCollectSourceTable).Ctx(ctx).Where("tenant_id", admin.TenantId).Where("account_id", accountID).Where("bot_id", botID).WhereIn("source_chat_id", tgChannelCacheLookupIds(channelID))
	_, err = mod.Data(g.Map{"channel_blacklisted": introFeeBoolToInt(in.Blacklisted != 0)}).Update()
	if err == nil {
		s.setCollectAccountChannelEnabled(ctx, admin.TenantId, accountID, channelID, in.Blacklisted == 0)
		s.refreshCollectSourceCache(ctx)
	}
	return err
}

func (s *sSysPublish) setCollectAccountChannelEnabled(ctx context.Context, tenantID, accountID int64, chatID string, enabled bool) {
	status := sysin.CollectAccountChannelDisabled
	approval := "rejected"
	if enabled {
		status = sysin.CollectAccountChannelEnabled
		approval = "approved"
	}
	var channel struct {
		Id int64 `json:"id"`
	}
	if err := pdao.YoubanPublishCollectChannel.Ctx(ctx).Fields("id").Where("tenant_id", tenantID).WhereIn("telegram_chat_id", tgChannelCacheLookupIds(chatID)).Scan(&channel); err != nil || channel.Id <= 0 {
		return
	}
	now := gtime.Now()
	data := pdo.YoubanPublishCollectAccountChannel{TenantId: tenantID, AccountId: accountID, ChannelId: channel.Id, CollectEnabled: introFeeBoolToInt(enabled), Status: status, ApprovalStatus: approval}
	if enabled {
		data.ApprovedAt = now
	} else {
		data.DisabledAt = now
	}
	pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).Data(data).OnConflict("tenant_id,account_id,channel_id").OnDuplicate("collect_enabled,status,approval_status,approved_at,disabled_at,updated_at").Save()
}

func introFeeBoolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
