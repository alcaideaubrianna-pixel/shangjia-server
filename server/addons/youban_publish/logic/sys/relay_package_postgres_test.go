package sys

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/internal/model/entity"
)

// Uses only the disposable database created from the checked-in migrations.
func TestRelayPackageCapacityLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SQL_REVIEW_LOCAL_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:55484")
	}
	if sqlReviewSubprocess(t) {
		return
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55484", User: "test", Name: "youban_relay_package_test"}})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, table := range []string{dao.YoubanPublishRelayPackage.Table(), dao.YoubanPublishRelayChannel.Table()} {
		if _, err := g.DB().Exec(ctx, "TRUNCATE TABLE "+table+" RESTART IDENTITY"); err != nil {
			t.Fatal(err)
		}
	}
	channel := func(tenant int64, capacity int, status string) int64 {
		id, err := dao.YoubanPublishRelayChannel.Ctx(ctx).Data(do.YoubanPublishRelayChannel{TenantId: tenant, TgAccountId: tenant, TargetChatId: time.Now().String(), Status: relayChannelStatusEnabled, HealthStatus: status, Capacity: capacity}).InsertAndGetId()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	a, b := channel(10, 1, relayChannelHealthReady), channel(10, 2, relayChannelHealthReady)
	channel(11, 20, relayChannelHealthReady)
	cool := channel(10, 20, relayChannelHealthCooldown)
	if _, err := dao.YoubanPublishRelayChannel.Ctx(ctx).WherePri(cool).Data(do.YoubanPublishRelayChannel{CooldownUntil: gtime.Now().Add(time.Hour)}).Update(); err != nil {
		t.Fatal(err)
	}
	packages := make([]*entity.YoubanPublishRelayPackage, 12)
	for i := range packages {
		id, err := dao.YoubanPublishRelayPackage.Ctx(ctx).Data(do.YoubanPublishRelayPackage{TenantId: 10, JobId: int64(i + 1), BotId: 1, TelegramUserId: 1, Generation: 1, Status: "pending"}).InsertAndGetId()
		if err != nil {
			t.Fatal(err)
		}
		packages[i] = &entity.YoubanPublishRelayPackage{Id: id, TenantId: 10, Generation: 1, Status: "pending"}
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(packages))
	for _, pkg := range packages {
		wg.Add(1)
		go func(p *entity.YoubanPublishRelayPackage) {
			defer wg.Done()
			_, err := new(sSysPublish).reserveRelayPackageChannel(ctx, p)
			errs <- err
		}(pkg)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := g.DB().GetAll(ctx, "SELECT relay_channel_id,COUNT(*) AS n FROM hg_youban_publish_relay_package WHERE status='preparing' GROUP BY relay_channel_id")
	if err != nil {
		t.Fatal(err)
	}
	counts := map[int64]int{}
	for _, row := range rows {
		counts[row["relay_channel_id"].Int64()] = row["n"].Int()
	}
	if len(counts) != 2 || counts[a] != 1 || counts[b] != 2 {
		t.Fatalf("overbooked, crossed tenant, or used cooldown: %v", counts)
	}
	// A stale generation cannot reserve a newly pending package.
	var pending entity.YoubanPublishRelayPackage
	if err = dao.YoubanPublishRelayPackage.Ctx(ctx).Where("status", "pending").Limit(1).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if _, err = dao.YoubanPublishRelayChannel.Ctx(ctx).WherePri(a).Data(do.YoubanPublishRelayChannel{Capacity: 2}).Update(); err != nil {
		t.Fatal(err)
	}
	stale := pending
	stale.Generation--
	selected, err := new(sSysPublish).reserveRelayPackageChannel(ctx, &stale)
	if err != nil || selected.Id != 0 {
		t.Fatalf("stale generation reserved: %+v %v", selected, err)
	}
	selected, err = new(sSysPublish).reserveRelayPackageChannel(ctx, &pending)
	if err != nil || selected.Id != a {
		t.Fatalf("valid generation failed: %+v %v", selected, err)
	}
	if _, err = dao.YoubanPublishRelayPackage.Ctx(ctx).WherePri(pending.Id).Data(do.YoubanPublishRelayPackage{AttemptId: 123}).Update(); err != nil {
		t.Fatal(err)
	}
	markers, err := new(sSysPublish).relayPackageAttemptMarkers(ctx, []telegramDeliveryAttempt{{Id: 123, JobId: pending.JobId, Phase: "package"}, {Id: 124, JobId: pending.JobId, Phase: "package"}})
	if err != nil || len(markers) != 1 || markers[123] != telegramAttemptMarker(relayPackageToken(&pending)) {
		t.Fatalf("package marker lookup: %v %v", markers, err)
	}
}

func TestRelayPackagePartialEvidenceLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SQL_REVIEW_LOCAL_PG") != "1" {
		t.Skip("requires disposable PostgreSQL")
	}
	if sqlReviewSubprocess(t) {
		return
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55484", User: "test", Name: "youban_relay_package_test"}})
	ctx := context.Background()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec("CREATE TEMP TABLE hg_youban_publish_tg_attempt_message(attempt_id bigint,job_id bigint,target_chat_id text,message_id bigint,message_payload text,source text,created_at timestamp,UNIQUE(attempt_id,message_id)) ON COMMIT DROP"); err != nil {
			return err
		}
		attempt := telegramDeliveryAttempt{Id: 1, JobId: 1, TargetChatId: "-1001"}
		mapped := &telegramSentMessage{MessageId: 100, RelaySourceId: 10, Purpose: "verify", MediaId: 7}
		if err := recordTelegramAttemptMessagesTx(ctx, tx, attempt, []*telegramSentMessage{mapped}, "webhook"); err != nil {
			return err
		}
		raw := &telegramSentMessage{MessageId: 100, Purpose: "package"}
		if err := recordTelegramAttemptMessagesTx(ctx, tx, attempt, []*telegramSentMessage{raw}, "partial_response"); err != nil {
			return err
		}
		row, err := tx.GetOne("SELECT source,message_payload FROM hg_youban_publish_tg_attempt_message WHERE attempt_id=1 AND message_id=100")
		if err != nil {
			return err
		}
		if row["source"].String() != "webhook" || !strings.Contains(row["message_payload"].String(), `"Purpose":"verify"`) {
			t.Fatalf("partial response overwrote authoritative webhook: %v", row)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
