package sys

import (
	"context"

	"hotgo/addons/youban_bot/api/admin/relay"
	"hotgo/addons/youban_bot/service"
)

var Relay = cRelay{}

type cRelay struct{}

func (c *cRelay) Bots(ctx context.Context, req *relay.BotsReq) (res *relay.BotsRes, err error) {
	list, total, err := service.SysBot().AdminRelayBotList(ctx, &req.BotListInp)
	if err != nil {
		return nil, err
	}
	return &relay.BotsRes{List: list, TotalCount: total}, nil
}

func (c *cRelay) Settings(ctx context.Context, req *relay.SettingsReq) (res *relay.SettingsRes, err error) {
	item, err := service.SysBot().AdminRelaySettings(ctx, &req.RelayBotInp)
	if err != nil {
		return nil, err
	}
	return &relay.SettingsRes{RelaySettingsModel: item}, nil
}

func (c *cRelay) SaveSettings(ctx context.Context, req *relay.SaveSettingsReq) (res *relay.SaveSettingsRes, err error) {
	err = service.SysBot().AdminRelaySaveSettings(ctx, &req.RelaySettingsInp)
	return &relay.SaveSettingsRes{}, err
}

func (c *cRelay) Bindings(ctx context.Context, req *relay.BindingsReq) (res *relay.BindingsRes, err error) {
	list, total, err := service.SysBot().AdminRelayBindingList(ctx, &req.RelayListInp)
	if err != nil {
		return nil, err
	}
	return &relay.BindingsRes{List: list, TotalCount: total}, nil
}

func (c *cRelay) Bind(ctx context.Context, req *relay.BindReq) (res *relay.BindRes, err error) {
	err = service.SysBot().AdminRelayBind(ctx, &req.RelayBindingInp)
	return &relay.BindRes{}, err
}

func (c *cRelay) Unbind(ctx context.Context, req *relay.UnbindReq) (res *relay.UnbindRes, err error) {
	err = service.SysBot().AdminRelayUnbind(ctx, &req.RelayDeleteInp)
	return &relay.UnbindRes{}, err
}

func (c *cRelay) Rules(ctx context.Context, req *relay.RulesReq) (res *relay.RulesRes, err error) {
	list, total, err := service.SysBot().AdminRelayRuleList(ctx, &req.RelayListInp)
	if err != nil {
		return nil, err
	}
	return &relay.RulesRes{List: list, TotalCount: total}, nil
}

func (c *cRelay) SaveRule(ctx context.Context, req *relay.SaveRuleReq) (res *relay.SaveRuleRes, err error) {
	err = service.SysBot().AdminRelaySaveRule(ctx, &req.RelayRuleInp)
	return &relay.SaveRuleRes{}, err
}

func (c *cRelay) DeleteRule(ctx context.Context, req *relay.DeleteRuleReq) (res *relay.DeleteRuleRes, err error) {
	err = service.SysBot().AdminRelayDeleteRule(ctx, &req.RelayDeleteInp)
	return &relay.DeleteRuleRes{}, err
}

func (c *cRelay) Records(ctx context.Context, req *relay.RecordsReq) (res *relay.RecordsRes, err error) {
	list, total, err := service.SysBot().AdminRelayRecordList(ctx, &req.RelayListInp)
	if err != nil {
		return nil, err
	}
	return &relay.RecordsRes{List: list, TotalCount: total}, nil
}

func (c *cRelay) Switch(ctx context.Context, req *relay.SwitchReq) (res *relay.SwitchRes, err error) {
	err = service.SysBot().AdminRelaySwitch(ctx, &req.RelaySwitchInp)
	return &relay.SwitchRes{}, err
}
