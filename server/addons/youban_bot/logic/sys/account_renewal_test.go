package sys

import (
	"context"
	"encoding/json"
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

// Models a locally committed payment identity; this must survive a later Bot
// session-write error, just as the actual Publish/Pay integration test proves.
type durableRenewalTestService struct {
	ps.ISysPublish
	afterCreate func(context.Context) error
	calls       int
	keys        []string
}

func (s *durableRenewalTestService) TenantVipOrderCreate(ctx context.Context, in *pi.TenantVipOrderCreateInp) (*pi.TenantVipOrderModel, error) {
	s.calls++
	s.keys = append(s.keys, in.RequestKey)
	if in.RequestKey == "" {
		return nil, fmt.Errorf("missing renewal identity")
	}
	db, _ := gdb.Instance()
	if _, err := db.Exec(ctx, `INSERT OR IGNORE INTO renewal_orders(request_key)VALUES(?)`, in.RequestKey); err != nil {
		return nil, err
	}
	row, err := db.GetOne(ctx, `SELECT id FROM renewal_orders WHERE request_key=?`, in.RequestKey)
	if err != nil {
		return nil, err
	}
	if s.afterCreate != nil {
		if err := s.afterCreate(ctx); err != nil {
			return nil, err
		}
	}
	return &pi.TenantVipOrderModel{Id: row["id"].Int64(), OrderNo: in.RequestKey, PayUrl: "https://example.com/payment"}, nil
}

func TestAccountRenewalSessionWriteFailureRecovery(t *testing.T) {
	ctx := relayTestDatabase(t)
	db, _ := gdb.Instance()
	for _, sql := range []string{
		`DROP TABLE IF EXISTS hg_youban_bot_profile_session`,
		`DROP TABLE IF EXISTS renewal_orders`,
		`CREATE TABLE renewal_orders(id integer primary key autoincrement,request_key text UNIQUE)`,
		`CREATE TABLE hg_youban_bot_profile_session(id bigint primary key,bot_id bigint,telegram_user_id text,chat_id text,account_id bigint,tenant_id bigint,scene text,status text,payload_json text,expires_at timestamp,updated_at timestamp)`,
		`CREATE TRIGGER fail_session_completion BEFORE UPDATE ON hg_youban_bot_profile_session WHEN NEW.status='completed' BEGIN SELECT RAISE(ABORT,'injected completion write failure'); END`,
	} {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO hg_youban_bot_profile_session VALUES(1,7,'11','11',99,1,'account_renewal','active','{"planCode":"vip_month"}',?,NULL)`, gtime.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	previous := managerPreviousPublish()
	fake := &durableRenewalTestService{ISysPublish: previous}
	ps.RegisterSysPublish(fake)
	t.Cleanup(func() { ps.RegisterSysPublish(previous) })
	s := &sSysBot{}
	account := &botProfileAccount{AccountId: 99, TenantId: 1}
	if _, err := s.accountRenewalOrder(ctx, 7, "11", "11", account, 1); err == nil {
		t.Fatal("completion write error hidden")
	}
	count, err := db.GetCount(ctx, `SELECT COUNT(*) FROM renewal_orders`)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("session rollback lost payment identity")
	}
	if _, err := s.accountRenewalOrder(ctx, 7, "11", "11", account, 1); err == nil || fake.calls != 1 {
		t.Fatal("live lease permitted duplicate payment call")
	}
	row, err := db.GetOne(ctx, `SELECT payload_json,status FROM hg_youban_bot_profile_session WHERE id=1`)
	if err != nil {
		t.Fatal(err)
	}
	var draft accountRenewalDraft
	if err := json.Unmarshal([]byte(row["payload_json"].String()), &draft); err != nil {
		t.Fatal(err)
	}
	if row["status"].String() != "processing" || draft.LeaseToken == "" {
		t.Fatal("missing recoverable claim")
	}
	draft.LeaseUntil = time.Now().Add(-time.Minute).Unix()
	raw, _ := json.Marshal(draft)
	if _, err := db.Exec(ctx, `UPDATE hg_youban_bot_profile_session SET payload_json=? WHERE id=1`, string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `DROP TRIGGER fail_session_completion`); err != nil {
		t.Fatal(err)
	}
	result, err := s.accountRenewalOrder(ctx, 7, "11", "11", account, 1)
	if err != nil || result == nil {
		t.Fatalf("expired claim did not recover: %v", err)
	}
	if fake.calls != 2 || fake.keys[0] != fake.keys[1] {
		t.Fatalf("recovery changed identity: %v", fake.keys)
	}
	count, err = db.GetCount(ctx, `SELECT COUNT(*) FROM renewal_orders`)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("recovery created duplicate payment")
	}
	again, err := s.accountRenewalOrder(ctx, 7, "11", "11", account, 1)
	if err != nil || again == nil || again.Id != result.Id || fake.calls != 2 {
		t.Fatalf("completed confirmation repeated creation: %v", err)
	}
	// A stale worker must not overwrite a replacement claim's token or result.
	if _, err := db.Exec(ctx, `INSERT INTO hg_youban_bot_profile_session VALUES(2,7,'11','11',99,1,'account_renewal','active','{"planCode":"vip_month"}',?,NULL)`, gtime.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	fake.afterCreate = func(ctx context.Context) error {
		replacement := accountRenewalDraft{PlanCode: "vip_month", LeaseToken: "replacement-worker", LeaseUntil: time.Now().Add(2 * time.Minute).Unix()}
		raw, _ := json.Marshal(replacement)
		_, err := db.Exec(ctx, `UPDATE hg_youban_bot_profile_session SET payload_json=? WHERE id=2`, string(raw))
		return err
	}
	if _, err := s.accountRenewalOrder(ctx, 7, "11", "11", account, 2); err == nil {
		t.Fatal("stale completion overwritten replacement claim")
	}
	latest, err := db.GetOne(ctx, `SELECT status,payload_json FROM hg_youban_bot_profile_session WHERE id=2`)
	if err != nil {
		t.Fatal(err)
	}
	if latest["status"].String() != "processing" || !strings.Contains(latest["payload_json"].String(), "replacement-worker") {
		t.Fatal("stale worker corrupted new lease")
	}

}
