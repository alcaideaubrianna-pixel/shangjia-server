package sys

import (
	"context"
	"strings"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	"hotgo/addons/youban_bot/model/input/sysin"

	"github.com/gogf/gf/v2/errors/gerror"
)

func (s *sSysBot) relayFeatureRelations(ctx context.Context, botID int64, model *sysin.FeatureModel) error {
	var bindings []*entity.YoubanBotRelayBinding
	if err := dao.YoubanBotRelayBinding.Ctx(ctx).Where(dao.YoubanBotRelayBinding.Columns().BotId, botID).OrderAsc(dao.YoubanBotRelayBinding.Columns().Id).Scan(&bindings); err != nil {
		return gerror.Wrap(err, "读取插件双向列表失败")
	}
	pairs := make([]string, 0, len(bindings))
	for _, b := range bindings {
		pairs = append(pairs, b.LeftChatId+" "+b.RightChatId)
	}
	model.ConfigValues["bindings"] = strings.Join(pairs, "\n")
	var rules []*entity.YoubanBotRelayRule
	if err := dao.YoubanBotRelayRule.Ctx(ctx).Where(dao.YoubanBotRelayRule.Columns().BotId, botID).WhereIn(dao.YoubanBotRelayRule.Columns().Kind, []string{consts.RelayRuleUser, consts.RelayRuleGroup}).OrderAsc(dao.YoubanBotRelayRule.Columns().Id).Scan(&rules); err != nil {
		return gerror.Wrap(err, "读取插件黑名单失败")
	}
	users, groups := []string{}, []string{}
	for _, r := range rules {
		if r.Kind == consts.RelayRuleUser {
			users = append(users, r.Value)
		} else {
			groups = append(groups, r.Value)
		}
	}
	model.ConfigValues["blacklistUsers"] = strings.Join(users, "\n")
	model.ConfigValues["blacklistGroups"] = strings.Join(groups, "\n")
	return nil
}

// Existing generic plugin forms can edit these lists without frontend code changes.
// Missing fields in a direct settings request leave their corresponding lists untouched.
func (s *sSysBot) saveRelayFeatureRelations(ctx context.Context, botID int64, values map[string]interface{}) error {
	base := sysin.RelayBotInp{BotId: botID}
	for field, kind := range map[string]string{"blacklistUsers": consts.RelayRuleUser, "blacklistGroups": consts.RelayRuleGroup} {
		value, exists := values[field]
		if !exists {
			continue
		}
		lines := relayConfigLines(value)
		if len(lines) > 500 {
			return gerror.New("单类黑名单最多500个对象")
		}
		if _, err := dao.YoubanBotRelayRule.Ctx(ctx).Where(do.YoubanBotRelayRule{BotId: botID, Kind: kind}).Delete(); err != nil {
			return err
		}
		for _, line := range lines {
			if err := s.AdminRelaySaveRule(ctx, &sysin.RelayRuleInp{RelayBotInp: base, Kind: kind, Value: line}); err != nil {
				return err
			}
		}
	}
	value, exists := values["bindings"]
	if !exists {
		return nil
	}
	lines := relayConfigLines(value)
	if len(lines) > 100 {
		return gerror.New("单Bot最多配置100对双向绑定")
	}
	desired := map[string][2]string{}
	endpoints := map[string]string{}
	for _, line := range lines {
		a, b, err := relayBindArguments(line)
		if err != nil {
			return err
		}
		left, err := s.relayResolvePeer(ctx, botID, a)
		if err != nil {
			return err
		}
		right, err := s.relayResolvePeer(ctx, botID, b)
		if err != nil {
			return err
		}
		if left.ID == right.ID {
			return gerror.New("不能绑定相同聊天对象")
		}
		key := relayPairKey(left.ID, right.ID)
		for _, id := range []string{left.ID, right.ID} {
			if pair, ok := endpoints[id]; ok && pair != key {
				return gerror.New("聊天对象在绑定列表中出现多次：" + id)
			}
			endpoints[id] = key
		}
		desired[key] = [2]string{left.ID, right.ID}
	}
	var existing []*entity.YoubanBotRelayBinding
	if err := dao.YoubanBotRelayBinding.Ctx(ctx).Where(dao.YoubanBotRelayBinding.Columns().BotId, botID).Scan(&existing); err != nil {
		return err
	}
	for _, binding := range existing {
		if _, keep := desired[binding.PairKey]; keep {
			delete(desired, binding.PairKey)
			continue
		}
		if err := s.AdminRelayUnbind(ctx, &sysin.RelayDeleteInp{RelayBotInp: base, Id: binding.Id}); err != nil {
			return err
		}
	}
	for _, pair := range desired {
		if err := s.AdminRelayBind(ctx, &sysin.RelayBindingInp{RelayBotInp: base, Left: pair[0], Right: pair[1]}); err != nil {
			return err
		}
	}
	return nil
}
