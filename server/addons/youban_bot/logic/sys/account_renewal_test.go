package sys

import (
	"context"
	"fmt"
	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	gs "hotgo/addons/youban_tg_bot_gateway/service"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
)

type accountRenewalTestService struct {
	ps.ISysPublish
	calls         int
	fail          bool
	allowNoCoupon bool
}

func (s *accountRenewalTestService) TenantVipOrderCreate(ctx context.Context, in *pi.TenantVipOrderCreateInp) (*pi.TenantVipOrderModel, error) {
	s.calls++
	if in.PlanCode != "month" || (in.CouponCode != "PROMO" && !(s.allowNoCoupon && in.CouponCode == "")) || in.ProductType != "vip" || in.Quantity != 1 {
		return nil, fmt.Errorf("wrong order input: %+v", in)
	}
	if s.fail {
		return nil, fmt.Errorf("injected payment failure")
	}
	return &pi.TenantVipOrderModel{Id: 42, OrderNo: "renewal42", PayUrl: "https://example.com/pay/42", Amount: 5}, nil
}

func TestAccountRenewalOrderSession(t *testing.T) {
	ctx := relayTestDatabase(t)
	db, _ := gdb.Instance()
	if _, err := db.Exec(ctx, `DROP TABLE IF EXISTS hg_youban_bot_profile_session`); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(ctx, `CREATE TABLE hg_youban_bot_profile_session(id bigint primary key,bot_id bigint,telegram_user_id text,chat_id text,account_id bigint,tenant_id bigint,scene text,status text,payload_json text,expires_at timestamp,updated_at timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	for id := 1; id <= 4; id++ {
		expires := gtime.Now().Add(time.Hour)
		status := "active"
		if id == 3 {
			expires = gtime.Now().Add(-time.Hour)
		}
		if id == 4 {
			status = "canceled"
		}
		_, err = db.Exec(ctx, `INSERT INTO hg_youban_bot_profile_session VALUES(?,7,'11','11',99,1,'account_renewal',?,'{"planCode":"month","couponCode":"PROMO"}',?,NULL)`, id, status, expires)
		if err != nil {
			t.Fatal(err)
		}
	}
	previous := managerPreviousPublish()
	fake := &accountRenewalTestService{ISysPublish: previous}
	ps.RegisterSysPublish(fake)
	t.Cleanup(func() { ps.RegisterSysPublish(previous) })
	s := &sSysBot{}
	account := &botProfileAccount{AccountId: 99, TenantId: 1}
	for _, tc := range []struct {
		name                string
		bot                 int64
		user, chat          string
		tenant, account, id int64
	}{
		{"wrong bot", 8, "11", "11", 1, 99, 1}, {"wrong user", 7, "12", "11", 1, 99, 1}, {"wrong chat", 7, "11", "12", 1, 99, 1}, {"wrong tenant", 7, "11", "11", 2, 99, 1}, {"wrong account", 7, "11", "11", 1, 100, 1}, {"expired", 7, "11", "11", 1, 99, 3}, {"canceled", 7, "11", "11", 1, 99, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := s.accountRenewalOrder(ctx, tc.bot, tc.user, tc.chat, &botProfileAccount{AccountId: tc.account, TenantId: tc.tenant}, tc.id)
			if err == nil || result != nil || fake.calls != 0 {
				t.Fatalf("unauthorized order: %v %v", result, err)
			}
		})
	}
	first, err := s.accountRenewalOrder(ctx, 7, "11", "11", account, 1)
	if err != nil || first == nil {
		t.Fatalf("create: %v %v", first, err)
	}
	again, err := s.accountRenewalOrder(ctx, 7, "11", "11", account, 1)
	if err != nil || again == nil || again.Id != first.Id || fake.calls != 1 {
		t.Fatalf("duplicate payment order: %v %v calls=%d", again, err, fake.calls)
	}
	fake.fail = true
	_, err = s.accountRenewalOrder(ctx, 7, "11", "11", account, 2)
	if err == nil {
		t.Fatal("payment failure swallowed")
	}
	row, err := db.GetOne(ctx, `SELECT status,payload_json FROM hg_youban_bot_profile_session WHERE id=2`)
	if err != nil || row["status"].String() != "active" {
		t.Fatalf("failure consumed session: %v %v", row, err)
	}
	fake.fail = false
	result, err := s.accountRenewalOrder(ctx, 7, "11", "11", account, 2)
	if err != nil || result == nil {
		t.Fatalf("retry after rollback: %v %v", result, err)
	}
}

func (s *accountRenewalTestService) TenantVipPlans(context.Context) ([]*pi.TenantVipPlanModel, error) {
	return []*pi.TenantVipPlanModel{{Code: "month", Name: "月卡", Level: 1, Price: 10, Currency: "USD", Days: 30}}, nil
}
func (s *accountRenewalTestService) TenantVipCouponCheck(_ context.Context, in *pi.TenantVipCouponCheckInp) (*pi.TenantVipCouponCheckModel, error) {
	return &pi.TenantVipCouponCheckModel{Valid: in.Code == "PROMO", Code: in.Code}, nil
}

func TestAccountRenewalBotInteraction(t *testing.T) {
	ctx := relayTestDatabase(t)
	db, _ := gdb.Instance()
	if _, err := db.Exec(ctx, `DROP TABLE IF EXISTS hg_youban_bot_profile_session`); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(ctx, `CREATE TABLE hg_youban_bot_profile_session(id integer primary key autoincrement,bot_id bigint,telegram_user_id text,chat_id text,app text,account_id bigint,tenant_id bigint,scene text,step text,profile_id bigint,profile_no text,status text,payload_json text,expires_at timestamp,created_at timestamp,updated_at timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	previous := managerPreviousPublish()
	fake := &accountRenewalTestService{ISysPublish: previous}
	ps.RegisterSysPublish(fake)
	t.Cleanup(func() { ps.RegisterSysPublish(previous) })
	s := &sSysBot{}
	requests := managerTestHTTP(t, ctx, s)
	var previousGateway gs.IGateway
	func() { defer func() { _ = recover() }(); previousGateway = gs.Gateway() }()
	gs.RegisterGateway(accountRenewalTestGateway{client: s.relayClientForToken})
	t.Cleanup(func() { gs.RegisterGateway(previousGateway) })
	account := &botProfileAccount{AccountId: 99, TenantId: 1, App: "api"}
	if err = s.startAccountRenewal(ctx, 1, "11", "11", account, "invalid"); err == nil {
		t.Fatal("invalid plan accepted")
	}
	if err = s.startAccountRenewal(ctx, 1, "11", "11", account, "month"); err != nil {
		t.Fatal(err)
	}
	text := <-requests
	if !strings.Contains(text, "月卡") || !strings.Contains(text, "不使用优惠码") || !strings.Contains(text, "acct:renew_confirm:") {
		t.Fatal("missing renewal prompt", text)
	}
	session := s.activeProfileSessionByIds(ctx, 1, "11", "11")
	if session == nil {
		t.Fatal("no renewal session")
	}
	if err = s.setAccountRenewalCoupon(ctx, 1, "11", session, account, "INVALID"); err != nil {
		t.Fatal(err)
	}
	if text = <-requests; !strings.Contains(text, "优惠码无效") {
		t.Fatal("invalid coupon feedback missing", text)
	}
	if err = s.setAccountRenewalCoupon(ctx, 1, "11", session, account, "PROMO"); err != nil {
		t.Fatal(err)
	}
	if text = <-requests; !strings.Contains(text, "PROMO") || !strings.Contains(text, "创建续费订单") {
		t.Fatal("missing coupon confirmation", text)
	}
	query := &models.CallbackQuery{From: models.User{ID: 11}, Data: fmt.Sprintf("acct:renew_confirm:%d", session.Id)}
	if err = s.confirmAccountRenewal(ctx, 1, "11", query, account); err != nil {
		t.Fatal(err)
	}
	text = <-requests
	if !strings.Contains(text, "5.00") || !strings.Contains(text, "https://example.com/pay/42") {
		t.Fatal("missing payment amount/link", text)
	}
	if err = s.confirmAccountRenewal(ctx, 1, "11", query, account); err != nil {
		t.Fatal(err)
	}
	<-requests
	if fake.calls != 1 {
		t.Fatalf("repeated callback created %d orders", fake.calls)
	}
	fake.allowNoCoupon = true
	if err = s.startAccountRenewal(ctx, 1, "11", "11", account, "month"); err != nil {
		t.Fatal(err)
	}
	<-requests
	session = s.activeProfileSessionByIds(ctx, 1, "11", "11")
	query.Data = fmt.Sprintf("acct:renew_confirm:%d", session.Id)
	if err = s.confirmAccountRenewal(ctx, 1, "11", query, account); err != nil {
		t.Fatal(err)
	}
	<-requests
	if fake.calls != 2 {
		t.Fatal("skip-coupon path did not create order")
	}
}

type accountRenewalTestGateway struct {
	gs.IGateway
	client func(context.Context, string) (*tgbot.Bot, error)
}

func (g accountRenewalTestGateway) Client(ctx context.Context, token string) (*tgbot.Bot, error) {
	return g.client(ctx, token)
}
