package sys

import (
	"context"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

type introFeeChannelSourceScope struct {
	Id     int64  `orm:"id"`
	ChatId string `orm:"source_chat_id"`
	BotId  int64  `orm:"bot_id"`
}

const introFeeChannelSourceScopeLimit = 1000

// Evidence proves ownership only, never permission to collect. In particular,
// a disabled account relation must remain addressable by an explicit toggle.
type introFeeChannelSourceEvidence struct {
	TenantID  int64
	AccountID int64
	SourceID  int64
	BotID     int64
	ChatID    string
	Status    string
	Deleted   bool
}

type introFeeChannelSourceEvidenceSet struct {
	AccountChannels []introFeeChannelSourceEvidence
	BotChannels     []introFeeChannelSourceEvidence
	BotCache        []introFeeChannelSourceEvidence
}

func introFeeChannelSourceHasBinding(tenantID, accountID, ruleBotID int64, chatID string, source introFeeChannelSourceScope, evidence introFeeChannelSourceEvidenceSet) bool {
	chat := canonicalCollectSourceChatID(chatID)
	if tenantID <= 0 || accountID <= 0 || source.Id <= 0 || chat == "" {
		return false
	}
	if introFeeConfiguredSourceMatchesChannel(source.ChatId, chatID) {
		return true
	}
	for _, relation := range evidence.AccountChannels {
		if !relation.Deleted && relation.TenantID == tenantID && relation.AccountID == accountID &&
			relation.SourceID == source.Id && canonicalCollectSourceChatID(relation.ChatID) == chat {
			return true
		}
	}
	if source.BotId <= 0 || ruleBotID != 0 && ruleBotID != source.BotId {
		return false
	}
	seen, removed := false, false
	for _, relation := range evidence.BotChannels {
		if relation.Deleted || relation.TenantID != tenantID || relation.BotID != source.BotId ||
			canonicalCollectSourceChatID(relation.ChatID) != chat {
			continue
		}
		switch relation.Status {
		case sysin.CollectBotChannelRemoved:
			removed = true
		case sysin.CollectBotChannelDiscovered, sysin.CollectBotChannelEnabled, sysin.CollectBotChannelDisabled:
			seen = true
		}
	}
	// Fail closed on contradictory aliases. Existing account ownership above
	// still permits an explicit toggle; a stale cache cannot create ownership.
	if removed {
		return false
	}
	if seen {
		return true
	}
	for _, cached := range evidence.BotCache {
		if !cached.Deleted && cached.TenantID == tenantID && cached.BotID == source.BotId &&
			canonicalCollectSourceChatID(cached.ChatID) == chat {
			return true
		}
	}
	return false
}

// The caller supplies sources already selected for this rule, tenant and
// account. No source discovery, wildcard scope or state mutation happens here.
// All three reads are batched over selected IDs and one chat's aliases.
func resolveIntroFeeChannelSourceBindings(ctx context.Context, tenantID, accountID int64, ruleBotID int64, chatID string, sources []introFeeChannelSourceScope) (map[int64]bool, error) {
	aliases := tgChannelCacheLookupIds(chatID)
	if tenantID <= 0 || accountID <= 0 || ruleBotID < 0 || canonicalCollectSourceChatID(chatID) == "" || len(aliases) == 0 {
		return nil, gerror.New("解析频道来源归属参数无效")
	}
	if len(sources) > introFeeChannelSourceScopeLimit {
		return nil, gerror.New("规则采集源数量超过单次频道归属解析上限")
	}
	bindings := make(map[int64]bool, len(sources))
	var sourceIDs, botIDs []int64
	for _, source := range sources {
		if source.Id <= 0 {
			return nil, gerror.New("规则采集源ID无效")
		}
		sourceIDs = append(sourceIDs, source.Id)
		if source.BotId > 0 && (ruleBotID == 0 || ruleBotID == source.BotId) {
			botIDs = append(botIDs, source.BotId)
		}
	}
	sourceIDs, botIDs = uniqueIds(sourceIDs), uniqueIds(botIDs)
	if len(sourceIDs) == 0 {
		return bindings, nil
	}
	var evidence introFeeChannelSourceEvidenceSet
	channelTable := pdao.YoubanPublishCollectChannel.Table()
	accountTable := pdao.YoubanPublishCollectAccountChannel.Table()
	accountCols := pdao.YoubanPublishCollectAccountChannel.Columns()
	channelCols := pdao.YoubanPublishCollectChannel.Columns()
	var accounts []struct {
		SourceID int64 `orm:"source_id"`
	}
	if err := g.DB().Model(accountTable+" ac").Ctx(ctx).
		InnerJoin(channelTable+" ch", "ch."+channelCols.Id+"=ac."+accountCols.ChannelId+
			" AND ch."+channelCols.TenantId+"=ac."+accountCols.TenantId).
		Fields("ac."+accountCols.SourceId).
		Where("ac."+accountCols.TenantId, tenantID).Where("ac."+accountCols.AccountId, accountID).
		WhereIn("ac."+accountCols.SourceId, sourceIDs).WhereIn("ch."+channelCols.TelegramChatId, aliases).
		WhereNull("ac." + accountCols.DeletedAt).WhereNull("ch." + channelCols.DeletedAt).
		Group("ac." + accountCols.SourceId).Limit(len(sourceIDs) + 1).Scan(&accounts); !collectOptionalRecordScanSucceeded(err) {
		return nil, gerror.Wrap(err, "读取规则来源频道归属失败")
	}
	for _, row := range accounts {
		evidence.AccountChannels = append(evidence.AccountChannels, introFeeChannelSourceEvidence{
			TenantID: tenantID, AccountID: accountID, SourceID: row.SourceID, ChatID: chatID,
		})
	}
	if len(botIDs) > 0 {
		botCols := pdao.YoubanPublishCollectBotChannel.Columns()
		var bots []struct {
			BotID  int64  `orm:"bot_id"`
			Status string `orm:"status"`
		}
		statuses := []string{sysin.CollectBotChannelDiscovered, sysin.CollectBotChannelEnabled,
			sysin.CollectBotChannelDisabled, sysin.CollectBotChannelRemoved}
		if err := g.DB().Model(pdao.YoubanPublishCollectBotChannel.Table()+" bc").Ctx(ctx).
			InnerJoin(channelTable+" ch", "ch."+channelCols.Id+"=bc."+botCols.ChannelId+
				" AND ch."+channelCols.TenantId+"=bc."+botCols.TenantId).
			Fields("bc."+botCols.BotId, "bc."+botCols.Status).
			Where("bc."+botCols.TenantId, tenantID).WhereIn("bc."+botCols.BotId, botIDs).
			WhereIn("ch."+channelCols.TelegramChatId, aliases).WhereIn("bc."+botCols.Status, statuses).
			WhereNull("bc." + botCols.DeletedAt).WhereNull("ch." + channelCols.DeletedAt).
			Group("bc." + botCols.BotId + ",bc." + botCols.Status).Limit(len(botIDs)*len(statuses) + 1).
			Scan(&bots); !collectOptionalRecordScanSucceeded(err) {
			return nil, gerror.Wrap(err, "读取规则机器人频道归属失败")
		}
		for _, row := range bots {
			evidence.BotChannels = append(evidence.BotChannels, introFeeChannelSourceEvidence{
				TenantID: tenantID, BotID: row.BotID, ChatID: chatID, Status: row.Status,
			})
		}
		var cached []struct {
			BotID int64 `orm:"bot_id"`
		}
		// bot_channel_cache has no soft-delete column in the actual schema.
		if err := g.DB().Model(publishBotChannelCacheTable).Ctx(ctx).
			Fields("bot_id").Where("tenant_id", tenantID).WhereIn("bot_id", botIDs).
			WhereIn("chat_id", aliases).Group("bot_id").Limit(len(botIDs) + 1).
			Scan(&cached); !collectOptionalRecordScanSucceeded(err) {
			return nil, gerror.Wrap(err, "读取规则机器人频道缓存归属失败")
		}
		for _, row := range cached {
			evidence.BotCache = append(evidence.BotCache, introFeeChannelSourceEvidence{
				TenantID: tenantID, BotID: row.BotID, ChatID: chatID,
			})
		}
	}
	for _, source := range sources {
		bindings[source.Id] = introFeeChannelSourceHasBinding(tenantID, accountID, ruleBotID, chatID, source, evidence)
	}
	return bindings, nil
}
