package invite

import (
	"hotgo/addons/youban_invite/model/input/sysin"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/frame/g"
)

type StatsReq struct {
	g.Meta `path:"/invite/stats" method:"get" tags:"邀请返现" summary:"邀请返现统计"`
}

type StatsRes struct {
	*sysin.InviteStatsModel
}

type LedgerReq struct {
	g.Meta `path:"/invite/ledger" method:"get" tags:"邀请返现" summary:"邀请返现账单"`
	sysin.InviteLedgerInp
}

type LedgerRes struct {
	form.PageRes
	List []*sysin.InviteRecordModel `json:"list" dc:"账单列表"`
}
