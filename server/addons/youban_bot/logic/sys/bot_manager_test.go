package sys

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
)

func managerTestDB(t *testing.T) (context.Context, *sSysBot, *entity.YoubanBotManagerState) {
	t.Helper()
	ctx := relayTestDatabase(t)
	db, _ := gdb.Instance()
	ddl, err := os.ReadFile("../../resource/sql/manager.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"managed", "managed_channel", "managed_member", "manager_state", "manager_task"} {
		if _, err := db.Exec(ctx, "DROP TABLE IF EXISTS hg_youban_bot_"+table); err != nil {
			t.Fatal(err)
		}
	}
	sql := string(ddl)
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		sql = strings.ReplaceAll(sql, "bigserial PRIMARY KEY", "integer PRIMARY KEY AUTOINCREMENT")
	}
	// Both fresh installation and a repeated plugin upgrade must work.
	for attempt := 0; attempt < 2; attempt++ {
		for _, statement := range strings.Split(sql, ";") {
			if strings.TrimSpace(statement) != "" {
				if _, err := db.Exec(ctx, statement); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if err := db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	s := NewSysBot()
	s.relayAccountForUser = func(_ context.Context, id int64) (*botProfileAccount, error) {
		if id != 12 && id != 23 {
			return nil, fmt.Errorf("unbound")
		}
		return &botProfileAccount{TenantId: 1, AccountId: 99}, nil
	}
	st, err := s.managerState(ctx, 1, 12)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, s, st
}

func managerTestDraft(t *testing.T, ctx context.Context, st *entity.YoubanBotManagerState, id int64) {
	t.Helper()
	_, err := dao.YoubanBotManaged.Ctx(ctx).Data(do.YoubanBotManaged{Id: id, TenantId: st.TenantId, AccountId: st.AccountId, TelegramBotId: 1000 + id, BotToken: fmt.Sprintf("%d:test", id), Status: 1}).Insert()
	if err != nil {
		t.Fatal(err)
	}
	_, err = dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Data(do.YoubanBotManagerState{DraftBotId: id}).Update()
	if err != nil {
		t.Fatal(err)
	}
}
func managerTestChannel(t *testing.T, ctx context.Context, id int64, count, limit, bots int) {
	t.Helper()
	_, err := dao.YoubanBotManagedChannel.Ctx(ctx).Data(do.YoubanBotManagedChannel{Id: id, TenantId: 1, AccountId: 99, PublishChannelId: id, TgAccountId: 7, ChatId: fmt.Sprint(id), AdminCount: count, AdminLimit: limit, BotCount: bots, SyncStatus: "success", CanManage: 1, LastSyncedAt: gtime.Now()}).Insert()
	if err != nil {
		t.Fatal(err)
	}
}

func TestBotManagerSelectionPagination(t *testing.T) {
	channels := []*entity.YoubanBotManagedChannel{}
	for i := int64(1); i <= 25; i++ {
		channels = append(channels, &entity.YoubanBotManagedChannel{Id: i, SyncStatus: "success"})
	}
	st := &entity.YoubanBotManagerState{}
	if len(managerSelected(st, channels)) != 25 {
		t.Fatal("first selection must select all")
	}
	st.SelectionSaved = 1
	st.SelectedChannelIds = "[2,12,24,999]"
	if managerJSON(managerSelected(st, channels)) != "[2,12,24]" {
		t.Fatal("selection must survive pages and remove stale channels")
	}
	channels[11].SyncStatus = "unavailable"
	if managerJSON(managerSelected(st, channels)) != "[2,24]" {
		t.Fatal("unavailable channel selected")
	}
	for _, tc := range []struct{ page, total, want, start, end int }{{1, 25, 1, 0, 10}, {2, 25, 2, 10, 20}, {99, 25, 3, 20, 25}, {0, 0, 1, 0, 0}} {
		p, a, b := managerPage(tc.page, tc.total)
		if p != tc.want || a != tc.start || b != tc.end {
			t.Fatalf("page: %d %d %d", p, a, b)
		}
	}
	for _, raw := range []string{"bm:confirm:1:0:1", "bm:x:-1:1:1", "bm:x:1:1:-1", "other:x:1:1:1", "bm:x:1:100001:0"} {
		if _, err := managerParseCommand(raw); err == nil {
			t.Fatal("accepted malformed callback", raw)
		}
	}
	button := managerPageButton("test", "toggle", 123, 2, 34)
	command, err := managerParseCommand(button.CallbackData)
	if err != nil || command.ID != 123 || command.Page != 2 || command.Revision != 34 {
		t.Fatal(command, err)
	}
}

func TestBotManagerFreeBoundAndOwnership(t *testing.T) {
	ctx, s, st := managerTestDB(t)
	if st.AccountId != 99 {
		t.Fatal("free bound account rejected")
	}
	if _, err := s.managerState(ctx, 1, 999); err == nil {
		t.Fatal("unbound user accepted")
	}
	managerTestDraft(t, ctx, st, 1)
	other := *st
	other.AccountId = 100
	if _, err := managerBot(ctx, &other, 1); err == nil {
		t.Fatal("cross-account access accepted")
	}
	s.relayAccountForUser = func(context.Context, int64) (*botProfileAccount, error) {
		return &botProfileAccount{TenantId: 2, AccountId: 100}, nil
	}
	migrated, err := s.managerState(ctx, 1, 12)
	if err != nil || migrated.AccountId != 100 || migrated.DraftBotId != 0 || migrated.SelectionSaved != 0 || migrated.SyncedAt != nil {
		t.Fatal("old binding state leaked", migrated, err)
	}
}

func TestBotManagerAllocationCapacityAndIdempotency(t *testing.T) {
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		t.Skip("allocation row locks require local PostgreSQL")
	}
	ctx, s, st := managerTestDB(t)
	managerTestDraft(t, ctx, st, 1)
	managerTestChannel(t, ctx, 1, 9, 10, 0)
	managerTestChannel(t, ctx, 2, 3, 10, 1)
	if _, err := managerNewTask(ctx, st, "reservation", "attach", 22, 1, "{}"); err != nil {
		t.Fatal(err)
	}
	id, err := s.managerAllocate(ctx, st, st.Revision)
	if err != nil {
		t.Fatal(err)
	}
	var task entity.YoubanBotManagerTask
	if err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(id).Scan(&task); err != nil {
		t.Fatal(err)
	}
	if task.ChannelId != 2 {
		t.Fatal("reserved full channel selected", task.ChannelId)
	}
	if _, err := s.managerAllocate(ctx, st, st.Revision); err == nil {
		t.Fatal("duplicate confirm accepted")
	}
	// A restarted service sees durable state and the exact same task.
	restarted := NewSysBot()
	restarted.relayAccountForUser = s.relayAccountForUser
	restored, err := restarted.managerState(ctx, 1, 12)
	if err != nil || restored.DraftBotId != 0 || restored.SelectionSaved != 1 {
		t.Fatal(restored, err)
	}
	managerTestDraft(t, ctx, restored, 2)
	_, err = dao.YoubanBotManagedChannel.Ctx(ctx).WherePri(2).Data(do.YoubanBotManagedChannel{LastSyncedAt: gtime.New(time.Now().Add(-time.Hour))}).Update()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.managerAllocate(ctx, restored, restored.Revision); err == nil {
		var channels []*entity.YoubanBotManagedChannel
		_ = dao.YoubanBotManagedChannel.Ctx(ctx).Scan(&channels)
		for _, ch := range channels {
			t.Logf("channel=%d last=%v now=%v age=%v", ch.Id, ch.LastSyncedAt, time.Now(), time.Since(ch.LastSyncedAt.Time))
		}
		t.Fatal("full/stale candidates accepted")
	}
}

func TestBotManagerExistingAdminNeedsNoNewSlot(t *testing.T) {
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		t.Skip("allocation row locks require local PostgreSQL")
	}
	ctx, s, st := managerTestDB(t)
	managerTestDraft(t, ctx, st, 1)
	managerTestChannel(t, ctx, 1, 10, 10, 5)
	_, err := dao.YoubanBotManagedMember.Ctx(ctx).Data(do.YoubanBotManagedMember{TenantId: 1, AccountId: 99, ManagedBotId: 1, ChannelId: 1, MemberStatus: "admin"}).Insert()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.managerAllocate(ctx, st, st.Revision); err != nil {
		t.Fatal("existing administrator consumes no new slot", err)
	}
}

func TestBotManagerConcurrentReservationPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		t.Skip("row-lock concurrency requires disposable local PostgreSQL")
	}
	ctx, s, st := managerTestDB(t)
	managerTestDraft(t, ctx, st, 1)
	managerTestChannel(t, ctx, 1, 9, 10, 0)
	managerTestChannel(t, ctx, 2, 9, 10, 0)
	second, err := s.managerState(ctx, 1, 23)
	if err != nil {
		t.Fatal(err)
	}
	managerTestDraft(t, ctx, second, 2)
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for _, state := range []*entity.YoubanBotManagerState{st, second} {
		wg.Add(1)
		go func(state *entity.YoubanBotManagerState) {
			defer wg.Done()
			_, err := s.managerAllocate(ctx, state, state.Revision)
			errors <- err
		}(state)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var tasks []*entity.YoubanBotManagerTask
	if err := dao.YoubanBotManagerTask.Ctx(ctx).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 || tasks[0].ChannelId == tasks[1].ChannelId {
		t.Fatal("parallel requests overbooked a slot", tasks)
	}
}
