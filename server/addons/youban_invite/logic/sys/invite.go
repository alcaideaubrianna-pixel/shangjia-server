package sys

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_invite/model/input/sysin"
	"hotgo/addons/youban_invite/service"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
	"hotgo/internal/model/input/form"
)

const (
	inviteConfigTable = "hg_yb_invite_config"
	inviteRebateTable = "hg_yb_invite_rebate"

	inviteTradeMemberVip = "member_vip"
	inviteStatusPending  = "pending"
	inviteStatusSettled  = "settled"
	inviteStatusCancel   = "cancelled"
)

type sSysInvite struct{}

type inviteOrderRow struct {
	Id          int64       `json:"id"`
	MemberId    int64       `json:"member_id"`
	OrderSn     string      `json:"order_sn"`
	Money       float64     `json:"money"`
	Status      int         `json:"status"`
	CreatedAt   *gtime.Time `json:"created_at"`
	InviterId   int64       `json:"inviter_id"`
	InviteCode  string      `json:"invite_code"`
	InviterCode string      `json:"inviter_code"`
}

func NewSysInvite() *sSysInvite {
	return &sSysInvite{}
}

func init() {
	service.RegisterSysInvite(NewSysInvite())
}

func (s *sSysInvite) Stats(ctx context.Context) (res *sysin.InviteStatsModel, err error) {
	memberId := contexts.GetUserId(ctx)
	if memberId <= 0 {
		err = gerror.New("请先登录")
		return
	}
	if err = s.syncVipRebates(ctx, memberId); err != nil {
		return
	}

	config, err := s.getConfig(ctx)
	if err != nil {
		return
	}
	member, err := s.memberBrief(ctx, memberId)
	if err != nil {
		return
	}
	orderCount, settledAmount, pendingAmount, monthAmount, err := s.statAmounts(ctx, memberId)
	if err != nil {
		return
	}
	invitedCount, err := dao.AdminMember.Ctx(ctx).Where(dao.AdminMember.Columns().Pid, memberId).Count()
	if err != nil {
		err = gerror.Wrap(err, "获取邀请人数失败")
		return
	}
	rate := s.rateForOrder(config, orderCount)
	res = &sysin.InviteStatsModel{
		InviteCode:       member["invite_code"].String(),
		InviteLink:       s.inviteLink(config.BaseUrl, member["invite_code"].String()),
		InvitedCount:     invitedCount,
		OrderCount:       orderCount,
		SettledAmount:    settledAmount,
		PendingAmount:    pendingAmount,
		MonthAmount:      monthAmount,
		CurrentLevel:     s.levelLabel(config, orderCount),
		CurrentRate:      rate,
		NextLevelMissing: s.nextLevelMissing(config, orderCount),
		Rules: []*sysin.InviteRuleModel{
			{Title: "第1档", Range: fmt.Sprintf("%d-%d单", config.Level1Min, config.Level1Max), Rate: config.Level1Rate},
			{Title: "第2档", Range: fmt.Sprintf("%d单起", config.Level2Min), Rate: config.Level2Rate},
		},
	}
	res.Trend, err = s.trend(ctx, memberId)
	if err != nil {
		return
	}
	res.LatestLedger, _, err = s.listRecords(ctx, &sysin.InviteRecordListInp{
		PageReq:   form.PageReq{Page: 1, PerPage: 5},
		InviterId: memberId,
	})
	return
}

func (s *sSysInvite) Ledger(ctx context.Context, in *sysin.InviteLedgerInp) (list []*sysin.InviteRecordModel, totalCount int, err error) {
	memberId := contexts.GetUserId(ctx)
	if memberId <= 0 {
		err = gerror.New("请先登录")
		return
	}
	if err = s.syncVipRebates(ctx, memberId); err != nil {
		return
	}
	return s.listRecords(ctx, &sysin.InviteRecordListInp{
		PageReq:   in.PageReq,
		InviterId: memberId,
	})
}

func (s *sSysInvite) AdminConfig(ctx context.Context) (res *sysin.InviteConfigModel, err error) {
	return s.getConfig(ctx)
}

func (s *sSysInvite) AdminSaveConfig(ctx context.Context, in *sysin.InviteConfigSaveInp) (err error) {
	if err = in.Filter(ctx); err != nil {
		return
	}
	_, err = g.DB().Model(inviteConfigTable).Safe().Ctx(ctx).
		Where("id", 1).
		Data(g.Map{
			"enabled":      in.Enabled,
			"base_url":     strings.TrimSpace(in.BaseUrl),
			"level1_min":   in.Level1Min,
			"level1_max":   in.Level1Max,
			"level1_rate":  in.Level1Rate,
			"level2_min":   in.Level2Min,
			"level2_rate":  in.Level2Rate,
			"manual_audit": in.ManualAudit,
			"remark":       in.Remark,
			"updated_at":   gtime.Now(),
		}).
		Update()
	if err != nil {
		err = gerror.Wrap(err, "保存邀请返现配置失败")
	}
	return
}

func (s *sSysInvite) AdminList(ctx context.Context, in *sysin.InviteRecordListInp) (list []*sysin.InviteRecordModel, totalCount int, err error) {
	if err = s.syncVipRebates(ctx, 0); err != nil {
		return
	}
	return s.listRecords(ctx, in)
}

func (s *sSysInvite) AdminSaveRecord(ctx context.Context, in *sysin.InviteRecordSaveInp) (err error) {
	if in.InviterId <= 0 {
		return gerror.New("邀请人ID不能为空")
	}
	if in.InviteeId <= 0 {
		return gerror.New("被邀请人ID不能为空")
	}
	if in.TradeType == "" {
		in.TradeType = inviteTradeMemberVip
	}
	if in.SettleStatus == "" {
		in.SettleStatus = inviteStatusSettled
	}
	if !validSettleStatus(in.SettleStatus) {
		return gerror.New("结算状态不合法")
	}
	if in.RebateAmount <= 0 && in.OrderAmount > 0 && in.RebateRate > 0 {
		in.RebateAmount = roundMoney(in.OrderAmount * in.RebateRate / 100)
	}
	data := g.Map{
		"inviter_id":    in.InviterId,
		"invitee_id":    in.InviteeId,
		"invite_code":   in.InviteCode,
		"order_id":      in.OrderId,
		"order_sn":      in.OrderSn,
		"trade_type":    in.TradeType,
		"order_amount":  in.OrderAmount,
		"rebate_rate":   in.RebateRate,
		"rebate_amount": in.RebateAmount,
		"settle_status": in.SettleStatus,
		"remark":        in.Remark,
		"updated_by":    contexts.GetUserId(ctx),
		"updated_at":    gtime.Now(),
	}
	if in.SettleStatus == inviteStatusSettled {
		data["settled_at"] = gtime.Now()
	}
	if in.Id > 0 {
		_, err = g.DB().Model(inviteRebateTable).Safe().Ctx(ctx).
			Where("id", in.Id).
			WhereNull("deleted_at").
			Data(data).
			Update()
	} else {
		data["created_by"] = contexts.GetUserId(ctx)
		data["created_at"] = gtime.Now()
		_, err = g.DB().Model(inviteRebateTable).Safe().Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		err = gerror.Wrap(err, "保存邀请返现记录失败")
	}
	return
}

func (s *sSysInvite) AdminDelete(ctx context.Context, in *sysin.InviteRecordDeleteInp) (err error) {
	if len(in.Ids) == 0 {
		return gerror.New("请选择要删除的数据")
	}
	_, err = g.DB().Model(inviteRebateTable).Safe().Ctx(ctx).
		WhereIn("id", in.Ids).
		Data(g.Map{
			"deleted_at": gtime.Now(),
			"deleted_by": contexts.GetUserId(ctx),
		}).
		Update()
	if err != nil {
		err = gerror.Wrap(err, "删除邀请返现记录失败")
	}
	return
}

func (s *sSysInvite) getConfig(ctx context.Context) (res *sysin.InviteConfigModel, err error) {
	err = g.DB().Model(inviteConfigTable).Safe().Ctx(ctx).Where("id", 1).WhereNull("deleted_at").Scan(&res)
	if err != nil {
		err = gerror.Wrap(err, "获取邀请返现配置失败")
		return
	}
	if res != nil {
		return
	}
	now := gtime.Now()
	_, err = g.DB().Model(inviteConfigTable).Safe().Ctx(ctx).Data(g.Map{
		"id":           1,
		"enabled":      1,
		"base_url":     "https://yuebanby.com",
		"level1_min":   1,
		"level1_max":   5,
		"level1_rate":  2,
		"level2_min":   6,
		"level2_rate":  3,
		"manual_audit": 0,
		"remark":       "默认邀请返现配置",
		"created_at":   now,
		"updated_at":   now,
	}).Insert()
	if err != nil {
		err = gerror.Wrap(err, "初始化邀请返现配置失败")
		return
	}
	return s.getConfig(ctx)
}

func (s *sSysInvite) syncVipRebates(ctx context.Context, inviterId int64) (err error) {
	config, err := s.getConfig(ctx)
	if err != nil || config.Enabled != 1 {
		return
	}
	orderCols := dao.AdminOrder.Columns()
	memberCols := dao.AdminMember.Columns()
	mod := g.DB().Model(dao.AdminOrder.Table()).Safe().Ctx(ctx).As("o").
		LeftJoin(dao.AdminMember.Table()+" m", "m."+memberCols.Id+"=o."+orderCols.MemberId).
		LeftJoin(dao.AdminMember.Table()+" inviter", "inviter."+memberCols.Id+"=m."+memberCols.Pid).
		Fields("o.id,o.member_id,o.order_sn,o.money,o.status,o.created_at,m.pid AS inviter_id,m.invite_code,inviter.invite_code AS inviter_code").
		Where("o."+orderCols.OrderType, consts.OrderTypeMemberVip).
		Where("o."+orderCols.Status, consts.OrderStatusDone).
		WhereGT("m."+memberCols.Pid, 0).
		OrderAsc("o." + orderCols.Id).
		Limit(200)
	if inviterId > 0 {
		mod = mod.Where("m."+memberCols.Pid, inviterId)
	}
	var orders []*inviteOrderRow
	if err = mod.Scan(&orders); err != nil {
		return gerror.Wrap(err, "同步会员邀请返现失败")
	}
	for _, order := range orders {
		if order == nil || order.OrderSn == "" || order.InviterId <= 0 {
			continue
		}
		exists, countErr := g.DB().Model(inviteRebateTable).Safe().Ctx(ctx).
			Where("trade_type", inviteTradeMemberVip).
			Where("order_sn", order.OrderSn).
			WhereNull("deleted_at").
			Count()
		if countErr != nil {
			return gerror.Wrap(countErr, "检查邀请返现记录失败")
		}
		if exists > 0 {
			continue
		}
		if err = s.createOrderRebate(ctx, config, order); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysInvite) createOrderRebate(ctx context.Context, config *sysin.InviteConfigModel, order *inviteOrderRow) error {
	status := inviteStatusSettled
	if config.ManualAudit == 1 {
		status = inviteStatusPending
	}
	orderCount, err := g.DB().Model(inviteRebateTable).Safe().Ctx(ctx).
		Where("inviter_id", order.InviterId).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "获取邀请返现档位失败")
	}
	rate := s.rateForOrder(config, orderCount+1)
	now := gtime.Now()
	data := g.Map{
		"inviter_id":    order.InviterId,
		"invitee_id":    order.MemberId,
		"invite_code":   order.InviterCode,
		"order_id":      order.Id,
		"order_sn":      order.OrderSn,
		"trade_type":    inviteTradeMemberVip,
		"order_amount":  order.Money,
		"rebate_rate":   rate,
		"rebate_amount": roundMoney(order.Money * rate / 100),
		"settle_status": status,
		"remark":        "会员认证订单自动返现",
		"created_at":    now,
		"updated_at":    now,
	}
	if status == inviteStatusSettled {
		data["settled_at"] = now
	}
	_, err = g.DB().Model(inviteRebateTable).Safe().Ctx(ctx).Data(data).Insert()
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		return nil
	}
	if err != nil {
		return gerror.Wrap(err, "创建邀请返现记录失败")
	}
	return nil
}

func (s *sSysInvite) listRecords(ctx context.Context, in *sysin.InviteRecordListInp) (list []*sysin.InviteRecordModel, totalCount int, err error) {
	if in.Page <= 0 {
		in.Page = 1
	}
	if in.PerPage <= 0 {
		in.PerPage = 10
	}
	memberCols := dao.AdminMember.Columns()
	mod := g.DB().Model(inviteRebateTable).Safe().Ctx(ctx).As("r").
		LeftJoin(dao.AdminMember.Table()+" inviter", "inviter."+memberCols.Id+"=r.inviter_id").
		LeftJoin(dao.AdminMember.Table()+" invitee", "invitee."+memberCols.Id+"=r.invitee_id").
		WhereNull("r.deleted_at")
	if in.InviterId > 0 {
		mod = mod.Where("r.inviter_id", in.InviterId)
	}
	if in.InviteeId > 0 {
		mod = mod.Where("r.invitee_id", in.InviteeId)
	}
	if in.SettleStatus != "" {
		mod = mod.Where("r.settle_status", in.SettleStatus)
	}
	if in.Keyword != "" {
		keyword := "%" + strings.TrimSpace(in.Keyword) + "%"
		mod = mod.WhereLike("r.order_sn", keyword).
			WhereOrLike("inviter.username", keyword).
			WhereOrLike("inviter.mobile", keyword).
			WhereOrLike("invitee.username", keyword).
			WhereOrLike("invitee.mobile", keyword)
	}
	if len(in.CreatedAt) == 2 && in.CreatedAt[0] != "" && in.CreatedAt[1] != "" {
		mod = mod.WhereBetween("r.created_at", in.CreatedAt[0], in.CreatedAt[1])
	}
	totalCount, err = mod.Count()
	if err != nil {
		err = gerror.Wrap(err, "获取邀请返现记录数量失败")
		return
	}
	if totalCount == 0 {
		list = []*sysin.InviteRecordModel{}
		return
	}
	fields := []string{
		"r.id,r.inviter_id,r.invitee_id,r.invite_code,r.order_id,r.order_sn,r.trade_type",
		"r.order_amount,r.rebate_rate,r.rebate_amount,r.settle_status,r.settled_at,r.remark,r.created_at",
		"inviter.username AS inviter_name,inviter.mobile AS inviter_mobile",
		"invitee.username AS invitee_name,invitee.mobile AS invitee_mobile",
	}
	err = mod.Fields(fields).Page(in.Page, in.PerPage).OrderDesc("r.id").Scan(&list)
	if err != nil {
		err = gerror.Wrap(err, "获取邀请返现记录失败")
	}
	return
}

func (s *sSysInvite) memberBrief(ctx context.Context, memberId int64) (gdb.Record, error) {
	memberCols := dao.AdminMember.Columns()
	record, err := dao.AdminMember.Ctx(ctx).
		Fields(memberCols.Id, memberCols.InviteCode).
		Where(memberCols.Id, memberId).
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "获取会员邀请码失败")
	}
	if record == nil {
		return nil, gerror.New("会员不存在")
	}
	return record, nil
}

func (s *sSysInvite) statAmounts(ctx context.Context, inviterId int64) (orderCount int, settledAmount float64, pendingAmount float64, monthAmount float64, err error) {
	var rows []*sysin.InviteRecordModel
	err = g.DB().Model(inviteRebateTable).Safe().Ctx(ctx).
		Fields("rebate_amount,settle_status,created_at").
		Where("inviter_id", inviterId).
		WhereNull("deleted_at").
		Scan(&rows)
	if err != nil {
		err = gerror.Wrap(err, "获取邀请返现统计失败")
		return
	}
	orderCount = len(rows)
	monthStart := time.Now().Format("2006-01")
	for _, row := range rows {
		if row.SettleStatus == inviteStatusSettled {
			settledAmount += row.RebateAmount
			if row.CreatedAt != nil && strings.HasPrefix(row.CreatedAt.Format("Y-m-d"), monthStart) {
				monthAmount += row.RebateAmount
			}
			continue
		}
		if row.SettleStatus == inviteStatusPending {
			pendingAmount += row.RebateAmount
		}
	}
	return
}

func (s *sSysInvite) trend(ctx context.Context, inviterId int64) (list []*sysin.InviteTrendModel, err error) {
	now := time.Now()
	amounts := map[string]float64{}
	for i := 5; i >= 0; i-- {
		label := now.AddDate(0, -i, 0).Format("01月")
		amounts[label] = 0
		list = append(list, &sysin.InviteTrendModel{Label: label})
	}
	var rows []*sysin.InviteRecordModel
	err = g.DB().Model(inviteRebateTable).Safe().Ctx(ctx).
		Fields("rebate_amount,created_at").
		Where("inviter_id", inviterId).
		Where("settle_status", inviteStatusSettled).
		WhereNull("deleted_at").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "获取邀请返现趋势失败")
	}
	for _, row := range rows {
		if row.CreatedAt == nil {
			continue
		}
		label := row.CreatedAt.Time.Format("01月")
		if _, ok := amounts[label]; ok {
			amounts[label] += row.RebateAmount
		}
	}
	for _, item := range list {
		item.Amount = roundMoney(amounts[item.Label])
	}
	return
}

func (s *sSysInvite) inviteLink(baseUrl string, inviteCode string) string {
	baseUrl = strings.TrimRight(strings.TrimSpace(baseUrl), "/")
	if baseUrl == "" {
		baseUrl = "https://yuebanby.com"
	}
	if inviteCode == "" {
		return baseUrl
	}
	return fmt.Sprintf("%s/#/pages/auth/register?inviteCode=%s", baseUrl, inviteCode)
}

func (s *sSysInvite) rateForOrder(config *sysin.InviteConfigModel, orderCount int) float64 {
	if orderCount >= config.Level2Min {
		return config.Level2Rate
	}
	return config.Level1Rate
}

func (s *sSysInvite) levelLabel(config *sysin.InviteConfigModel, orderCount int) string {
	if orderCount >= config.Level2Min {
		return "第2档"
	}
	return "第1档"
}

func (s *sSysInvite) nextLevelMissing(config *sysin.InviteConfigModel, orderCount int) int {
	if orderCount >= config.Level2Min {
		return 0
	}
	return config.Level2Min - orderCount
}

func roundMoney(v float64) float64 {
	return math.Round(v*100) / 100
}

func validSettleStatus(status string) bool {
	return status == inviteStatusPending || status == inviteStatusSettled || status == inviteStatusCancel
}
