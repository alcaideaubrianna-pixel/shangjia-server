package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
	"time"
)

const introFeeChannelRuleTable = "hg_youban_publish_intro_fee_channel_rule"
const introFeeChannelRuleCacheTTL = 10 * time.Minute

func introFeeChannelRuleCacheKey(tenant, rule, bot int64, channel string) string {
	return fmt.Sprintf("youban_publish:intro_fee_rule:%d:%d:%d:%s", tenant, rule, bot, channel)
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
	key := introFeeChannelRuleCacheKey(event["tenant_id"].Int64(), rule["id"].Int64(), botID, chatID)
	if value, cacheErr := cache.Instance().Get(ctx, key); cacheErr == nil && !value.IsNil() {
		if value.String() == "__none__" {
			return "", false, nil
		}
		return value.String(), true, nil
	}
	mod := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).
		Fields("suffix").Where("tenant_id", event["tenant_id"]).Where("rule_id", rule["id"]).WhereIn("channel_id", tgChannelCacheLookupIds(chatID)).Where("status", 1).WhereNull("deleted_at")
	if botID > 0 {
		mod = mod.Where("bot_id", botID)
	} else {
		mod = mod.Where("bot_id", 0)
	}
	row, err := mod.OrderDesc("id").Limit(1).One()
	if err != nil {
		return "", false, gerror.Wrap(err, "读取介绍费频道后缀规则失败")
	}
	if row.IsEmpty() && botID > 0 {
		row, err = g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Fields("suffix").Where("tenant_id", event["tenant_id"]).Where("rule_id", 0).Where("bot_id", botID).WhereIn("channel_id", tgChannelCacheLookupIds(chatID)).Where("status", 1).WhereNull("deleted_at").OrderDesc("id").Limit(1).One()
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
	mod := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("tenant_id", account.TenantId).Where("rule_id", in.RuleId).Where("bot_id", in.BotId).WhereNull("deleted_at")
	if in.IncludeCandidates {
		return s.introFeeChannelRuleCandidates(ctx, account.TenantId, account.Id, in.RuleId, in.BotId, in.Keyword, in.Page, in.PerPage)
	}
	if kw := strings.TrimSpace(in.Keyword); kw != "" {
		like := "%" + kw + "%"
		mod = mod.Where("(channel_id LIKE ? OR channel_title LIKE ? OR channel_username LIKE ?)", like, like, like)
	}
	err = mod.Page(in.Page, in.PerPage).OrderDesc("updated_at").ScanAndCount(&list, &totalCount, false)
	return
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
	if err := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("tenant_id", tenantID).Where("rule_id", ruleID).Where("bot_id", botID).WhereNull("deleted_at").OrderDesc("updated_at").Scan(&configured); err != nil {
		return nil, 0, err
	}
	seen := make(map[string]struct{}, len(configured))
	for _, row := range configured {
		row.Configured = true
		seen[canonicalCollectSourceChatID(row.ChannelId)] = struct{}{}
	}
	cacheModel := g.DB().Model(publishBotChannelCacheTable).Ctx(ctx).Where("tenant_id", tenantID)
	if botID > 0 {
		cacheModel = cacheModel.Where("bot_id", botID)
	}
	if botID == 0 {
		cacheModel = cacheModel.WhereIn("chat_id", g.DB().Model(publishCollectSourceTable).Ctx(ctx).Fields("source_chat_id").Where("tenant_id", tenantID).Where("account_id", accountID).WhereNull("deleted_at"))
	}
	var candidates []struct {
		ChatId       string      `orm:"chat_id"`
		ChatTitle    string      `orm:"chat_title"`
		ChatUsername string      `orm:"chat_username"`
		LastSeenAt   *gtime.Time `orm:"last_message_at"`
	}
	if err := cacheModel.Fields("chat_id,chat_title,chat_username,last_message_at").OrderDesc("last_message_at").Scan(&candidates); err != nil {
		return nil, 0, err
	}
	for _, candidate := range candidates {
		id := canonicalCollectSourceChatID(candidate.ChatId)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		configured = append(configured, &sysin.IntroFeeChannelRuleItem{RuleId: ruleID, BotId: botID, ChannelId: id, ChannelTitle: candidate.ChatTitle, ChannelUsername: candidate.ChatUsername, LastSeenAt: candidate.LastSeenAt, Configured: false})
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		filtered := configured[:0]
		lower := strings.ToLower(keyword)
		for _, row := range configured {
			if strings.Contains(strings.ToLower(row.ChannelId+row.ChannelTitle+row.ChannelUsername), lower) {
				filtered = append(filtered, row)
			}
		}
		configured = filtered
	}
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
	data := g.Map{"tenant_id": account.TenantId, "rule_id": in.RuleId, "bot_id": in.BotId, "channel_id": strings.TrimSpace(in.ChannelId), "suffix": strings.TrimSpace(in.Suffix), "status": in.Status, "updated_at": now}
	if in.Status == 0 {
		data["status"] = 1
	}
	if in.Id > 0 {
		_, err = g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("id", in.Id).Where("tenant_id", account.TenantId).WhereNull("deleted_at").Data(data).Update()
		_, _ = cache.Instance().Remove(ctx, introFeeChannelRuleCacheKey(account.TenantId, in.RuleId, in.BotId, in.ChannelId))
		return in.Id, err
	}
	if existing, queryErr := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("tenant_id", account.TenantId).Where("rule_id", in.RuleId).Where("bot_id", in.BotId).WhereIn("channel_id", tgChannelCacheLookupIds(in.ChannelId)).One(); queryErr != nil {
		return 0, queryErr
	} else if !existing.IsEmpty() {
		id := existing["id"].Int64()
		_, err = g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("id", id).Data(g.Map{"suffix": data["suffix"], "status": data["status"], "deleted_at": nil, "updated_at": now}).Update()
		_, _ = cache.Instance().Remove(ctx, introFeeChannelRuleCacheKey(account.TenantId, in.RuleId, in.BotId, in.ChannelId))
		return id, err
	}
	data["created_at"] = now
	id, err := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Data(data).InsertAndGetId()
	return id, err
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
	row, _ := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Fields("rule_id,bot_id,channel_id").Where("id", id).Where("tenant_id", account.TenantId).WhereNull("deleted_at").One()
	_, err = g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("id", id).Where("tenant_id", account.TenantId).WhereNull("deleted_at").Data(g.Map{"deleted_at": gtime.Now()}).Update()
	if !row.IsEmpty() {
		_, _ = cache.Instance().Remove(ctx, introFeeChannelRuleCacheKey(account.TenantId, row["rule_id"].Int64(), row["bot_id"].Int64(), row["channel_id"].String()))
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
	mod := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("tenant_id", admin.TenantId).Where("bot_id", in.BotId).Where("rule_id", in.RuleId).WhereNull("deleted_at")
	if in.IncludeCandidates {
		return s.introFeeChannelRuleCandidates(ctx, admin.TenantId, 0, in.RuleId, in.BotId, in.Keyword, in.Page, in.PerPage)
	}
	if kw := strings.TrimSpace(in.Keyword); kw != "" {
		like := "%" + kw + "%"
		mod = mod.Where("(channel_id LIKE ? OR channel_title LIKE ? OR channel_username LIKE ?)", like, like, like)
	}
	err = mod.Page(in.Page, in.PerPage).OrderDesc("updated_at").ScanAndCount(&list, &totalCount, false)
	return
}

func (s *sSysPublish) AdminIntroFeeChannelRuleSave(ctx context.Context, in *sysin.AdminIntroFeeChannelRuleSaveInp) (int64, error) {
	admin, err := s.currentAdminAccount(ctx)
	if err != nil {
		return 0, err
	}
	if in == nil || in.BotId <= 0 || strings.TrimSpace(in.ChannelId) == "" {
		return 0, gerror.New("机器人、规则和频道ID不能为空")
	}
	channelID := canonicalCollectSourceChatID(in.ChannelId)
	now := gtime.Now()
	data := g.Map{"tenant_id": admin.TenantId, "bot_id": in.BotId, "rule_id": in.RuleId, "channel_id": channelID, "suffix": strings.TrimSpace(in.Suffix), "status": 1, "updated_at": now}
	if in.Id > 0 {
		_, err = g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("id", in.Id).Where("tenant_id", admin.TenantId).Data(data).Update()
		return in.Id, err
	}
	row, _ := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("tenant_id", admin.TenantId).Where("bot_id", in.BotId).Where("rule_id", in.RuleId).Where("channel_id", channelID).One()
	if !row.IsEmpty() {
		id := row["id"].Int64()
		_, err = g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("id", id).Data(g.Map{"suffix": data["suffix"], "status": 1, "deleted_at": nil, "updated_at": now}).Update()
		return id, err
	}
	data["created_at"] = now
	id, err := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Data(data).InsertAndGetId()
	return id, err
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
	if in == nil || in.Id <= 0 {
		return gerror.New("规则ID无效")
	}
	row, err := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Fields("rule_id,bot_id,channel_id").Where("id", in.Id).Where("tenant_id", account.TenantId).WhereNull("deleted_at").One()
	if err != nil {
		return gerror.Wrap(err, "读取介绍费频道规则失败")
	}
	if row.IsEmpty() || !s.introFeeRuleOwned(ctx, account.TenantId, account.Id, row["rule_id"].Int64()) {
		return gerror.New("介绍费规则不存在或无权修改")
	}
	mod := g.DB().Model(publishCollectSourceTable).Ctx(ctx).Where("tenant_id", account.TenantId).Where("account_id", account.Id).WhereIn("source_chat_id", tgChannelCacheLookupIds(canonicalCollectSourceChatID(row["channel_id"].String())))
	if botID := row["bot_id"].Int64(); botID > 0 {
		mod = mod.Where("bot_id", botID)
	}
	_, err = mod.Data(g.Map{"channel_blacklisted": introFeeBoolToInt(in.Blacklisted != 0)}).Update()
	return err
}

func (s *sSysPublish) AdminIntroFeeChannelBlacklist(ctx context.Context, in *sysin.IntroFeeChannelBlacklistInp) error {
	admin, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || in.Id <= 0 {
		return gerror.New("规则ID无效")
	}
	row, err := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Fields("bot_id,channel_id").Where("id", in.Id).Where("tenant_id", admin.TenantId).WhereNull("deleted_at").One()
	if err != nil {
		return gerror.Wrap(err, "读取介绍费频道规则失败")
	}
	if row.IsEmpty() {
		return gerror.New("介绍费频道规则不存在")
	}
	_, err = g.DB().Model(publishCollectSourceTable).Ctx(ctx).Where("tenant_id", admin.TenantId).Where("bot_id", row["bot_id"].Int64()).WhereIn("source_chat_id", tgChannelCacheLookupIds(canonicalCollectSourceChatID(row["channel_id"].String()))).Data(g.Map{"channel_blacklisted": introFeeBoolToInt(in.Blacklisted != 0)}).Update()
	return err
}

func introFeeBoolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
