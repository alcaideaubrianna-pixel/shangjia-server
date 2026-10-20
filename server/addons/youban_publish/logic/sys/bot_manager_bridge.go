package sys

import (
	"context"
	"fmt"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/model/input/sysin"
	"strings"
)

// Only channels reachable by an account owned by this user are manageable.
// Tenant sharing for reading profiles does not confer Telegram administration.
func (s *sSysPublish) BotManagerCatalog(ctx context.Context, tenantId, accountId int64) ([]*sysin.BotManagerChannel, []*sysin.BotManagerBot, error) {
	if tenantId <= 0 || accountId <= 0 {
		return nil, nil, gerror.New("租户账户无效")
	}
	var channels []*sysin.BotManagerChannel
	var bots []*sysin.BotManagerBot
	c := dao.YoubanPublishChannel.Columns()
	q := dao.YoubanPublishChannel.Ctx(ctx).As("c").Fields("c.id,c.tg_account_id,c.target_chat_id,c.channel_title,c.channel_username").
		Where("c."+c.TenantId, tenantId).Where("c."+c.Status, 1).Where("c."+c.PublishDirection, "up").
		Where("EXISTS (SELECT 1 FROM "+publishTgAccountTable+" a WHERE a.id=c.tg_account_id AND a.tenant_id=? AND a.account_id=? AND a.deleted_at IS NULL)", tenantId, accountId)
	if err := q.OrderAsc("c.id").Scan(&channels); err != nil {
		return nil, nil, gerror.Wrap(err, "读取可管理上架频道失败")
	}
	var caches []struct {
		TgAccountId int64
		ChannelId   string
		AccessHash  string
	}
	if len(channels) > 0 {
		accountErrors := make(map[int64]string)
		for _, channel := range channels {
			if _, checked := accountErrors[channel.TgAccountId]; !checked {
				available, err := checkTelegramProtocolAccount(ctx, dao.YoubanPublishChannel.DB(), tenantId, channel.TgAccountId)
				if err != nil {
					return nil, nil, err
				}
				accountErrors[channel.TgAccountId] = ""
				if !available {
					accountErrors[channel.TgAccountId] = "协议号未登录或登录已失效，请重新登录后同步"
				}
			}
			channel.AccountError = accountErrors[channel.TgAccountId]
		}
		if err := dao.YoubanPublishChannel.DB().Model(publishTgChannelTable).Ctx(ctx).Fields("tg_account_id,channel_id,access_hash").Where("tenant_id", tenantId).Scan(&caches); err != nil {
			return nil, nil, err
		}
		hashes := map[string]string{}
		for _, c := range caches {
			hashes[fmt.Sprintf("%d:%s", c.TgAccountId, strings.TrimPrefix(strings.TrimSpace(c.ChannelId), "-100"))] = c.AccessHash
		}
		for _, c := range channels {
			c.AccessHash = hashes[fmt.Sprintf("%d:%s", c.TgAccountId, strings.TrimPrefix(strings.TrimSpace(c.TargetChatId), "-100"))]
		}
	}
	b := dao.YoubanPublishBot.Columns()
	err := dao.YoubanPublishBot.Ctx(ctx).Fields(b.Id, b.BotName, b.BotUsername, b.BotToken).Where(b.TenantId, tenantId).Where(b.AccountId, accountId).Where(b.Status, 1).OrderAsc(b.Id).Scan(&bots)
	return channels, bots, gerror.Wrap(err, "读取已有上架机器人失败")
}

// Compatibility projection keeps every historical publish Bot ID unchanged.
// New manager data belongs to youban_bot; publishing only consumes this binding.
func (s *sSysPublish) BotManagerApplyBinding(ctx context.Context, in *sysin.BotManagerProjection) (int64, error) {
	if in == nil || in.TenantId <= 0 || in.AccountId <= 0 {
		return 0, gerror.New("机器人归属无效")
	}
	botId := in.BotId
	err := dao.YoubanPublishChannel.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		// Share the durable creation gate with every new profile job. Keep it
		// through the unfinished-job check and binding commit, before remote RPCs.
		if _, err := lockChannelPermissionCreationTx(ctx, tx, in.TenantId, in.ChannelId); err != nil {
			return err
		}
		// Serialize compatibility Bot creation across different channels of one account.
		var owner *struct{ Id int64 }
		if err := tx.Model(publishAccountTable).Where("id", in.AccountId).Where("tenant_id", in.TenantId).WhereNull("deleted_at").LockUpdate().Scan(&owner); err != nil {
			return err
		}
		if owner == nil {
			return gerror.New("上架账户已不存在")
		}
		if !in.Remove {
			if err := s.BotManagerValidateToken(ctx, in.TenantId, in.AccountId, in.BotToken); err != nil {
				return err
			}
		}
		channels, _, err := s.BotManagerCatalog(ctx, in.TenantId, in.AccountId)
		if err != nil {
			return err
		}
		allowed := false
		for _, c := range channels {
			if c.Id == in.ChannelId {
				allowed = true
				break
			}
		}
		if !allowed {
			return gerror.New("频道不存在或不属于当前账户")
		}
		var channel struct {
			BotIdJson    string
			TargetChatId string
		}
		cc := dao.YoubanPublishChannel.Columns()
		if err := dao.YoubanPublishChannel.Ctx(ctx).WherePri(in.ChannelId).LockUpdate().Scan(&channel); err != nil {
			return err
		}
		if in.Remove {
			n, err := botManagerUnfinishedJobs(ctx, in.TenantId, in.ChannelId, botId)
			if err != nil {
				return err
			}
			if n > 0 {
				return gerror.New("该机器人还有未完成发布任务，请完成或取消后移出")
			}
		} else {
			if strings.TrimSpace(in.BotToken) == "" {
				return gerror.New("尚未配置上架机器人Token")
			}
			bc := dao.YoubanPublishBot.Columns()
			if botId > 0 {
				n, err := dao.YoubanPublishBot.Ctx(ctx).WherePri(botId).Where(bc.TenantId, in.TenantId).Where(bc.AccountId, in.AccountId).Count()
				if err != nil {
					return err
				}
				if n != 1 {
					return gerror.New("机器人归属已变化")
				}
			} else {
				var existing *struct{ Id int64 }
				if err := dao.YoubanPublishBot.Ctx(ctx).Where(bc.TenantId, in.TenantId).Where(bc.AccountId, in.AccountId).Where(bc.BotToken, in.BotToken).Scan(&existing); err != nil {
					return err
				}
				if existing != nil {
					botId = existing.Id
				}
			}
			data := do.YoubanPublishBot{TenantId: in.TenantId, AccountId: in.AccountId, BotName: in.BotName, BotUsername: in.BotUsername, BotToken: in.BotToken, Status: 1}
			if botId == 0 {
				botId, err = dao.YoubanPublishBot.Ctx(ctx).Data(data).InsertAndGetId()
			} else {
				// A queued operation may hold an older credential snapshot. Existing
				// credentials change only through the explicit credential-save path.
				data.BotToken = nil
				_, err = dao.YoubanPublishBot.Ctx(ctx).WherePri(botId).Data(data).Update()
			}
			if err != nil {
				return err
			}
		}
		ids := decodeBotIds(channel.BotIdJson)
		next := make([]int64, 0, len(ids)+1)
		for _, id := range ids {
			if id != botId {
				next = append(next, id)
			}
		}
		if !in.Remove {
			next = append(next, botId)
		}
		raw, err := encodeBotIds(next)
		if err != nil {
			return err
		}
		_, err = dao.YoubanPublishChannel.Ctx(ctx).Where(cc.Id, in.ChannelId).Data(do.YoubanPublishChannel{BotIdJson: raw}).Update()
		return err
	})
	if err == nil {
		s.clearTelegramBotCache()
	}
	return botId, gerror.Wrap(err, "保存上架机器人频道绑定失败")
}

func (s *sSysPublish) BotManagerRemoveFromChannel(ctx context.Context, in *sysin.BotManagerProjection, remove func() error) error {
	if in == nil || !in.Remove || remove == nil {
		return gerror.New("移除机器人参数无效")
	}
	return s.BotManagerProtectChannel(ctx, in, func() error {
		// Stop selecting this Bot before Telegram can revoke its membership.
		// Remote failure must not restore an unsafe sending binding.
		if _, err := s.BotManagerApplyBinding(ctx, in); err != nil {
			return err
		}
		return remove()
	})
}

func (s *sSysPublish) BotManagerProtectChannel(ctx context.Context, in *sysin.BotManagerProjection, mutate func() error) error {
	if in == nil || mutate == nil {
		return gerror.New("频道管理参数无效")
	}
	channels, _, err := s.BotManagerCatalog(ctx, in.TenantId, in.AccountId)
	if err != nil {
		return err
	}
	chat := ""
	for _, c := range channels {
		if c.Id == in.ChannelId {
			chat = c.TargetChatId
		}
	}
	if chat == "" {
		return gerror.New("频道不存在或无权管理")
	}
	lease, ok, err := s.tryTelegramChannelLease(ctx, chat)
	if err != nil {
		return err
	}
	if !ok {
		return gerror.New("频道正在发送，请稍后重试移出机器人")
	}
	defer s.releaseTelegramChannelLease(ctx, lease)
	// Queued jobs retain Bot IDs. Do not break their transport by removing it.
	n, err := botManagerUnfinishedJobs(ctx, in.TenantId, in.ChannelId, in.BotId)
	if err != nil {
		return err
	}
	if n > 0 {
		return gerror.New("该机器人还有未完成发布任务，请完成或取消后移出")
	}
	return mutate()
}

func botManagerUnfinishedJobs(ctx context.Context, tenantId, channelId, botId int64) (int, error) {
	return dao.YoubanPublishChannel.DB().Model(publishTgJobTable).Ctx(ctx).
		Where("tenant_id", tenantId).Where("channel_id", channelId).Where("bot_id", botId).
		WhereNotIn("status", []string{telegramJobStatusSent, telegramJobStatusFailed, telegramJobStatusSuperseded, "cancelled", "timeout_cancelled"}).Count()
}

func (s *sSysPublish) BotManagerValidateToken(ctx context.Context, tenantId, accountId int64, token string) error {
	identity := strings.Split(token, ":")[0] + ":%"
	c := dao.YoubanPublishBot.Columns()
	n, err := dao.YoubanPublishBot.Ctx(ctx).WhereLike(c.BotToken, identity).Where("("+c.TenantId+"<>? OR "+c.AccountId+"<>?)", tenantId, accountId).Count()
	if err != nil {
		return err
	}
	if n > 0 {
		return gerror.New("机器人已有其他账户的上架配置，不能接管")
	}
	return nil
}

// Credential persistence shares the caller's transaction with the registry.
// Existing publishing rows remain authoritative; status and type are preserved.
func (s *sSysPublish) BotManagerUpdateCredentials(ctx context.Context, in *sysin.BotManagerProjection) (int64, error) {
	if in == nil || in.TenantId <= 0 || in.AccountId <= 0 || strings.TrimSpace(in.BotToken) == "" {
		return 0, gerror.New("机器人凭据参数无效")
	}
	var botId int64
	err := dao.YoubanPublishBot.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var owner *struct{ Id int64 }
		if err := tx.Model(publishAccountTable).Where("id", in.AccountId).Where("tenant_id", in.TenantId).WhereNull("deleted_at").LockUpdate().Scan(&owner); err != nil {
			return err
		}
		if owner == nil {
			return gerror.New("上架账户已不存在")
		}
		if err := s.BotManagerValidateToken(ctx, in.TenantId, in.AccountId, in.BotToken); err != nil {
			return err
		}
		c := dao.YoubanPublishBot.Columns()
		identity := strings.Split(in.BotToken, ":")[0] + ":%"
		v, err := dao.YoubanPublishBot.Ctx(ctx).Where(c.TenantId, in.TenantId).Where(c.AccountId, in.AccountId).WhereLike(c.BotToken, identity).OrderAsc(c.Id).Value(c.Id)
		if err != nil {
			return err
		}
		botId = v.Int64()
		if botId == 0 {
			botId, err = dao.YoubanPublishBot.Ctx(ctx).Data(do.YoubanPublishBot{TenantId: in.TenantId, AccountId: in.AccountId, BotToken: in.BotToken, BotName: in.BotName, BotUsername: in.BotUsername, Status: 1}).InsertAndGetId()
			return err
		}
		_, err = dao.YoubanPublishBot.Ctx(ctx).Where(c.TenantId, in.TenantId).Where(c.AccountId, in.AccountId).
			WhereLike(c.BotToken, strings.Split(in.BotToken, ":")[0]+":%").
			Data(do.YoubanPublishBot{BotToken: in.BotToken, BotName: in.BotName, BotUsername: in.BotUsername}).Update()
		return gerror.Wrap(err, "更新发送机器人凭据失败")
	})
	return botId, err
}

func (s *sSysPublish) BotManagerDelete(ctx context.Context, tenantId, accountId, botId int64) error {
	if botId <= 0 {
		return nil
	}
	err := dao.YoubanPublishBot.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		bc := dao.YoubanPublishBot.Columns()
		var owned *struct{ Id int64 }
		err := dao.YoubanPublishBot.Ctx(ctx).WherePri(botId).Where(bc.TenantId, tenantId).Where(bc.AccountId, accountId).LockUpdate().Scan(&owned)
		if err != nil {
			return err
		}
		if owned == nil {
			return gerror.New("机器人归属不一致")
		}
		var channels []struct{ BotIdJson string }
		if err := dao.YoubanPublishChannel.Ctx(ctx).Where(dao.YoubanPublishChannel.Columns().TenantId, tenantId).Fields(dao.YoubanPublishChannel.Columns().BotIdJson).Scan(&channels); err != nil {
			return err
		}
		for _, ch := range channels {
			for _, id := range decodeBotIds(ch.BotIdJson) {
				if id == botId {
					return gerror.New("机器人仍被上架频道引用，请先移出频道")
				}
			}
		}
		for _, table := range []string{publishTgJobTable, publishCollectSourceTable} {
			q := tx.Model(table).Where("bot_id", botId)
			if table == publishTgJobTable {
				q = q.WhereNotIn("status", []string{"sent", "failed", "cancelled", "timeout_cancelled"})
			} else {
				q = q.WhereNull("deleted_at")
			}
			n, err := q.Count()
			if err != nil {
				return err
			}
			if n > 0 {
				return gerror.New("机器人仍被采集或未完成发布任务引用")
			}
		}
		_, err = dao.YoubanPublishBot.Ctx(ctx).WherePri(botId).Delete()
		return err
	})
	if err == nil {
		s.clearTelegramBotCache()
	}
	return err
}
