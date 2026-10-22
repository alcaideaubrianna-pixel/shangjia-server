package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	publishmodel "hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
)

// Real incoming metadata and its read-only DBX ledger export. This deliberately
// measures identity lookup only: misses still require fingerprint search.
func TestScanIncidentIdentityLocalReplay(t *testing.T) {
	path := os.Getenv("YOUBAN_SCAN_INCIDENT_FIXTURE")
	if path == "" {
		t.Skip("requires private incident metadata exported through DBX")
	}
	var fixture struct {
		TenantID  int64 `json:"tenantId"`
		AccountID int64 `json:"accountId"`
		Ledger    []struct {
			ID        int64  `json:"id"`
			TenantID  int64  `json:"tenant_id"`
			AccountID int64  `json:"account_id"`
			ProfileID int64  `json:"profile_id"`
			BotID     int64  `json:"bot_id"`
			ChatID    string `json:"target_chat_id"`
			MessageID int64  `json:"tg_message_id"`
			FileID    string `json:"tg_file_id"`
			UniqueID  string `json:"tg_file_unique_id"`
		} `json:"ledger"`
		Inputs []struct {
			MessageID int64                     `json:"messageId"`
			TraceID   string                    `json:"traceId"`
			GroupID   string                    `json:"groupId"`
			Item      *sysin.BotMediaSearchItem `json:"item"`
		} `json:"inputs"`
		InvalidProfiles []int64 `json:"invalidProfiles"`
		Accounts        []struct {
			ID       int64 `json:"id"`
			TenantID int64 `json:"tenantId"`
		} `json:"accounts"`
		Fingerprints map[string]string `json:"fingerprints"`
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Inputs) == 0 || len(fixture.Ledger) == 0 {
		t.Fatal("empty incident fixture")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	// Never allow this replay to select a production connection.
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	err = g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			"CREATE TEMP TABLE hg_youban_publish_tg_message(id bigint,tenant_id bigint,account_id bigint,profile_id bigint,bot_id bigint,target_chat_id text,tg_message_id bigint,tg_file_id text,tg_file_unique_id text,status text,deleted_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_content_profile(id bigint,deleted_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_youban_publish_profile_state(profile_id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp) ON COMMIT DROP",
			"CREATE INDEX ON hg_youban_publish_tg_message(tg_file_unique_id)",
			"CREATE INDEX ON hg_youban_publish_tg_message(target_chat_id,tg_message_id)",
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		invalid := map[int64]bool{}
		for _, id := range fixture.InvalidProfiles {
			invalid[id] = true
		}
		for _, row := range fixture.Ledger {
			if _, err := tx.Exec("INSERT INTO hg_youban_publish_tg_message VALUES(?,?,?,?,?,?,?,?,?,'sent',NULL)", row.ID, row.TenantID, row.AccountID, row.ProfileID, row.BotID, row.ChatID, row.MessageID, row.FileID, row.UniqueID); err != nil {
				return err
			}
			if _, err := tx.Exec("INSERT INTO hg_content_profile VALUES(?,NULL)", row.ProfileID); err != nil {
				return err
			}
			if !invalid[row.ProfileID] {
				if _, err := tx.Exec("INSERT INTO hg_youban_publish_profile_state VALUES(?,?,?,NULL)", row.ProfileID, row.TenantID, row.AccountID); err != nil {
					return err
				}
			}
		}
		groups := map[string][]int{}
		for i, input := range fixture.Inputs {
			key := input.TraceID
			if key == "" {
				key = input.GroupID
			}
			groups[key] = append(groups[key], i)
		}
		partitions := []publishmodel.MediaSearchScopePartition{}
		allowed := map[string]bool{}
		for _, a := range fixture.Accounts {
			partitions = append(partitions, publishmodel.MediaSearchScopePartition{TenantId: a.TenantID, AccountIds: []int64{a.ID}})
			allowed[fmt.Sprintf("%d:%d", a.TenantID, a.ID)] = true
		}
		if len(partitions) == 0 {
			t.Fatal("missing real account visibility scope")
		}
		scope := mediaSearchScopeFromPartitions(partitions)
		matched, missing := 0, 0
		var max time.Duration
		for key, indexes := range groups {
			items := make([]*sysin.BotMediaSearchItem, 0, len(indexes))
			for _, i := range indexes {
				items = append(items, fixture.Inputs[i].Item)
			}
			started := time.Now()
			got, err := botMediaIdentityCandidates(ctx, items, scope)
			if err != nil {
				return err
			}
			elapsed := time.Since(started)
			if elapsed > max {
				max = elapsed
			}
			for i, item := range items {
				want := map[int64]bool{}
				for _, row := range fixture.Ledger {
					if !allowed[fmt.Sprintf("%d:%d", row.TenantID, row.AccountID)] || invalid[row.ProfileID] {
						continue
					}
					exact := row.UniqueID == item.FileUniqueId && row.UniqueID != ""
					for _, id := range item.FileUniqueIds {
						exact = exact || (id != "" && id == row.UniqueID)
					}
					forward := item.SourceChatId != "" && item.SourceChatId == row.ChatID && item.SourceMessageId == row.MessageID
					if exact || forward {
						want[row.ProfileID] = true
					}
				}
				// An exact identity short-circuits the optional forward lookup.
				if len(got[i]) == 0 && len(want) > 0 {
					t.Fatalf("trace=%s message=%d missing known profile", key, fixture.Inputs[indexes[i]].MessageID)
				}
				for _, candidate := range got[i] {
					if !want[candidate.ProfileId] {
						t.Fatalf("trace=%s unexpected profile %d", key, candidate.ProfileId)
					}
				}
				if len(got[i]) > 0 {
					matched++
				} else {
					missing++
				}
			}
			if elapsed > 3*time.Second {
				t.Fatalf("identity lookup exceeded 3 seconds: %s", elapsed)
			}
			t.Logf("trace=%s items=%d identity_elapsed=%s", key, len(items), elapsed)
		}
		t.Logf("incident batches=%d items=%d known=%d fingerprint_required=%d max_identity=%s", len(groups), len(fixture.Inputs), matched, missing, max)
		if output := os.Getenv("YOUBAN_SCAN_INCIDENT_SQL_OUTPUT"); output != "" {
			queries := map[string]map[string]string{}
			for _, input := range fixture.Inputs {
				var fp cachedBotMediaFingerprint
				cached := fixture.Fingerprints[botMediaFingerprintCacheKey(input.Item.FileUniqueId)]
				if cached == "" {
					continue
				}
				if err := json.Unmarshal([]byte(cached), &fp); err != nil {
					return err
				}
				if fp.PHash == 0 {
					continue
				}
				hash := fmt.Sprintf("%016x", fp.PHash)
				if queries[input.TraceID] == nil {
					queries[input.TraceID] = map[string]string{}
				}
				for name, strategy := range map[string]bool{"lsh": false, "scope": true} {
					query, args, err := mediaPHashLshCandidateSQLWithStrategy(hash, botMediaSearchMaxDistance, scope.Partitions, nil, "", 0, strategy)
					if err != nil {
						return err
					}
					queries[input.TraceID][fmt.Sprintf("%d:%s", input.MessageID, name)] = gdb.FormatSqlWithArgs(query, args)
				}
			}
			data, err := json.Marshal(queries)
			if err != nil {
				return err
			}
			if err = os.WriteFile(output, data, 0600); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
