package sys

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcache"
	"github.com/gogf/gf/v2/os/gcfg"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/payment"
	paylogic "hotgo/internal/logic/pay"
	"hotgo/internal/model"
	baseentity "hotgo/internal/model/entity"
	"hotgo/internal/model/input/payin"
	"hotgo/internal/service"
)

type renewalConfigTestService struct{ service.ISysConfig }

func (*renewalConfigTestService) GetYoubanPublishVip(context.Context) (*model.YoubanPublishVipConfig, error) {
	return &model.YoubanPublishVipConfig{Enabled: true, MonthlyPrice: 30, PaymentGateway: consts.PayTypeRainbow, Currency: "USDT"}, nil
}
func (*renewalConfigTestService) GetYoubanPublishVipActivity(context.Context) (*model.YoubanPublishVipActivityConfig, error) {
	return &model.YoubanPublishVipActivityConfig{}, nil
}
func (*renewalConfigTestService) GetBasic(context.Context) (*model.BasicConfig, error) {
	return &model.BasicConfig{Domain: "https://example.com"}, nil
}

func TestTenantVipRenewalDurableLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	node := gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}
	root, err := gdb.New(node)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("renewal_test_%d", time.Now().UnixNano())
	if _, err := root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = root.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
	node.Namespace = schema
	gdb.SetConfigGroup("default", gdb.ConfigGroup{node})
	db := g.DB()
	if !cache.Initialized() {
		adapter := g.Cfg().GetAdapter().(*gcfg.AdapterFile)
		old := g.Cfg().MustGet(ctx, "cache.adapter").Val()
		if err := adapter.Set("cache.adapter", "memory"); err != nil {
			t.Fatal(err)
		}
		cache.SetAdapter(ctx)
		_ = adapter.Set("cache.adapter", old)
	}
	oldCache := cache.Instance().GetAdapter()
	cache.Instance().SetAdapter(gcache.NewAdapterMemory())
	t.Cleanup(func() { cache.Instance().SetAdapter(oldCache) })
	var oldConfig service.ISysConfig
	func() { defer func() { _ = recover() }(); oldConfig = service.SysConfig() }()
	service.RegisterSysConfig(&renewalConfigTestService{ISysConfig: oldConfig})
	t.Cleanup(func() { service.RegisterSysConfig(oldConfig) })
	var oldPay service.IPay
	func() { defer func() { _ = recover() }(); oldPay = service.Pay() }()
	service.RegisterPay(paylogic.NewPay())
	t.Cleanup(func() { service.RegisterPay(oldPay) })
	oldPayment := payment.GetConfig()
	payment.SetConfig(&model.PayConfig{})
	t.Cleanup(func() { payment.SetConfig(oldPayment) })
	for _, sql := range []string{
		`CREATE TABLE hg_youban_publish_tenant(id bigint primary key,name text,deleted_at timestamp)`,
		`CREATE TABLE hg_youban_publish_account(id bigint primary key,tenant_id bigint,account_type text,username text,status int,last_active_at timestamp,updated_at timestamp,deleted_at timestamp)`,
		`CREATE TABLE hg_admin_order(id bigserial primary key,member_id bigint,order_type text,product_id bigint,order_sn text UNIQUE,money numeric,remark text,status int,created_at timestamp,updated_at timestamp)`,
		`CREATE TABLE hg_pay_log(id bigserial primary key,member_id bigint,app_id text,addons_name text,order_sn text,order_group text,openid text,mch_id text,subject text,detail jsonb,out_trade_no text UNIQUE,transaction_id text,pay_type text,pay_amount numeric,actual_amount numeric,pay_status int,pay_at timestamp,trade_type text,is_refund int,create_ip text,notify_url text,return_url text,trace_ids jsonb,status int,created_at timestamp,updated_at timestamp)`,
		`INSERT INTO hg_youban_publish_tenant VALUES(1,'测试租户',NULL),(2,'其他租户',NULL)`,
		`INSERT INTO hg_youban_publish_account(id,tenant_id,account_type,username,status) VALUES(99,1,'admin','manager',1),(100,2,'admin','other',1)`,
	} {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.GetCore().ClearTableFieldsAll(context.Background()) })
	authCtx := context.WithValue(ctx, consts.ContextHTTPKey, &model.Context{Module: consts.AppApi, AddonName: "youban_bot", User: &model.Identity{Id: 99, App: consts.AppApi}})
	s := NewSysPublish()
	input := func() *sysin.TenantVipOrderCreateInp {
		return &sysin.TenantVipOrderCreateInp{RequestKey: "bot-renewal:7:1", PlanCode: tenantVipPlanMonth, ProductType: "vip", Quantity: 1}
	}
	// Invalid gateway configuration fails after local commit.
	if _, err := s.TenantVipOrderCreate(authCtx, input()); err == nil {
		t.Fatal("expected gateway failure")
	} else {
		t.Logf("injected gateway failure: %v", err)
	}
	order, err := db.GetOne(ctx, `SELECT * FROM hg_admin_order`)
	if err != nil {
		t.Fatal(err)
	}
	pay, err := db.GetOne(ctx, `SELECT * FROM hg_pay_log`)
	if err != nil {
		t.Fatal(err)
	}
	if order.IsEmpty() || pay.IsEmpty() || pay["out_trade_no"].String() == "" {
		t.Fatal("gateway failure rolled back payment identity")
	}
	payment.SetConfig(&model.PayConfig{RainbowPid: "test-merchant", RainbowKey: "test-key"})
	result, err := s.TenantVipOrderCreate(authCtx, input())
	if err != nil {
		t.Fatal(err)
	}
	if result.Id <= 0 || result.PayUrl == "" {
		t.Fatalf("invalid resumed payment: %+v", result)
	}
	parsed, err := url.Parse(result.PayUrl)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("out_trade_no") != pay["out_trade_no"].String() {
		t.Fatal("retry changed external payment identity")
	}
	gate := make(chan struct{})
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			again, err := s.TenantVipOrderCreate(authCtx, input())
			if err == nil && (again == nil || again.Id != result.Id) {
				err = fmt.Errorf("different order")
			}
			errs <- err
		}()
	}
	close(gate)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"hg_admin_order", "hg_pay_log"} {
		count, err := db.GetCount(ctx, "SELECT COUNT(*) FROM "+table)
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("duplicate %s: %d", table, count)
		}
	}
	changed := input()
	changed.CouponCode = "OTHER"
	if _, err := s.TenantVipOrderCreate(authCtx, changed); err == nil {
		t.Fatal("same confirmation changed purchase parameters")
	}
	if !strings.HasPrefix(result.OrderNo, "YBPB") {
		t.Fatal("missing scoped renewal identity")
	}
	// Concurrent first creation also serializes before either local row exists.
	freshGate := make(chan struct{})
	freshResults := make(chan *sysin.TenantVipOrderModel, 8)
	freshErrors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-freshGate
			in := input()
			in.RequestKey = "bot-renewal:7:fresh"
			result, err := s.TenantVipOrderCreate(authCtx, in)
			freshResults <- result
			freshErrors <- err
		}()
	}
	close(freshGate)
	wg.Wait()
	close(freshResults)
	close(freshErrors)
	for err := range freshErrors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var firstID int64
	for result := range freshResults {
		if result == nil || result.Id <= 0 {
			t.Fatal("missing concurrent first order")
		}
		if firstID == 0 {
			firstID = result.Id
		}
		if firstID != result.Id {
			t.Fatal("concurrent first creation returned different orders")
		}
	}
	for _, table := range []string{"hg_admin_order", "hg_pay_log"} {
		count, err := db.GetCount(ctx, "SELECT COUNT(*) FROM "+table)
		if err != nil {
			t.Fatal(err)
		}
		if count != 2 {
			t.Fatalf("first creation duplicated %s: %d", table, count)
		}
	}
	// Use the production membership ledger and coupon definitions for free-order
	// recovery. Membership application can fail after the paid local order commits.
	raw, err := os.ReadFile("../../resource/sql/install.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	tables := []string{"hg_youban_publish_tenant_vip", "hg_youban_publish_tenant_vip_log", "hg_youban_publish_tenant_vip_coupon", "hg_youban_publish_tenant_vip_event"}
	for _, table := range tables {
		start := strings.Index(content, `CREATE TABLE IF NOT EXISTS "`+table+`" (`)
		if start < 0 {
			t.Fatalf("missing production membership table %s", table)
		}
		end := strings.Index(content[start:], ";") + start
		if _, err := db.Exec(ctx, content[start:end]); err != nil {
			t.Fatal(err)
		}
	}
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, "CREATE UNIQUE INDEX") {
			continue
		}
		for _, table := range tables {
			if strings.Contains(line, ` ON "`+table+`" `) {
				if _, err := db.Exec(ctx, line); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	for _, sql := range []string{
		`INSERT INTO hg_youban_publish_tenant_vip_coupon(code,use_type,amount,total_count,status)VALUES('FREE','single',30,1,1)`,
		`ALTER TABLE hg_youban_publish_tenant_vip_log ADD CONSTRAINT reject_membership_log CHECK(false)`,
	} {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	freeInput := &sysin.TenantVipOrderCreateInp{RequestKey: "bot-renewal:7:2", PlanCode: tenantVipPlanMonth, CouponCode: "free", ProductType: "vip", Quantity: 1}
	if _, err := s.TenantVipOrderCreate(authCtx, freeInput); err == nil {
		t.Fatal("injected membership application failure hidden")
	}
	freeOrder, err := db.GetOne(ctx, `SELECT status FROM hg_admin_order WHERE order_sn=?`, tenantVipRenewalOrderNo(&sysin.AccountModel{Id: 99, TenantId: 1}, freeInput.RequestKey))
	if err != nil {
		t.Fatal(err)
	}
	if freeOrder.IsEmpty() || freeOrder["status"].Int() != consts.OrderStatusPay {
		t.Fatal("free paid order was not durable")
	}
	if _, err := db.Exec(ctx, `ALTER TABLE hg_youban_publish_tenant_vip_log DROP CONSTRAINT reject_membership_log`); err != nil {
		t.Fatal(err)
	}
	freeResult, err := s.TenantVipOrderCreate(authCtx, freeInput)
	if err != nil || freeResult == nil || freeResult.Status != consts.OrderStatusPay {
		t.Fatalf("free-order recovery failed: %v", err)
	}
	membership, err := db.GetOne(ctx, `SELECT expired_at FROM hg_youban_publish_tenant_vip WHERE tenant_id=1`)
	if err != nil {
		t.Fatal(err)
	}
	if membership["expired_at"].GTime().Timestamp() < time.Now().AddDate(0, 0, 29).Unix() {
		t.Fatal("free order did not extend membership")
	}
	expiredAt := membership["expired_at"].String()
	if _, err := s.TenantVipOrderCreate(authCtx, freeInput); err != nil {
		t.Fatal(err)
	}
	after, err := db.GetOne(ctx, `SELECT expired_at FROM hg_youban_publish_tenant_vip WHERE tenant_id=1`)
	if err != nil {
		t.Fatal(err)
	}
	if after["expired_at"].String() != expiredAt {
		t.Fatal("repeated confirmation extended membership twice")
	}
	coupon, err := db.GetOne(ctx, `SELECT used_count FROM hg_youban_publish_tenant_vip_coupon WHERE code='FREE'`)
	if err != nil {
		t.Fatal(err)
	}
	if coupon["used_count"].Int() != 1 {
		t.Fatal("coupon consumed more than once")
	}
	logs, err := db.GetCount(ctx, `SELECT COUNT(*) FROM hg_youban_publish_tenant_vip_log`)
	if err != nil {
		t.Fatal(err)
	}
	if logs != 1 {
		t.Fatalf("membership applied %d times", logs)
	}

	// Exercise the actual paid-order business callback inside its caller's
	// transaction, including rollback and duplicate callback delivery.
	var paidLog *baseentity.PayLog
	if err := db.Model("hg_pay_log").Ctx(ctx).Where("order_sn", result.OrderNo).Scan(&paidLog); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `ALTER TABLE hg_youban_publish_tenant_vip_log ADD CONSTRAINT reject_paid_callback CHECK(false) NOT VALID`); err != nil {
		t.Fatal(err)
	}
	callback := func(ctx context.Context, tx gdb.TX) error {
		return s.TenantVipPayNotify(ctx, &payin.NotifyCallFuncInp{Pay: paidLog})
	}
	if err := db.Transaction(authCtx, callback); err == nil {
		t.Fatal("callback failure hidden")
	}
	failedPaidOrder, err := db.GetOne(ctx, `SELECT status FROM hg_admin_order WHERE order_sn=?`, result.OrderNo)
	if err != nil {
		t.Fatal(err)
	}
	if failedPaidOrder["status"].Int() != consts.OrderStatusNotPay {
		t.Fatal("failed membership callback committed paid status")
	}
	if _, err := db.Exec(ctx, `ALTER TABLE hg_youban_publish_tenant_vip_log DROP CONSTRAINT reject_paid_callback`); err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(authCtx, callback); err != nil {
		t.Fatal(err)
	}
	paidMembership, err := db.GetOne(ctx, `SELECT expired_at FROM hg_youban_publish_tenant_vip WHERE tenant_id=1`)
	if err != nil {
		t.Fatal(err)
	}
	if paidMembership["expired_at"].GTime().Timestamp() < time.Now().AddDate(0, 0, 59).Unix() {
		t.Fatal("paid callback did not extend membership")
	}
	if err := db.Transaction(authCtx, callback); err != nil {
		t.Fatal(err)
	}
	afterPaidRepeat, err := db.GetOne(ctx, `SELECT expired_at FROM hg_youban_publish_tenant_vip WHERE tenant_id=1`)
	if err != nil {
		t.Fatal(err)
	}
	if afterPaidRepeat["expired_at"].String() != paidMembership["expired_at"].String() {
		t.Fatal("duplicate paid callback extended membership twice")
	}
	resumedPaid, err := s.TenantVipOrderCreate(authCtx, input())
	if err != nil {
		t.Fatal(err)
	}
	if resumedPaid.Status != consts.OrderStatusPay || resumedPaid.PayUrl != "" {
		t.Fatal("paid confirmation attempted another payment")
	}

}
