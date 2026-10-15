package sys

import (
	"os"
	"sync"
	"testing"
	"time"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
)

func TestRelayPostgresAlbumClaimIsExclusive(t *testing.T) {
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		t.Skip("requires local PostgreSQL row locks")
	}
	ctx := relayTestDatabase(t)
	for i := 1; i <= 5; i++ {
		_, err := dao.YoubanBotRelayDelivery.Ctx(ctx).Data(do.YoubanBotRelayDelivery{BotId: 1, SourceChatId: "12", SourceMessageId: i, MediaGroupId: "album", TargetChatId: "13", Kind: consts.RelayKindChat, RawJson: "{}", Status: consts.RelayPending, AvailableAt: gtime.Now()}).Insert()
		if err != nil {
			t.Fatal(err)
		}
	}
	var next *entity.YoubanBotRelayDelivery
	if err := dao.YoubanBotRelayDelivery.Ctx(ctx).OrderAsc(dao.YoubanBotRelayDelivery.Columns().Id).Scan(&next); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	counts := make(chan int, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rows, _, err := claimRelayDeliveries(ctx, next)
			counts <- len(rows)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(counts)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	total := 0
	winners := 0
	for n := range counts {
		total += n
		if n > 0 {
			winners++
		}
	}
	if total != 5 || winners != 1 {
		t.Fatalf("claimed=%d winners=%d, album can duplicate", total, winners)
	}
	// A crashed sender is recorded as uncertain rather than enqueued a second time.
	c := dao.YoubanBotRelayDelivery.Columns()
	if _, err := dao.YoubanBotRelayDelivery.Ctx(ctx).Where(c.BotId, 1).Data(do.YoubanBotRelayDelivery{AvailableAt: gtime.Now().Add(-time.Minute)}).Update(); err != nil {
		t.Fatal(err)
	}
	// Exercise the actual runtime recovery after simulating an expired send lease.
	if err := recoverRelayDeliveries(ctx); err != nil {
		t.Fatal(err)
	}
	count, err := dao.YoubanBotRelayDelivery.Ctx(ctx).Where(c.Status, consts.RelayUncertain).Count()
	if err != nil || count != 5 {
		t.Fatalf("uncertain=%d err=%v", count, err)
	}
}

func TestRelayOwnerIdentityAndPendingAdmin(t *testing.T) {
	ctx := relayTestDatabase(t)
	db, _ := gdb.Instance()
	_, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS hg_youban_bot_account_bind(id bigint PRIMARY KEY,app varchar(32),account_id bigint,telegram_user_id varchar(64),status integer,deleted_at timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `DELETE FROM hg_youban_bot_account_bind`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO hg_youban_bot_account_bind(id,app,account_id,telegram_user_id,status) VALUES(1,'admin',99,'12',1)`); err != nil {
		t.Fatal(err)
	}
	s := NewSysBot()
	cfg := &entity.YoubanBotRelayConfig{BotId: 1, OwnerId: 99}
	ok, err := s.relayIsAdmin(ctx, cfg, 12)
	if err != nil || !ok {
		t.Fatalf("owner not admin: %t %v", ok, err)
	}
	ok, err = s.relayIsAdmin(ctx, cfg, 13)
	if err != nil || ok {
		t.Fatalf("unrelated user became admin: %t %v", ok, err)
	}
	if _, err = dao.YoubanBotRelayAdmin.Ctx(ctx).Data(do.YoubanBotRelayAdmin{BotId: 1, UserId: "", Username: "@alice"}).Insert(); err != nil {
		t.Fatal(err)
	}
	ok, err = s.relayIsAdmin(ctx, cfg, 13)
	if err != nil || ok {
		t.Fatal("pending username already granted access")
	}
	msg := &models.Message{From: &models.User{ID: 13, Username: "Alice"}, Chat: models.Chat{ID: 13, Type: models.ChatTypePrivate}}
	if err = s.relayResolvePendingAdmins(ctx, 1, msg); err != nil {
		t.Fatal(err)
	}
	ok, err = s.relayIsAdmin(ctx, cfg, 13)
	if err != nil || !ok {
		t.Fatal("observed pending admin not resolved")
	}
	msg.From.ID = 14
	if err = s.relayResolvePendingAdmins(ctx, 1, msg); err != nil {
		t.Fatal(err)
	}
	ok, err = s.relayIsAdmin(ctx, cfg, 14)
	if err != nil || ok {
		t.Fatal("reassigned username inherited admin rights")
	}
}
