package sys

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestBotManagerPublishBridgePostgres(t *testing.T) {
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
	ctx := context.Background()
	db := g.DB()
	err := db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		statements := []string{
			"CREATE TEMP TABLE hg_youban_publish_channel_permission_circuit(tenant_id bigint,channel_id bigint,paused int DEFAULT 0,PRIMARY KEY(tenant_id,channel_id)) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_youban_publish_account(id bigint PRIMARY KEY,tenant_id bigint,deleted_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_youban_publish_tg_account(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,deleted_at timestamp,status text DEFAULT 'authorized',session_key text DEFAULT 'test') ON COMMIT DROP",
			"CREATE TEMP TABLE hg_youban_publish_tg_channel(id bigint PRIMARY KEY,tenant_id bigint,tg_account_id bigint,channel_id text,access_hash text) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_youban_publish_channel(id bigint PRIMARY KEY,tenant_id bigint,tg_account_id bigint,target_chat_id text,channel_title text,channel_username text,publish_direction text,status int,bot_id_json text DEFAULT '[]',created_at timestamp,updated_at timestamp,deleted_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_youban_publish_bot(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,bot_name text,bot_username text,bot_token text,status int,created_at timestamp,updated_at timestamp,deleted_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_youban_publish_tg_job(id bigint PRIMARY KEY,tenant_id bigint,channel_id bigint,bot_id bigint,status text) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_youban_publish_collect_source(id bigint PRIMARY KEY,bot_id bigint,deleted_at timestamp) ON COMMIT DROP",
			"INSERT INTO hg_youban_publish_account VALUES(99,1,NULL),(100,1,NULL)",
			"INSERT INTO hg_youban_publish_tg_account(id,tenant_id,account_id,deleted_at) VALUES(7,1,99,NULL),(8,1,100,NULL)",
			"INSERT INTO hg_youban_publish_tg_channel VALUES(1,1,7,'123','567'),(2,1,8,'124','568')",
			"INSERT INTO hg_youban_publish_channel(id,tenant_id,tg_account_id,target_chat_id,channel_title,channel_username,publish_direction,status)VALUES(1,1,7,'-100123','Mine','mine','up',1),(2,1,8,'-100124','Other','other','up',1),(3,2,7,'-100125','Other tenant','other','up',1)",
		}
		for _, statement := range statements {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		if err := db.GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		s := &sSysPublish{}
		channels, _, err := s.BotManagerCatalog(ctx, 1, 99)
		if err != nil {
			return err
		}
		if len(channels) != 1 || channels[0].Id != 1 || channels[0].AccessHash != "567" {
			t.Fatalf("catalog leaked inaccessible channel or lost access hash: %+v", channels[0])
		}
		projection := &sysin.BotManagerProjection{TenantId: 1, AccountId: 99, ChannelId: 1, BotName: "test", BotUsername: "test_bot", BotToken: "12345:fake"}
		id, err := s.BotManagerApplyBinding(ctx, projection)
		if err != nil {
			return err
		}
		projection.BotId = id
		fresh := &sysin.BotManagerProjection{TenantId: 1, AccountId: 99, BotToken: "99999:first", BotUsername: "new_bot"}
		freshId, err := s.BotManagerUpdateCredentials(ctx, fresh)
		if err != nil || freshId <= 0 {
			t.Fatal("new credentials did not establish canonical publishing record", freshId, err)
		}
		if _, err := tx.Exec("UPDATE hg_youban_publish_bot SET status=2 WHERE id=?", freshId); err != nil {
			return err
		}
		fresh.BotToken = "99999:rotated"
		if again, err := s.BotManagerUpdateCredentials(ctx, fresh); err != nil || again != freshId {
			t.Fatal("new Bot rotation duplicated identity", again, err)
		}
		freshStatus, err := tx.Model(publishBotTable).Where("id", freshId).Value("status")
		if err != nil || freshStatus.Int() != 2 {
			t.Fatal("credential update unexpectedly enabled Bot", freshStatus, err)
		}
		credential := *projection
		credential.BotToken = "12345:rotated"
		if updated, err := s.BotManagerUpdateCredentials(ctx, &credential); err != nil || updated != id {
			return err
		}
		stored, err := tx.Model(publishBotTable).Where("id", id).Value("bot_token")
		if err != nil || stored.String() != credential.BotToken {
			t.Fatal("rotation did not update publishing credentials", stored, err)
		}
		rollback := errors.New("registry persistence failed")
		if err := db.Transaction(ctx, func(ctx context.Context, _ gdb.TX) error {
			failed := credential
			failed.BotToken = "12345:must-rollback"
			if _, err := s.BotManagerUpdateCredentials(ctx, &failed); err != nil {
				return err
			}
			return rollback
		}); !errors.Is(err, rollback) {
			t.Fatal(err)
		}
		stored, err = tx.Model(publishBotTable).Where("id", id).Value("bot_token")
		if err != nil || stored.String() != credential.BotToken {
			t.Fatal("registry rollback left credentials inconsistent", stored, err)
		}
		// A previously queued binding operation still carries the old Token.
		// It must not roll back a successful rotation.
		if again, err := s.BotManagerApplyBinding(ctx, projection); err != nil || again != id {
			t.Fatalf("binding created duplicate Bot: %d %v", again, err)
		}
		stored, err = tx.Model(publishBotTable).Where("id", id).Value("bot_token")
		if err != nil || stored.String() != credential.BotToken {
			t.Fatal("queued binding reverted rotated Token", stored, err)
		}
		value, err := tx.Model(publishChannelTable).Where("id", 1).Value("bot_id_json")
		if err != nil {
			return err
		}
		ids := decodeBotIds(value.String())
		if len(ids) != 1 || ids[0] != id {
			t.Fatal("binding duplicated", ids)
		}
		projection.ChannelId = 2
		if _, err := s.BotManagerApplyBinding(ctx, projection); err == nil {
			t.Fatal("cross-account binding accepted")
		}
		if err := s.BotManagerValidateToken(ctx, 1, 100, "12345:rotated"); err == nil {
			t.Fatal("other account claimed existing Bot")
		}
		if err := s.BotManagerDelete(ctx, 1, 99, id); err == nil {
			t.Fatal("deleted bound Bot")
		}
		projection.ChannelId = 1
		if _, err := tx.Exec("INSERT INTO hg_youban_publish_channel(id,tenant_id,tg_account_id,target_chat_id,channel_title,publish_direction,status,bot_id_json) VALUES(4,1,7,'-100126','Shared Bot','up',1,?)", value.String()); err != nil {
			return err
		}
		projection.Remove = true
		if _, err := s.BotManagerApplyBinding(ctx, projection); err != nil {
			return err
		}
		remaining, err := tx.Model(publishChannelTable).Where("id", 1).Value("bot_id_json")
		if err != nil || len(decodeBotIds(remaining.String())) != 0 {
			t.Fatal("revoked channel retained sending binding", remaining, err)
		}
		shared, err := tx.Model(publishChannelTable).Where("id", 4).Value("bot_id_json")
		if err != nil || shared.String() != value.String() {
			t.Fatal("channel revocation altered another channel", shared, err)
		}
		status, err := tx.Model(publishBotTable).Where("id", id).Value("status")
		if err != nil || status.Int() != 1 {
			t.Fatal("channel revocation disabled shared Bot globally", status, err)
		}
		if _, err := tx.Exec("UPDATE hg_youban_publish_channel SET bot_id_json='[]' WHERE id=4"); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO hg_youban_publish_tg_job VALUES(1,1,1,?,'pending')", id); err != nil {
			return err
		}
		if err := s.BotManagerDelete(ctx, 1, 99, id); err == nil {
			t.Fatal("deleted queued Bot")
		}
		if _, err := tx.Exec("UPDATE hg_youban_publish_tg_job SET status='sent' WHERE id=1"); err != nil {
			return err
		}
		if err := s.BotManagerDelete(ctx, 1, 99, id); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
