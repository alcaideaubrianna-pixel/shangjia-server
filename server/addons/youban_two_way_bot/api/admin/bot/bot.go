package bot

import (
	"hotgo/addons/youban_two_way_bot/model/input/sysin"

	"github.com/gogf/gf/v2/frame/g"
)

type ListReq struct {
	g.Meta `path:"/twoWayBot/list" method:"get" tags:"双向机器人" summary:"双向机器人列表"`
	sysin.BotListInp
}

type ListRes struct {
	List       []*sysin.BotModel `json:"list" dc:"列表"`
	TotalCount int               `json:"totalCount" dc:"总数"`
}

type SaveReq struct {
	g.Meta `path:"/twoWayBot/save" method:"post" tags:"双向机器人" summary:"保存双向机器人"`
	sysin.BotSaveInp
}

type SaveRes struct{}

type DeleteReq struct {
	g.Meta `path:"/twoWayBot/delete" method:"post" tags:"双向机器人" summary:"删除双向机器人"`
	sysin.BotDeleteInp
}

type DeleteRes struct{}

type RefreshWebhookReq struct {
	g.Meta `path:"/twoWayBot/refreshWebhook" method:"post" tags:"双向机器人" summary:"刷新双向机器人Webhook"`
	sysin.BotActionInp
}

type RefreshWebhookRes struct{}

type SetupReq struct {
	g.Meta `path:"/twoWayBot/setup" method:"post" tags:"双向机器人" summary:"初始化双向机器人"`
	sysin.BotActionInp
}

type SetupRes struct{}
