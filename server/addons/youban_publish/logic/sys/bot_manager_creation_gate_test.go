package sys

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_publish/model/input/sysin"
)

// Separate transactions/connections exercise the production row locks, not a
// mock mutex. All tables live in a disposable local PostgreSQL schema.
func TestBotManagerCreationGatePostgres(t *testing.T) {
	link := os.Getenv("YOUBAN_RELAY_TEST_PG")
	if link == "" {
		t.Skip("requires local PostgreSQL")
	}
	if !strings.Contains(link, "127.0.0.1") || !strings.HasSuffix(link, "/youban_relay_test") {
		t.Fatal("refusing non-test database")
	}
	previous := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", previous) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Link: link}})
	db := g.DB()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	schema := fmt.Sprintf("bot_manager_gate_test_%d", time.Now().UnixNano())
	if _, err := db.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
	transaction := func(fn func(context.Context, gdb.TX) error) error {
		return db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			if _, err := tx.Exec("SET LOCAL search_path TO " + schema); err != nil {
				return err
			}
			return fn(ctx, tx)
		})
	}
	err := transaction(func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			"CREATE TABLE hg_youban_publish_account(id bigint PRIMARY KEY,tenant_id bigint,deleted_at timestamp)",
			"CREATE TABLE hg_youban_publish_tg_account(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,deleted_at timestamp)",
			"CREATE TABLE hg_youban_publish_tg_channel(id bigint,tenant_id bigint,tg_account_id bigint,channel_id text,access_hash text)",
			"CREATE TABLE hg_youban_publish_channel(id bigint PRIMARY KEY,tenant_id bigint,tg_account_id bigint,target_chat_id text,channel_title text,channel_username text,publish_direction text,status int,bot_id_json text,updated_at timestamp,deleted_at timestamp)",
			"CREATE TABLE hg_youban_publish_bot(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,bot_token text,bot_name text,bot_username text,status int,updated_at timestamp,deleted_at timestamp)",
			"CREATE TABLE hg_youban_publish_channel_permission_circuit(tenant_id bigint,channel_id bigint,paused int DEFAULT 0,PRIMARY KEY(tenant_id,channel_id))",
			"CREATE TABLE hg_youban_publish_tg_job(id bigserial PRIMARY KEY,tenant_id bigint,merchant_id bigint,account_id bigint,profile_id bigint,channel_id bigint,bot_id bigint,target_chat_id text,status text,operation_no text,collect_event_id bigint,collect_source_id bigint,collect_source_chat_id text,collect_source_message_id bigint,priority int,queue_name text,dispatch_status text,created_at timestamp,updated_at timestamp)",
			"INSERT INTO hg_youban_publish_account VALUES(99,1,NULL)",
			"INSERT INTO hg_youban_publish_tg_account VALUES(7,1,99,NULL)",
			"INSERT INTO hg_youban_publish_bot(id,tenant_id,account_id,bot_token,status) VALUES(1,1,99,'1001:token',1),(2,1,99,'1002:token',1)",
			"INSERT INTO hg_youban_publish_channel(id,tenant_id,tg_account_id,target_chat_id,channel_title,publish_direction,status,bot_id_json) VALUES(1,1,7,'-100123','One','up',1,'[1,2]'),(2,1,7,'-100124','Two','up',1,'[1,2]')",
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		return db.GetCore().ClearTableFieldsAll(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &sSysPublish{}
	source := gdb.Record{"tenant_id": gvar.New(1), "account_id": gvar.New(99), "profile_id": gvar.New(10)}
	for _, removalFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("removalFirst=%t", removalFirst), func(t *testing.T) {
			channelId := int64(1)
			if !removalFirst {
				channelId = 2
			}
			projection := &sysin.BotManagerProjection{TenantId: 1, AccountId: 99, ChannelId: channelId, BotId: 1, Remove: true}
			stale := telegramJobChannel{Id: channelId, TargetChatId: "-100999", BotIdJson: "[1,2]"}
			create := func(ctx context.Context, tx gdb.TX) error {
				_, err := s.ensureTelegramProfileJobWithMetaTx(ctx, tx, source, stale, fmt.Sprintf("manual:%d", channelId), telegramProfilePublishMeta{})
				return err
			}
			remove := func(ctx context.Context, _ gdb.TX) error {
				_, err := s.BotManagerApplyBinding(ctx, projection)
				return err
			}
			first, second := remove, create
			if !removalFirst {
				first, second = create, remove
			}
			held, release := make(chan struct{}), make(chan struct{})
			firstResult, secondResult := make(chan error, 1), make(chan error, 1)
			go func() {
				firstResult <- transaction(func(ctx context.Context, tx gdb.TX) error {
					if err := first(ctx, tx); err != nil {
						close(held)
						return err
					}
					close(held)
					select {
					case <-release:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				})
			}()
			select {
			case <-held:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			secondStarted := make(chan struct{})
			go func() {
				secondResult <- transaction(func(ctx context.Context, tx gdb.TX) error {
					close(secondStarted)
					return second(ctx, tx)
				})
			}()
			select {
			case <-secondStarted:
			case <-ctx.Done():
				close(release)
				t.Fatal(ctx.Err())
			}
			select {
			case err := <-secondResult:
				close(release)
				<-firstResult
				t.Fatalf("second operation passed the uncommitted creation gate: %v", err)
			case <-time.After(150 * time.Millisecond):
			}
			close(release)
			if err := <-firstResult; err != nil {
				t.Fatal(err)
			}
			err := <-secondResult
			if removalFirst && err != nil || !removalFirst && err == nil {
				t.Fatalf("unsafe operation result: %v", err)
			}
			if err := transaction(func(ctx context.Context, tx gdb.TX) error {
				job, err := tx.Model(publishTgJobTable).Where("channel_id", channelId).One()
				if err != nil {
					return err
				}
				wantBot, wantBinding, wantChat := int64(2), "[2]", "-100123"
				if !removalFirst {
					wantBot, wantBinding, wantChat = 1, "[1,2]", "-100124"
				}
				if job["bot_id"].Int64() != wantBot || job["target_chat_id"].String() != wantChat {
					return fmt.Errorf("stale transport persisted: %v", job)
				}
				binding, err := tx.Model(publishChannelTable).Where("id", channelId).Value("bot_id_json")
				if err == nil && binding.String() != wantBinding {
					return fmt.Errorf("unsafe binding: %s", binding.String())
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
	// Removing the last sender must fail new creation cleanly, even if the
	// caller captured a usable Bot before removal. Never persist that old ID.
	if err := transaction(func(ctx context.Context, tx gdb.TX) error {
		_, err := s.BotManagerApplyBinding(ctx, &sysin.BotManagerProjection{TenantId: 1, AccountId: 99, ChannelId: 1, BotId: 2, Remove: true})
		return err
	}); err == nil {
		// Channel 1 has a pending Bot 2 job from the first ordering test.
		t.Fatal("last sender removed despite a newly created pending job")
	}
	if err := transaction(func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec("UPDATE hg_youban_publish_tg_job SET status='superseded' WHERE channel_id=1"); err != nil {
			return err
		}
		_, err := s.BotManagerApplyBinding(ctx, &sysin.BotManagerProjection{TenantId: 1, AccountId: 99, ChannelId: 1, BotId: 2, Remove: true})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := transaction(func(ctx context.Context, tx gdb.TX) error {
		_, err := s.ensureTelegramProfileJobWithMetaTx(ctx, tx, source, telegramJobChannel{Id: 1, BotIdJson: "[2]"}, "manual:after-last-removal", telegramProfilePublishMeta{})
		return err
	}); err == nil {
		t.Fatal("stale snapshot created a job after the last sender was removed")
	}
	if err := transaction(func(ctx context.Context, tx gdb.TX) error {
		n, err := tx.Model(publishTgJobTable).Where("operation_no", "manual:after-last-removal").Count()
		if err == nil && n != 0 {
			return fmt.Errorf("orphan job persisted after last removal: %d", n)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
