package sys

import (
	"context"
	"fmt"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"strings"
)

func (s *sSysPublish) MyProfileFilterOptions(ctx context.Context, in *sysin.ProfileFilterOptionsInp) (*sysin.ProfileFilterOptionsModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	capability, err := s.activeAccountCapability(ctx, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	selected := int64(0)
	if in != nil {
		selected = in.AccountId
	}
	allIds, err := s.readableProfileAccountIds(ctx, capability, 0)
	if err != nil {
		return nil, err
	}
	ids, err := s.readableProfileAccountIds(ctx, capability, selected)
	if err != nil {
		return nil, err
	}
	var accounts []accountOptionRow
	columns := pdao.YoubanPublishAccount.Columns()
	if err = pdao.YoubanPublishAccount.Ctx(ctx).Fields(columns.Id, columns.Nickname).Where(columns.TenantId, account.TenantId).WhereIn(columns.Id, allIds).WhereNull(columns.DeletedAt).OrderAsc(columns.Id).Scan(&accounts); err != nil {
		return nil, gerror.Wrap(err, "读取资料账户筛选失败")
	}
	res := &sysin.ProfileFilterOptionsModel{Accounts: buildAccountOptions(accounts, ""), SourceChannels: []*sysin.CollectSourceOptionModel{}, PushChannels: []*sysin.AccountOptionModel{}}
	// Compact account/channel relationship tables, never the historical event table.
	var channels []struct {
		SourceId     int64
		SourceChatId string
		Label        string
	}
	ac := pdao.YoubanPublishCollectAccountChannel
	ch := pdao.YoubanPublishCollectChannel
	err = ac.Ctx(ctx).As("ac").InnerJoin(ch.Table()+" c", "c.id=ac.channel_id AND c.tenant_id=ac.tenant_id AND c.deleted_at IS NULL").Fields("ac.source_id,c.telegram_chat_id AS source_chat_id,c.title AS label").Where("ac.tenant_id", account.TenantId).WhereIn("ac.account_id", ids).WhereGT("ac.source_id", 0).WhereNull("ac.deleted_at").OrderAsc("c.title").Scan(&channels)
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集频道筛选失败")
	}

	// Retain legacy and removed source configurations for historical filtering.
	var legacy []struct {
		SourceId     int64
		SourceChatId string
		Label        string
	}
	if err = g.DB().Model(publishCollectSourceTable+" s").Safe().Ctx(ctx).
		Fields("s.id AS source_id,s.source_chat_id,s.title AS label").
		Where("s.tenant_id", account.TenantId).WhereIn("s.account_id", ids).
		Where("s.source_chat_id != ''").Scan(&legacy); err != nil {
		return nil, gerror.Wrap(err, "读取历史采集频道筛选失败")
	}
	for _, row := range legacy {
		channels = append(channels, row)
	}
	// Bot-all sources have no source_chat_id; use the compact bot channel cache.
	var botChannels []struct {
		SourceId     int64
		SourceChatId string
		Label        string
	}
	if err = g.DB().Model(publishCollectSourceTable+" s").Safe().Ctx(ctx).
		InnerJoin(publishBotChannelCacheTable+" bc", "bc.tenant_id=s.tenant_id AND bc.bot_id=s.bot_id").
		Fields("s.id AS source_id,bc.chat_id AS source_chat_id,bc.chat_title AS label").
		Where("s.tenant_id", account.TenantId).WhereIn("s.account_id", ids).
		Where("s.source_type", "bot").Where("s.bot_collect_scope", "all").
		Where("bc.chat_id != ''").Scan(&botChannels); err != nil {
		return nil, gerror.Wrap(err, "读取机器人采集频道筛选失败")
	}
	for _, row := range botChannels {
		channels = append(channels, row)
	}
	seen := map[string]bool{}
	for _, row := range channels {
		key := fmt.Sprintf("%d:%s", row.SourceId, row.SourceChatId)
		if seen[key] {
			continue
		}
		seen[key] = true
		label := strings.TrimSpace(row.Label)
		if label == "" {
			label = row.SourceChatId
		}
		res.SourceChannels = append(res.SourceChannels, &sysin.CollectSourceOptionModel{Id: row.SourceId, SourceId: row.SourceId, SourceChatId: row.SourceChatId, Label: label})
	}
	// Push destinations are tenant-owned, including defaults shared by all accounts.
	if err = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Fields("id AS value,channel_title AS label").Where("tenant_id", account.TenantId).Where("publish_direction", "up").WhereNull("deleted_at").OrderAsc("id").Scan(&res.PushChannels); err != nil {
		return nil, gerror.Wrap(err, "读取推送频道筛选失败")
	}
	return res, nil
}
