package sys

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	publishmodel "hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestBotMediaIdentityProductionSampleLocal(t *testing.T) {
	path := os.Getenv("YOUBAN_SCAN_IDENTITY_FIXTURE")
	if path == "" {
		t.Skip("requires DBX exported production sample and local PostgreSQL")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var samples []struct {
		ID        int64  `json:"id"`
		TenantID  int64  `json:"tenant_id"`
		AccountID int64  `json:"account_id"`
		ProfileID int64  `json:"profile_id"`
		BotID     int64  `json:"bot_id"`
		ChatID    string `json:"target_chat_id"`
		MessageID int64  `json:"tg_message_id"`
		FileID    string `json:"tg_file_id"`
		UniqueID  string `json:"tg_file_unique_id"`
	}
	if err = json.Unmarshal(raw, &samples); err != nil {
		t.Fatal(err)
	}
	if len(samples) != 100 {
		t.Fatalf("want 100 real images, got %d", len(samples))
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	// Hard-coded loopback test connection: this test cannot write production.
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	err = g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			"CREATE TEMP TABLE hg_youban_publish_tg_message(id bigint,tenant_id bigint,account_id bigint,profile_id bigint,bot_id bigint,target_chat_id text,tg_message_id bigint,tg_file_id text,tg_file_unique_id text,status text,deleted_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_content_profile(id bigint,deleted_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_youban_publish_profile_state(profile_id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp) ON COMMIT DROP",
			"CREATE INDEX ON hg_youban_publish_tg_message(tg_file_unique_id,account_id) WHERE tg_file_unique_id<>'' AND deleted_at IS NULL",
			"CREATE INDEX ON hg_youban_publish_tg_message(target_chat_id,tg_message_id)",
			"CREATE INDEX ON hg_youban_publish_tg_message(bot_id,tg_file_id,account_id) WHERE tg_file_id<>'' AND deleted_at IS NULL AND status='sent'",
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		for _, s := range samples {
			if _, err := tx.Exec("INSERT INTO hg_youban_publish_tg_message VALUES(?,?,?,?,?,?,?,?,?,'sent',NULL)", s.ID, s.TenantID, s.AccountID, s.ProfileID, s.BotID, s.ChatID, s.MessageID, s.FileID, s.UniqueID); err != nil {
				return err
			}
			if _, err := tx.Exec("INSERT INTO hg_content_profile VALUES(?,NULL)", s.ProfileID); err != nil {
				return err
			}
			if _, err := tx.Exec("INSERT INTO hg_youban_publish_profile_state VALUES(?,?,?,NULL)", s.ProfileID, s.TenantID, s.AccountID); err != nil {
				return err
			}
		}
		for _, size := range []int{1, 2, 5, 10, 30, 100} {
			var times []time.Duration
			for start := 0; start < len(samples); start += size {
				end := start + size
				if end > len(samples) {
					end = len(samples)
				}
				items := make([]*sysin.BotMediaSearchItem, 0, end-start)
				scope := &publishmodel.MediaSearchScope{}
				for _, s := range samples[start:end] {
					items = append(items, &sysin.BotMediaSearchItem{FileUniqueId: "unknown-size", FileUniqueIds: []string{s.UniqueID, s.UniqueID}, MediaType: "image"})
					scope.Partitions = append(scope.Partitions, publishmodel.MediaSearchScopePartition{TenantId: s.TenantID, AccountIds: []int64{s.AccountID}})
				}
				scope = mediaSearchScopeFromPartitions(scope.Partitions)
				began := time.Now()
				groups, err := botMediaIdentityCandidates(ctx, items, scope)
				if err != nil {
					return err
				}
				times = append(times, time.Since(began))
				for i, s := range samples[start:end] {
					found := false
					for _, c := range groups[i] {
						if c.ProfileId == s.ProfileID {
							found = true
						}
					}
					if !found {
						t.Fatalf("size=%d sample=%d missing expected profile %d", size, start+i, s.ProfileID)
					}
				}
				if len(rankBotMediaSearchCandidates(groups)) == 0 {
					t.Fatal("empty ranking")
				}
				if times[len(times)-1] > 3*time.Second {
					t.Fatal("local identity search exceeded 3 seconds")
				}
			}
			sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
			t.Logf("images=%d batches=%d p50=%s p95=%s max=%s", size, len(times), times[len(times)/2], times[(len(times)-1)*95/100], times[len(times)-1])
		}
		s := samples[0]
		scope := &publishmodel.MediaSearchScope{Partitions: []publishmodel.MediaSearchScopePartition{{TenantId: s.TenantID, AccountIds: []int64{s.AccountID}}}}
		for name, item := range map[string]*sysin.BotMediaSearchItem{
			"forward":       {SourceChatId: s.ChatID, SourceMessageId: s.MessageID},
			"same-bot-file": {BotId: s.BotID, FileId: s.FileID},
			"unknown":       {FileUniqueId: "never-published"},
			"wrong-bot":     {BotId: s.BotID + 100000, FileId: s.FileID},
		} {
			groups, err := botMediaIdentityCandidates(ctx, []*sysin.BotMediaSearchItem{item}, scope)
			if err != nil {
				return err
			}
			want := name == "forward" || name == "same-bot-file"
			if (len(groups[0]) > 0) != want {
				t.Fatalf("%s identity mismatch", name)
			}
		}
		wrong := &publishmodel.MediaSearchScope{Partitions: []publishmodel.MediaSearchScopePartition{{TenantId: s.TenantID + 100000, AccountIds: []int64{s.AccountID}}}}
		groups, err := botMediaIdentityCandidates(ctx, []*sysin.BotMediaSearchItem{{FileUniqueId: s.UniqueID}}, wrong)
		if err != nil {
			return err
		}
		if len(groups[0]) != 0 {
			t.Fatal("cross tenant leak")
		}
		wrong = &publishmodel.MediaSearchScope{Partitions: []publishmodel.MediaSearchScopePartition{{TenantId: s.TenantID, AccountIds: []int64{s.AccountID + 100000}}}}
		groups, err = botMediaIdentityCandidates(ctx, []*sysin.BotMediaSearchItem{{FileUniqueId: s.UniqueID}}, wrong)
		if err != nil {
			return err
		}
		if len(groups[0]) != 0 {
			t.Fatal("cross account leak")
		}
		// Historical deleted/failed sends cannot become exact matches.
		for _, field := range []string{"deleted_at=now()", "status='failed'"} {
			if _, err := tx.Exec("SAVEPOINT identity_state"); err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE hg_youban_publish_tg_message SET "+field+" WHERE tg_file_unique_id=?", s.UniqueID); err != nil {
				return err
			}
			groups, err := botMediaIdentityCandidates(ctx, []*sysin.BotMediaSearchItem{{FileUniqueId: s.UniqueID}}, scope)
			if err != nil {
				return err
			}
			if len(groups[0]) != 0 {
				t.Fatalf("invalid send matched: %s", field)
			}
			if _, err := tx.Exec("ROLLBACK TO SAVEPOINT identity_state"); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
