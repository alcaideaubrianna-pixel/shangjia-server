package sysin

import (
	"context"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/os/gtime"
)

type InviteConfigModel struct {
	Id          int64       `json:"id"`
	Enabled     int         `json:"enabled"`
	BaseUrl     string      `json:"baseUrl"`
	Level1Min   int         `json:"level1Min"`
	Level1Max   int         `json:"level1Max"`
	Level1Rate  float64     `json:"level1Rate"`
	Level2Min   int         `json:"level2Min"`
	Level2Rate  float64     `json:"level2Rate"`
	ManualAudit int         `json:"manualAudit"`
	Remark      string      `json:"remark"`
	UpdatedAt   *gtime.Time `json:"updatedAt"`
}

type InviteConfigSaveInp struct {
	Enabled     int     `json:"enabled" dc:"是否启用"`
	BaseUrl     string  `json:"baseUrl" dc:"邀请链接域名"`
	Level1Min   int     `json:"level1Min" dc:"一档最小单数"`
	Level1Max   int     `json:"level1Max" dc:"一档最大单数"`
	Level1Rate  float64 `json:"level1Rate" dc:"一档返现比例"`
	Level2Min   int     `json:"level2Min" dc:"二档最小单数"`
	Level2Rate  float64 `json:"level2Rate" dc:"二档返现比例"`
	ManualAudit int     `json:"manualAudit" dc:"人工审核"`
	Remark      string  `json:"remark" dc:"备注"`
}

func (in *InviteConfigSaveInp) Filter(ctx context.Context) error {
	if in.Level1Min <= 0 {
		in.Level1Min = 1
	}
	if in.Level1Max < in.Level1Min {
		in.Level1Max = in.Level1Min
	}
	if in.Level2Min <= in.Level1Max {
		in.Level2Min = in.Level1Max + 1
	}
	if in.Level1Rate < 0 {
		in.Level1Rate = 0
	}
	if in.Level2Rate < 0 {
		in.Level2Rate = 0
	}
	return nil
}

type InviteRecordListInp struct {
	form.PageReq
	Keyword      string   `json:"keyword" dc:"关键词"`
	InviterId    int64    `json:"inviterId" dc:"邀请人ID"`
	InviteeId    int64    `json:"inviteeId" dc:"被邀请人ID"`
	SettleStatus string   `json:"settleStatus" dc:"结算状态"`
	CreatedAt    []string `json:"createdAt" dc:"创建时间"`
}

type InviteRecordModel struct {
	Id            int64       `json:"id"`
	InviterId     int64       `json:"inviterId"`
	InviterName   string      `json:"inviterName"`
	InviterMobile string      `json:"inviterMobile"`
	InviteeId     int64       `json:"inviteeId"`
	InviteeName   string      `json:"inviteeName"`
	InviteeMobile string      `json:"inviteeMobile"`
	InviteCode    string      `json:"inviteCode"`
	OrderId       int64       `json:"orderId"`
	OrderSn       string      `json:"orderSn"`
	TradeType     string      `json:"tradeType"`
	OrderAmount   float64     `json:"orderAmount"`
	RebateRate    float64     `json:"rebateRate"`
	RebateAmount  float64     `json:"rebateAmount"`
	SettleStatus  string      `json:"settleStatus"`
	SettledAt     *gtime.Time `json:"settledAt"`
	Remark        string      `json:"remark"`
	CreatedAt     *gtime.Time `json:"createdAt"`
}

type InviteRecordSaveInp struct {
	Id           int64   `json:"id" dc:"ID"`
	InviterId    int64   `json:"inviterId" dc:"邀请人ID"`
	InviteeId    int64   `json:"inviteeId" dc:"被邀请人ID"`
	InviteCode   string  `json:"inviteCode" dc:"邀请码"`
	OrderId      int64   `json:"orderId" dc:"订单ID"`
	OrderSn      string  `json:"orderSn" dc:"订单号"`
	TradeType    string  `json:"tradeType" dc:"交易类型"`
	OrderAmount  float64 `json:"orderAmount" dc:"订单金额"`
	RebateRate   float64 `json:"rebateRate" dc:"返现比例"`
	RebateAmount float64 `json:"rebateAmount" dc:"返现金额"`
	SettleStatus string  `json:"settleStatus" dc:"结算状态"`
	Remark       string  `json:"remark" dc:"备注"`
}

type InviteRecordDeleteInp struct {
	Ids []int64 `json:"ids" v:"required#请选择要删除的数据" dc:"ID"`
}

type InviteLedgerInp struct {
	form.PageReq
}

type InviteStatsModel struct {
	InviteCode       string               `json:"inviteCode"`
	InviteLink       string               `json:"inviteLink"`
	InvitedCount     int                  `json:"invitedCount"`
	OrderCount       int                  `json:"orderCount"`
	SettledAmount    float64              `json:"settledAmount"`
	PendingAmount    float64              `json:"pendingAmount"`
	MonthAmount      float64              `json:"monthAmount"`
	CurrentLevel     string               `json:"currentLevel"`
	CurrentRate      float64              `json:"currentRate"`
	NextLevelMissing int                  `json:"nextLevelMissing"`
	Rules            []*InviteRuleModel   `json:"rules"`
	Trend            []*InviteTrendModel  `json:"trend"`
	LatestLedger     []*InviteRecordModel `json:"latestLedger"`
}

type InviteRuleModel struct {
	Title string  `json:"title"`
	Range string  `json:"range"`
	Rate  float64 `json:"rate"`
}

type InviteTrendModel struct {
	Label  string  `json:"label"`
	Amount float64 `json:"amount"`
}
