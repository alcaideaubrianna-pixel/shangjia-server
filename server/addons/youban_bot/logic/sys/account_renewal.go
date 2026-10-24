package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
)

type accountRenewalDraft struct {
	PlanCode   string                  `json:"planCode"`
	CouponCode string                  `json:"couponCode,omitempty"`
	Order      *pi.TenantVipOrderModel `json:"order,omitempty"`
}

func (s *sSysBot) startAccountRenewal(ctx context.Context, botID int64, userID, chatID string, account *botProfileAccount, planCode string) error {
	plans, err := ps.SysPublish().TenantVipPlans(quickPushMediaUploadContext(ctx, account.AccountId))
	if err != nil {
		return err
	}
	var selected *pi.TenantVipPlanModel
	for _, plan := range plans {
		if plan != nil && plan.Code == planCode && plan.Level > 0 {
			selected = plan
			break
		}
	}
	if selected == nil {
		return gerror.New("套餐已失效，请重新选择。")
	}
	if err = s.startProfileSessionByIds(ctx, botID, userID, chatID, account, "account_renewal", "coupon", 0, "", accountRenewalDraft{PlanCode: planCode}, ""); err != nil {
		return err
	}
	session := s.activeProfileSessionByIds(ctx, botID, userID, chatID)
	if session == nil {
		return gerror.New("续费会话创建失败，请重试。")
	}
	return s.replyWithMarkup(ctx, botID, chatID, fmt.Sprintf("<b>会员续费</b>\n套餐：%s\n价格：%.2f %s\n\n有优惠码请直接发送；没有则点击下方按钮。提交订单前系统会重新校验优惠码。", html.EscapeString(selected.Name), selected.Price, html.EscapeString(selected.Currency)), &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "不使用优惠码，创建订单", CallbackData: fmt.Sprintf("acct:renew_confirm:%d", session.Id)}}, {{Text: "取消", CallbackData: "acct:reset_cancel"}}}})
}

func (s *sSysBot) setAccountRenewalCoupon(ctx context.Context, botID int64, chatID string, session *profileSessionRow, account *botProfileAccount, code string) error {
	code = strings.TrimSpace(code)
	if code == "" || len([]rune(code)) > 128 {
		return s.reply(ctx, botID, chatID, "优惠码不能为空，且不能超过128个字。")
	}
	check, err := ps.SysPublish().TenantVipCouponCheck(quickPushMediaUploadContext(ctx, account.AccountId), &pi.TenantVipCouponCheckInp{Code: code})
	if err != nil {
		return s.reply(ctx, botID, chatID, "校验优惠码失败："+html.EscapeString(err.Error()))
	}
	if check == nil || !check.Valid {
		return s.reply(ctx, botID, chatID, "优惠码无效，请重新输入，或取消后重新选择套餐。")
	}
	var draft accountRenewalDraft
	if err = json.Unmarshal([]byte(session.PayloadJson), &draft); err != nil || draft.PlanCode == "" {
		return gerror.New("续费会话已失效，请重新选择套餐。")
	}
	draft.CouponCode = code
	raw, err := json.Marshal(draft)
	if err != nil {
		return err
	}
	result, err := g.DB().Model(profileSessionTable).Ctx(ctx).Where("id", session.Id).Where("account_id", account.AccountId).Where("tenant_id", account.TenantId).Where("scene", "account_renewal").Where("status", profileSessionStatusActive).WhereGT("expires_at", gtime.Now()).Where("payload_json", session.PayloadJson).Data(g.Map{"payload_json": string(raw), "step": "confirm"}).Update()
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return s.reply(ctx, botID, chatID, "会话已改变，请重新打开会员续费。")
	}
	return s.replyWithMarkup(ctx, botID, chatID, "优惠码已通过校验：<code>"+html.EscapeString(code)+"</code>\n点击创建订单查看最终支付金额。", &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "创建续费订单", CallbackData: fmt.Sprintf("acct:renew_confirm:%d", session.Id)}}, {{Text: "取消", CallbackData: "acct:reset_cancel"}}}})
}

// Claim and order persistence share a transaction. A duplicate confirmation
// returns the persisted order instead of issuing another payment order.
func (s *sSysBot) accountRenewalOrder(ctx context.Context, botID int64, userID, chatID string, account *botProfileAccount, sessionID int64) (*pi.TenantVipOrderModel, error) {
	var order *pi.TenantVipOrderModel
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var session *profileSessionRow
		mod := g.DB().Model(profileSessionTable).Ctx(ctx).Where("id", sessionID).Where("bot_id", botID).Where("telegram_user_id", userID).Where("chat_id", chatID).Where("tenant_id", account.TenantId).Where("account_id", account.AccountId).Where("scene", "account_renewal").WhereGT("expires_at", gtime.Now())
		// UPDATE acquires the row lock before reading; concurrent confirms serialize.
		result, err := mod.Clone().WhereIn("status", []string{profileSessionStatusActive, profileSessionStatusCompleted}).Data(g.Map{"updated_at": gtime.Now()}).Update()
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return gerror.New("续费确认已过期或已取消，请重新选择套餐。")
		}
		if err = mod.Clone().Scan(&session); err != nil {
			return err
		}
		if session == nil {
			return gerror.New("续费会话不存在。")
		}
		var draft accountRenewalDraft
		if err = json.Unmarshal([]byte(session.PayloadJson), &draft); err != nil {
			return err
		}
		if session.Status == profileSessionStatusCompleted {
			if draft.Order == nil {
				return gerror.New("订单记录不完整，请联系管理员。")
			}
			order = draft.Order
			return nil
		}
		if draft.PlanCode == "" {
			return gerror.New("套餐参数无效。")
		}
		order, err = ps.SysPublish().TenantVipOrderCreate(quickPushMediaUploadContext(ctx, account.AccountId), &pi.TenantVipOrderCreateInp{PlanCode: draft.PlanCode, CouponCode: draft.CouponCode, ProductType: "vip", Quantity: 1})
		if err != nil {
			return err
		}
		if order == nil {
			return gerror.New("支付服务未返回订单。")
		}
		draft.Order = order
		raw, err := json.Marshal(draft)
		if err != nil {
			return err
		}
		_, err = mod.Clone().Data(g.Map{"status": profileSessionStatusCompleted, "payload_json": string(raw)}).Update()
		return err
	})
	if err != nil {
		return nil, err
	}
	return order, nil
}

func (s *sSysBot) confirmAccountRenewal(ctx context.Context, botID int64, chatID string, query *models.CallbackQuery, account *botProfileAccount) error {
	id, err := strconv.ParseInt(strings.TrimPrefix(query.Data, "acct:renew_confirm:"), 10, 64)
	if err != nil || id <= 0 {
		return s.reply(ctx, botID, chatID, "续费确认参数无效。")
	}
	order, err := s.accountRenewalOrder(ctx, botID, fmt.Sprint(query.From.ID), chatID, account, id)
	if err != nil {
		return s.reply(ctx, botID, chatID, "创建续费订单失败："+html.EscapeString(err.Error()))
	}
	text := fmt.Sprintf("<b>续费订单</b>\n订单号：<code>%s</code>\n支付金额：%.2f %s\n状态：%s", html.EscapeString(order.OrderNo), order.Amount, html.EscapeString(order.Currency), html.EscapeString(order.StatusTxt))
	rows := [][]models.InlineKeyboardButton{}
	if order.PayUrl != "" {
		rows = append(rows, []models.InlineKeyboardButton{{Text: "打开支付页面", URL: order.PayUrl}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "返回我的账户", CallbackData: "acct:mine"}})
	return s.replyWithMarkup(ctx, botID, chatID, text, &models.InlineKeyboardMarkup{InlineKeyboard: rows})
}
