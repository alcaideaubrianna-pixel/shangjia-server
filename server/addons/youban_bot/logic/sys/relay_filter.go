package sys

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	"hotgo/addons/youban_bot/model/input/sysin"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
)

var relayTelegramLink = regexp.MustCompile(`(?i)(?:t\.me|telegram\.me|telegram\.dog)/|tg://`)

func relayHasJump(msg *models.Message) bool {
	if strings.Contains(msg.Text, "@") || strings.Contains(msg.Caption, "@") || relayTelegramLink.MatchString(msg.Text) || relayTelegramLink.MatchString(msg.Caption) {
		return true
	}
	for _, entities := range [][]models.MessageEntity{msg.Entities, msg.CaptionEntities} {
		for _, e := range entities {
			if e.Type == models.MessageEntityTypeMention || e.Type == models.MessageEntityTypeTextMention || strings.Contains(e.URL, "@") || relayTelegramLink.MatchString(e.URL) {
				return true
			}
		}
	}
	return false
}

func relayFilterReason(cfg *entity.YoubanBotRelayConfig, rules []*entity.YoubanBotRelayRule, msg *models.Message, target string) string {
	source := strconv.FormatInt(msg.Chat.ID, 10)
	user := ""
	if msg.From != nil {
		user = strconv.FormatInt(msg.From.ID, 10)
	}
	for _, r := range rules {
		if r.Kind == consts.RelayRuleUser && (r.Value == user || r.Value == source || r.Value == target) {
			return "当前用户已被屏蔽，此消息未传递。"
		}
		if r.Kind == consts.RelayRuleGroup && (r.Value == source || r.Value == target) {
			return "当前群聊已被屏蔽，此消息未传递。"
		}
	}
	if cfg.BlockLinks == 1 && relayHasJump(msg) {
		return "消息包含 @ 或 Telegram 跳转链接，已拦截，未传递。"
	}
	content := strings.ToLower(msg.Text + "\n" + msg.Caption)
	for _, r := range rules {
		if r.Kind == consts.RelayRuleKeyword && strings.Contains(content, r.Value) {
			return "消息包含屏蔽关键词，已拦截，未传递。"
		}
	}
	return ""
}

func (s *sSysBot) AdminRelayRuleList(ctx context.Context, in *sysin.RelayListInp) ([]*entity.YoubanBotRelayRule, int, error) {
	if err := in.Filter(); err != nil {
		return nil, 0, err
	}
	list := make([]*entity.YoubanBotRelayRule, 0)
	total := 0
	c := dao.YoubanBotRelayRule.Columns()
	mod := dao.YoubanBotRelayRule.Ctx(ctx).Where(c.BotId, in.BotId)
	if in.Kind != "" {
		mod = mod.Where(c.Kind, in.Kind)
	}
	if in.Keyword != "" {
		mod = mod.WhereLike(c.Label, "%"+in.Keyword+"%")
	}
	err := mod.Page(in.Page, in.PerPage).OrderDesc(c.Id).ScanAndCount(&list, &total, false)
	return list, total, gerror.Wrap(err, "读取屏蔽规则失败")
}

func (s *sSysBot) adminRelaySaveRule(ctx context.Context, in *sysin.RelayRuleInp) error {
	if err := in.Filter(); err != nil {
		return err
	}
	if _, err := s.botById(ctx, in.BotId); err != nil {
		return err
	}
	value, label := strings.ToLower(in.Value), in.Value
	if in.Kind != consts.RelayRuleKeyword {
		if strings.HasPrefix(in.Value, "@") {
			peer, err := s.relayResolvePeer(ctx, in.BotId, in.Value)
			if err != nil {
				return err
			}
			if peer.Kind != in.Kind {
				return gerror.New("屏蔽对象类型不匹配")
			}
			value, label = peer.ID, peer.Name
		} else {
			id, _ := strconv.ParseInt(in.Value, 10, 64)
			if (in.Kind == consts.RelayRuleUser && id <= 0) || (in.Kind == consts.RelayRuleGroup && id >= 0) {
				return gerror.New("屏蔽对象类型不匹配")
			}
		}
	}
	_, err := dao.YoubanBotRelayRule.Ctx(ctx).Data(do.YoubanBotRelayRule{BotId: in.BotId, Kind: in.Kind, Value: value, Label: label}).OnConflict(dao.YoubanBotRelayRule.Columns().BotId, dao.YoubanBotRelayRule.Columns().Kind, dao.YoubanBotRelayRule.Columns().Value).InsertIgnore()
	return gerror.Wrap(err, "保存屏蔽规则失败")
}

func (s *sSysBot) adminRelayDeleteRule(ctx context.Context, in *sysin.RelayDeleteInp) error {
	if err := in.RelayBotInp.Filter(); err != nil {
		return err
	}
	if in.Id <= 0 {
		return gerror.New("请选择屏蔽规则")
	}
	_, err := dao.YoubanBotRelayRule.Ctx(ctx).Where(do.YoubanBotRelayRule{Id: in.Id, BotId: in.BotId}).Delete()
	return gerror.Wrap(err, "删除屏蔽规则失败")
}
