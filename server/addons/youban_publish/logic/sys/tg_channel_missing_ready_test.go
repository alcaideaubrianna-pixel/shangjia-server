package sys

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestTelegramChannelMissingReadyLegacyIndexIsolation(t *testing.T) {
	reliableChannelTestRedis(t, false)
	ctx := context.Background()
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			t.Setenv("YOUBAN_PUBLISH_CHANNEL_QUEUE_ENABLED", fmt.Sprint(enabled))
			for _, priority := range []int{tgJobPriorityInteractive, 10, 95} {
				channelID, jobID := int64(5000+priority), int64(3349820+priority)
				due := time.Now().Add(time.Hour)
				if err := enqueueTelegramChannelJob(ctx, channelID, jobID, priority, due); err != nil {
					t.Fatal(err)
				}
				key := telegramChannelQueueKeyForPriority(channelID, priority)
				before, err := telegramChannelRedisDo(ctx, "ZSCORE", key, jobID)
				if err != nil {
					t.Fatal(err)
				}
				index := tgChannelPendingIndex{}
				if err := index.add(ctx, 56, channelID, jobID, priority); err != nil {
					t.Fatal(err)
				}
				afterAdd, err := telegramChannelRedisDo(ctx, "ZSCORE", key, jobID)
				if err != nil {
					t.Fatal(err)
				}
				if enabled && afterAdd.String() != before.String() {
					t.Fatal("legacy add changed ready due score")
				}
				if !enabled {
					head, err := index.head(ctx, 56, channelID)
					if err != nil || head != jobID {
						t.Fatalf("legacy add/head changed: head=%d want=%d err=%v", head, jobID, err)
					}
					legacyScore, err := telegramChannelRedisDo(ctx, "ZSCORE", tgChannelPendingKey(56, channelID), jobID)
					if err != nil || legacyScore.Int64() != int64(priority)*1_000_000_000_000+jobID {
						t.Fatalf("legacy priority encoding changed: %v %v", legacyScore, err)
					}
				}
				if err := index.remove(ctx, 56, channelID, jobID); err != nil {
					t.Fatal(err)
				}
				after, err := telegramChannelRedisDo(ctx, "ZSCORE", key, jobID)
				if err != nil {
					t.Fatal(err)
				}
				if enabled && after.String() != before.String() {
					t.Fatal("Outbox legacy cleanup removed ready job")
				}
				if !enabled {
					head, err := index.head(ctx, 56, channelID)
					if err != nil || head != 0 {
						t.Fatalf("legacy cleanup/head changed: head=%d err=%v", head, err)
					}
				}
			}
			if enabled {
				for lane, priority := range []int{tgJobPriorityInteractive, 10, 95} {
					// A real live claim must retain its token, score, deadline and
					// due index when legacy cleanup runs concurrently with sending.
					channelID, jobID := int64(6001+lane), int64(3349999+lane)
					if err := enqueueTelegramChannelJob(ctx, channelID, jobID, priority, time.Now().Add(-time.Second)); err != nil {
						t.Fatal(err)
					}
					claim, err := claimTelegramChannelJob(ctx, channelID)
					if err != nil || claim.JobID != jobID {
						t.Fatalf("claim failed: %+v %v", claim, err)
					}
					before, err := telegramChannelRedisDo(ctx, "HGETALL", telegramChannelProcessingKey(channelID))
					if err != nil {
						t.Fatal(err)
					}
					deadline, _ := telegramChannelRedisDo(ctx, "ZSCORE", telegramChannelProcessingDueKey, channelID)
					index := tgChannelPendingIndex{}
					if err := index.add(ctx, 56, channelID, jobID, priority); err != nil {
						t.Fatal(err)
					}
					if err := index.remove(ctx, 56, channelID, jobID); err != nil {
						t.Fatal(err)
					}
					if head, err := index.head(ctx, 56, channelID); err != nil || head != 0 {
						t.Fatalf("legacy head visible in channel mode: %d %v", head, err)
					}
					after, _ := telegramChannelRedisDo(ctx, "HGETALL", telegramChannelProcessingKey(channelID))
					afterDeadline, _ := telegramChannelRedisDo(ctx, "ZSCORE", telegramChannelProcessingDueKey, channelID)
					if fmt.Sprint(before.Array()) != fmt.Sprint(after.Array()) || deadline.String() != afterDeadline.String() {
						t.Fatal("legacy pending index changed active claim or due index")
					}
					for _, key := range []string{telegramChannelQueueKey(channelID), telegramChannelUrgentQueueKey(channelID), telegramChannelHistoryQueueKey(channelID)} {
						v, err := telegramChannelRedisDo(ctx, "ZSCORE", key, jobID)
						if err != nil || !v.IsNil() {
							t.Fatalf("legacy add recreated a ready member for live claim: %v %v", v, err)
						}
					}
					if ok, err := settleTelegramChannelClaim(ctx, claim, false, priority, time.Now()); err != nil || !ok {
						t.Fatalf("original claim lost ownership: %v %v", ok, err)
					}
				}
			}
		})
	}
}

// Explicit opt-in to a disposable loopback PostgreSQL; no application DSN or
// file configuration is read. Each table lives in a private schema.
func missingReadyLocalDB(t *testing.T) gdb.DB {
	t.Helper()
	if os.Getenv("YOUBAN_MISSING_READY_LOCAL_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:35432")
	}
	reliableChannelTestRedis(t, false)
	previousGroup, groupErr := gdb.GetConfigGroup("default")
	previousGroup = append(gdb.ConfigGroup(nil), previousGroup...)
	// GetAllConfig returns the live map. Copy it before SetConfigGroup so an
	// initially absent default group can also be restored exactly.
	previousConfig := make(gdb.Config)
	if groupErr != nil {
		for name, nodes := range gdb.GetAllConfig() {
			previousConfig[name] = append(gdb.ConfigGroup(nil), nodes...)
		}
	}
	node := gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}
	admin, err := gdb.New(node)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("missing_ready_%d", time.Now().UnixNano())
	if _, err = admin.Exec(context.Background(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Drop through the explicit admin connection first, then invalidate the
		// test default group. Restore even if schema cleanup reports an error.
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
		var restoreErr error
		if groupErr == nil {
			restoreErr = gdb.SetConfigGroup("default", previousGroup)
		} else {
			restoreErr = gdb.SetConfig(previousConfig)
		}
		if restoreErr != nil {
			t.Error(restoreErr)
		}
	})
	node.Namespace = schema
	if err := gdb.SetConfigGroup("default", gdb.ConfigGroup{node}); err != nil {
		t.Fatal(err)
	}
	return g.DB()
}

func TestTelegramChannelMissingReadyOutboxCompleteLocalPostgres(t *testing.T) {
	previousGroup, previousErr := gdb.GetConfigGroup("default")
	previousGroup = append(gdb.ConfigGroup(nil), previousGroup...)
	t.Run("isolated_database", func(t *testing.T) {
		db := missingReadyLocalDB(t)
		ctx := context.Background()
		for _, sql := range []string{
			"CREATE TABLE " + publishTgJobTable + " (id bigint PRIMARY KEY,tenant_id bigint,channel_id bigint,priority int)",
			"CREATE TABLE " + publishTgOutboxTable + " (id bigint PRIMARY KEY,job_id bigint,status text,lease_owner text,lease_expires_at timestamp,delivered_at timestamp,last_error text,updated_at timestamp)",
		} {
			if _, err := db.Exec(ctx, sql); err != nil {
				t.Fatal(err)
			}
		}
		for i, enabled := range []bool{true, false} {
			t.Run(fmt.Sprint(enabled), func(t *testing.T) {
				t.Setenv("YOUBAN_PUBLISH_CHANNEL_QUEUE_ENABLED", fmt.Sprint(enabled))
				for lane, priority := range []int{10, tgJobPriorityInteractive, 95} {
					channelID, jobID := int64(1674+i*10+lane), int64(3349820+i*10+lane)
					if _, err := db.Exec(ctx, "INSERT INTO "+publishTgJobTable+" VALUES(?,56,?,?)", jobID, channelID, priority); err != nil {
						t.Fatal(err)
					}
					if _, err := db.Exec(ctx, "INSERT INTO "+publishTgOutboxTable+" (id,job_id,status) VALUES(?,?,'processing')", jobID, jobID); err != nil {
						t.Fatal(err)
					}
					due := time.Now().Add(time.Minute)
					if err := enqueueTelegramChannelJob(ctx, channelID, jobID, priority, due); err != nil {
						t.Fatal(err)
					}
					for n := 0; n < 2; n++ {
						if err := new(sSysPublish).completeTelegramOutbox(ctx, jobID); err != nil {
							t.Fatal(err)
						}
					}
					member, err := telegramChannelRedisDo(ctx, "ZSCORE", telegramChannelQueueKeyForPriority(channelID, priority), jobID)
					if err != nil {
						t.Fatal(err)
					}
					if enabled && member.Int64() != telegramChannelQueueScore(priority, due, jobID) {
						t.Fatalf("priority10 disappeared or due changed after Outbox complete: %v", member)
					}
					if !enabled && priority == 10 && !member.IsNil() {
						t.Fatal("legacy Outbox cleanup no longer removes pending index")
					}
					status, err := db.GetValue(ctx, "SELECT status FROM "+publishTgOutboxTable+" WHERE id=?", jobID)
					if err != nil || status.String() != telegramOutboxDelivered {
						t.Fatalf("completion status=%v err=%v", status, err)
					}
				}
			})
		}
	})
	restoredGroup, restoredErr := gdb.GetConfigGroup("default")
	if (previousErr == nil) != (restoredErr == nil) || !reflect.DeepEqual(previousGroup, restoredGroup) {
		t.Fatal("local PostgreSQL cleanup did not restore the original default DB group")
	}
}
