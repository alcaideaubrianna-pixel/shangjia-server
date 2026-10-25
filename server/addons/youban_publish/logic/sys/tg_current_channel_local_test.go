package sys

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestCurrentJobChannelLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_ACCOUNT_PHASE_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL")
	}
	if os.Getenv("YOUBAN_CURRENT_CHANNEL_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCurrentJobChannelLocalPostgres$", "-test.count=1")
		cmd.Env = append(os.Environ(), "YOUBAN_CURRENT_CHANNEL_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	rollback := errors.New("rollback fixtures")
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_youban_publish_tg_channel(id bigint,tenant_id bigint,tg_account_id bigint,channel_id text,access_hash text,channel_title text,is_broadcast int,is_megagroup int) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_tg_channel VALUES(1,31,82,'4326436231','123','current',1,0),(2,31,484,'4326436231','999','wrong publishing account',1,0)`,
			`CREATE TEMP TABLE hg_youban_publish_channel(id bigint,tenant_id bigint,tg_account_id bigint,channel_title text,target_chat_id text,bot_id_json text,status int,deleted_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_channel VALUES (89,31,32,'old','4326436231','[1]',1,now()),(1598,31,82,'new','4326436231','[2,3]',1,NULL),(2000,99,999,'other tenant','4326436231','[9]',1,NULL),(2001,31,83,'group','-5500489183','[4]',1,NULL)`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		s := new(sSysPublish)
		job := telegramJobRecord{TenantId: 31, AccountId: 484, ChannelId: 89, TargetChatId: "-1004326436231", BotId: 2}
		channel, err := s.currentTelegramJobChannel(ctx, job)
		if err != nil || channel.Id != 1598 || channel.TgAccountId != 82 {
			t.Fatalf("current channel=%+v err=%v", channel, err)
		}
		peer, peerErr := s.messagePushChannelFromJob(ctx, job)
		if peerErr != nil || peer.TgAccountId != 82 || peer.AccessHash != "123" {
			t.Fatalf("protocol lookup used publishing account: %+v %v", peer, peerErr)
		}
		bot, err := s.nextTelegramChannelBotId(ctx, job)
		if err != nil || bot != 3 {
			t.Fatalf("current fallback bot=%d err=%v", bot, err)
		}
		if _, err := tx.Exec(`UPDATE hg_youban_publish_channel SET tg_account_id=84 WHERE id=1598`); err != nil {
			return err
		}
		channel, err = s.currentTelegramJobChannel(ctx, job)
		if err != nil || channel.TgAccountId != 84 {
			t.Fatalf("configuration was stale: %+v %v", channel, err)
		}
		job.TargetChatId = "-5500489183"
		channel, err = s.currentTelegramJobChannel(ctx, job)
		if err != nil || channel.Id != 2001 {
			t.Fatalf("basic group changed destination: %+v %v", channel, err)
		}
		job.TargetChatId = "-10099999999"
		if _, err = s.currentTelegramJobChannel(ctx, job); err == nil {
			t.Fatal("stale channel ID redirected to a different chat")
		}
		job.TargetChatId = "-1004326436231"
		job.TenantId = 0
		if _, err = s.currentTelegramJobChannel(ctx, job); err == nil {
			t.Fatal("missing tenant accepted")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}
