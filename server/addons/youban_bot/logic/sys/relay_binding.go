package sys

import (
	"context"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	"hotgo/addons/youban_bot/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
)

func (s *sSysBot) AdminRelayBindingList(ctx context.Context, in *sysin.RelayListInp) ([]*entity.YoubanBotRelayBinding, int, error) {
	botID, err := s.relayCanonicalBotID(ctx, in.BotId)
	if err != nil {
		return nil, 0, err
	}
	in.BotId = botID
	if err := in.Filter(); err != nil {
		return nil, 0, err
	}
	list := make([]*entity.YoubanBotRelayBinding, 0)
	total := 0
	c := dao.YoubanBotRelayBinding.Columns()
	mod := dao.YoubanBotRelayBinding.Ctx(ctx).Where(c.BotId, in.BotId)
	if in.Keyword != "" {
		mod = mod.Where("("+c.LeftName+" LIKE ? OR "+c.RightName+" LIKE ?)", "%"+in.Keyword+"%", "%"+in.Keyword+"%")
	}
	err = mod.Page(in.Page, in.PerPage).OrderDesc(c.Id).ScanAndCount(&list, &total, false)
	return list, total, gerror.Wrap(err, "读取双向绑定失败")
}

func (s *sSysBot) adminRelayBind(ctx context.Context, in *sysin.RelayBindingInp) error {
	if err := in.Filter(); err != nil {
		return err
	}
	cfg, err := relayConfig(ctx, in.BotId)
	if err != nil {
		return err
	}
	if cfg == nil {
		return gerror.New("请先保存双向机器人设置")
	}
	left, err := s.relayResolvePeerOrPending(ctx, in.BotId, in.Left)
	if err != nil {
		return err
	}
	right, err := s.relayResolvePeerOrPending(ctx, in.BotId, in.Right)
	if err != nil {
		return err
	}
	if left.ID == right.ID {
		return gerror.New("不能绑定同一个聊天对象")
	}
	if left.ID == cfg.AuditGroupId || right.ID == cfg.AuditGroupId {
		return gerror.New("审计群不能作为双向绑定对象")
	}
	for _, peer := range []*relayPeer{left, right} {
		if relayPeerPending(peer) {
			continue
		}
		if err = s.relayValidatePeer(ctx, in.BotId, peer); err != nil {
			return err
		}
	}
	if left.ID > right.ID {
		left, right = right, left
	}
	return dao.YoubanBotRelayBinding.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		// Lock the configuration row to serialize bindings and audit-target changes across instances.
		var locked *entity.YoubanBotRelayConfig
		if err := dao.YoubanBotRelayConfig.Ctx(ctx).WherePri(cfg.Id).LockUpdate().Scan(&locked); err != nil {
			return err
		}
		if locked.AuditGroupId == left.ID || locked.AuditGroupId == right.ID {
			return gerror.New("审计群不能作为双向绑定对象")
		}
		var old *entity.YoubanBotRelayBinding
		if err := dao.YoubanBotRelayBinding.Ctx(ctx).Where(do.YoubanBotRelayBinding{BotId: in.BotId, PairKey: relayPairKey(left.ID, right.ID)}).Scan(&old); err != nil {
			return err
		}
		if old != nil {
			return nil
		}
		count, err := dao.YoubanBotRelayEndpoint.Ctx(ctx).Where(dao.YoubanBotRelayEndpoint.Columns().BotId, in.BotId).WhereIn(dao.YoubanBotRelayEndpoint.Columns().ChatId, []string{left.ID, right.ID}).Count()
		if err != nil {
			return err
		}
		if count > 0 {
			return gerror.New("聊天对象已有绑定，请先解绑")
		}
		id, err := dao.YoubanBotRelayBinding.Ctx(ctx).Data(do.YoubanBotRelayBinding{BotId: in.BotId, PairKey: relayPairKey(left.ID, right.ID), LeftChatId: left.ID, RightChatId: right.ID, LeftName: left.Name, RightName: right.Name}).InsertAndGetId()
		if err != nil {
			return gerror.Wrap(err, "创建双向绑定失败")
		}
		for _, peer := range []*relayPeer{left, right} {
			if _, err = dao.YoubanBotRelayEndpoint.Ctx(ctx).Data(do.YoubanBotRelayEndpoint{BotId: in.BotId, BindingId: id, ChatId: peer.ID}).Insert(); err != nil {
				return gerror.Wrap(err, "聊天对象已有绑定，请先解绑")
			}
		}
		return nil
	})
}

func (s *sSysBot) adminRelayUnbind(ctx context.Context, in *sysin.RelayDeleteInp) error {
	if err := in.RelayBotInp.Filter(); err != nil {
		return err
	}
	if in.Id <= 0 {
		return gerror.New("请选择绑定记录")
	}
	return dao.YoubanBotRelayBinding.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var binding *entity.YoubanBotRelayBinding
		if err := dao.YoubanBotRelayBinding.Ctx(ctx).Where(do.YoubanBotRelayBinding{Id: in.Id, BotId: in.BotId}).LockUpdate().Scan(&binding); err != nil {
			return err
		}
		if binding == nil {
			return gerror.New("绑定不存在或已解除")
		}
		if _, err := dao.YoubanBotRelayEndpoint.Ctx(ctx).Where(do.YoubanBotRelayEndpoint{BotId: in.BotId, BindingId: in.Id}).Delete(); err != nil {
			return err
		}
		if _, err := dao.YoubanBotRelayBinding.Ctx(ctx).WherePri(in.Id).Delete(); err != nil {
			return err
		}
		c := dao.YoubanBotRelayDelivery.Columns()
		_, err := dao.YoubanBotRelayDelivery.Ctx(ctx).Where(c.BotId, in.BotId).Where(c.BindingId, in.Id).Where(c.Kind, consts.RelayKindChat).Where(c.Status, consts.RelayPending).Data(do.YoubanBotRelayDelivery{Status: consts.RelayCancelled, RawJson: ""}).Update()
		return gerror.Wrap(err, "取消待转发消息失败")
	})
}

func relayBindingByChat(ctx context.Context, botID int64, chatID string) (*entity.YoubanBotRelayBinding, error) {
	var endpoint *entity.YoubanBotRelayEndpoint
	if err := dao.YoubanBotRelayEndpoint.Ctx(ctx).Where(do.YoubanBotRelayEndpoint{BotId: botID, ChatId: chatID}).Scan(&endpoint); err != nil {
		return nil, err
	}
	if endpoint == nil {
		return nil, nil
	}
	var row *entity.YoubanBotRelayBinding
	err := dao.YoubanBotRelayBinding.Ctx(ctx).Where(do.YoubanBotRelayBinding{Id: endpoint.BindingId, BotId: botID}).Scan(&row)
	return row, gerror.Wrap(err, "读取聊天绑定失败")
}
