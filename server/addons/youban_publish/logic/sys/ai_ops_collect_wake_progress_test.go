package sys

import (
	"context"
	"path/filepath"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestAIOpsCollectWakeProgressBoundary(t *testing.T) {
	if sqlReviewSubprocess(t) {
		return
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "sqlite", Name: filepath.Join(t.TempDir(), "wake-progress.db")}})
	ctx := context.Background()
	if _, err := g.DB().Exec(ctx, `CREATE TABLE hg_youban_publish_collect_event (
		id INTEGER PRIMARY KEY, tenant_id bigint, account_id bigint, source_id bigint,
		source_chat_id text, source_message_id bigint, source_unique_key text,
		created_at text, status text, processed_at text)`); err != nil {
		t.Fatal(err)
	}
	payload := aiOpsRepairPlanPayload{Action: aiOpsCollectWakeAction, TenantID: 78, AccountID: 615, SourceID: 351}
	statuses := []string{"pending", "group_collecting", "ignored", "dispatched", "processed", "failed", "suspended"}
	for i, status := range statuses {
		id := int64(i + 1)
		if _, err := g.DB().Exec(ctx, `INSERT INTO hg_youban_publish_collect_event
			VALUES(?,78,615,351,'-1004411799859',? ,?,'2026-11-18 01:00:00',?,NULL)`, id, id+100, status, status); err != nil {
			t.Fatal(err)
		}
		row, err := g.DB().GetOne(ctx, `SELECT * FROM hg_youban_publish_collect_event WHERE id=?`, id)
		if err != nil {
			t.Fatal(err)
		}
		payload.EventIDs = append(payload.EventIDs, id)
		payload.CollectWakeEvents = append(payload.CollectWakeEvents, &sysin.AIOpsCollectWakePreviewItem{
			EventId: id, TenantId: 78, AccountId: 615, OwnerAccountId: 615, SourceId: 351,
			SourceChatId: "-1004411799859", SourceMessageId: id + 100,
			Decision: sysin.CollectWakeEligible, Identity: collectWakeEventIdentity(row),
		})
	}
	s := &sSysPublish{}
	progress, err := s.aiOpsCollectWakeProgress(ctx, payload)
	if err != nil {
		t.Fatal(err)
	}
	if *progress != (sysin.AIOpsCollectWakeProgress{Total: 7, Pending: 2, Ignored: 1, Dispatched: 1, Processed: 1, Failed: 2}) {
		t.Fatalf("wrong business progress: %+v", progress)
	}
	// A normal state change must not invalidate the immutable plan identity.
	if _, err = g.DB().Exec(ctx, `UPDATE hg_youban_publish_collect_event SET status='dispatched' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	progress, err = s.aiOpsCollectWakeProgress(ctx, payload)
	if err != nil || progress.Dispatched != 2 || progress.Missing != 0 {
		t.Fatalf("state transition lost plan identity: %+v %v", progress, err)
	}
	// Same numeric PK after source/message identity replacement is missing,
	// not a completion receipt for the originally planned message.
	for _, sql := range []string{
		`UPDATE hg_youban_publish_collect_event SET source_message_id=999 WHERE id=1`,
		`UPDATE hg_youban_publish_collect_event SET tenant_id=79 WHERE id=2`,
		`DELETE FROM hg_youban_publish_collect_event WHERE id=3`,
	} {
		if _, err = g.DB().Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	progress, err = s.aiOpsCollectWakeProgress(ctx, payload)
	if err != nil || progress.Missing != 3 || progress.Total != 7 {
		t.Fatalf("replaced or foreign events were counted as done: %+v %v", progress, err)
	}
	bad := payload
	bad.EventIDs = append(append([]int64{}, payload.EventIDs...), payload.EventIDs[0])
	if _, err := s.aiOpsCollectWakeProgress(ctx, bad); err == nil {
		t.Fatal("duplicate plan IDs must be rejected")
	}
	if _, err = g.DB().Exec(ctx, `DROP TABLE hg_youban_publish_collect_event`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.aiOpsCollectWakeProgress(ctx, payload); err == nil {
		t.Fatal("database failure must not report a drained backlog")
	}
}
