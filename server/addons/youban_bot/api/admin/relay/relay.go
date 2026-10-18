package relay

import (
	"hotgo/addons/youban_bot/internal/model/entity"
	"hotgo/addons/youban_bot/model/input/sysin"

	"github.com/gogf/gf/v2/frame/g"
)

type BotsReq struct {
	g.Meta `path:"/relay/bots" method:"get" tags:"双向机器人" summary:"Bot列表"`
	sysin.BotListInp
}
type BotsRes struct {
	List       []*sysin.RelayBotModel `json:"list"`
	TotalCount int                    `json:"totalCount"`
}

type SettingsReq struct {
	g.Meta `path:"/relay/settings" method:"get" tags:"双向机器人" summary:"查看设置"`
	sysin.RelayBotInp
}
type SettingsRes struct{ *sysin.RelaySettingsModel }

type SaveSettingsReq struct {
	g.Meta `path:"/relay/saveSettings" method:"post" tags:"双向机器人" summary:"保存设置"`
	sysin.RelaySettingsInp
}
type SaveSettingsRes struct{}

type BindingsReq struct {
	g.Meta `path:"/relay/bindings" method:"get" tags:"双向机器人" summary:"双向绑定列表"`
	sysin.RelayListInp
}
type BindingsRes struct {
	List       []*entity.YoubanBotRelayBinding `json:"list"`
	TotalCount int                             `json:"totalCount"`
}

type BindReq struct {
	g.Meta `path:"/relay/bind" method:"post" tags:"双向机器人" summary:"新增双向绑定"`
	sysin.RelayBindingInp
}
type BindRes struct{}

type UnbindReq struct {
	g.Meta `path:"/relay/unbind" method:"post" tags:"双向机器人" summary:"解除双向绑定"`
	sysin.RelayDeleteInp
}
type UnbindRes struct{}

type RulesReq struct {
	g.Meta `path:"/relay/rules" method:"get" tags:"双向机器人" summary:"屏蔽规则列表"`
	sysin.RelayListInp
}
type RulesRes struct {
	List       []*entity.YoubanBotRelayRule `json:"list"`
	TotalCount int                          `json:"totalCount"`
}

type SaveRuleReq struct {
	g.Meta `path:"/relay/saveRule" method:"post" tags:"双向机器人" summary:"添加屏蔽规则"`
	sysin.RelayRuleInp
}
type SaveRuleRes struct{}

type DeleteRuleReq struct {
	g.Meta `path:"/relay/deleteRule" method:"post" tags:"双向机器人" summary:"删除屏蔽规则"`
	sysin.RelayDeleteInp
}
type DeleteRuleRes struct{}

type RecordsReq struct {
	g.Meta `path:"/relay/records" method:"get" tags:"双向机器人" summary:"聊天审计记录"`
	sysin.RelayListInp
}
type RecordsRes struct {
	List       []*entity.YoubanBotRelayDelivery `json:"list"`
	TotalCount int                              `json:"totalCount"`
}

type SwitchReq struct {
	g.Meta `path:"/relay/switch" method:"post" tags:"双向机器人" summary:"启停双向机器人"`
	sysin.RelaySwitchInp
}
type SwitchRes struct{}
