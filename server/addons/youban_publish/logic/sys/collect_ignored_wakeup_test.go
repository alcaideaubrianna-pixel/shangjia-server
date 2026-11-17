package sys

import (
	"context"
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
)

func TestCollectIgnoredVerifyStopsWakeupButKeepsPassivePairing(t *testing.T) {
	event := gdb.Record{
		"id":                    gvar.New(int64(19183196)),
		"tenant_id":             gvar.New(int64(78)),
		"account_id":            gvar.New(int64(615)),
		"source_id":             gvar.New(int64(351)),
		"source_chat_id":        gvar.New("-1003763752459"),
		"source_message_id":     gvar.New(int64(1332272)),
		"status":                gvar.New(sysin.CollectEventStatusIgnored),
		"material_role":         gvar.New(collectMaterialRoleVerify),
		"material_group_status": gvar.New("unmatched"),
		"error_message":         gvar.New(collectMaterialVerifyUnmatchedMessage),
	}
	if !collectProcessEventFinished(event) {
		t.Fatal("an ignored verification must release its own durable wakeup")
	}
	if !collectMaterialEventNeedsPairRepair(event) {
		t.Fatal("stopping a wakeup must not remove late-pairing eligibility")
	}
	// No timestamps, Redis, SQL, or source configuration are provided. A
	// terminal target must stop before trying to build another query window.
	s := &sSysPublish{}
	if err := s.processTargetCollectEventWindow(context.Background(), collectProcessQueuePayload{
		EventId: 19183196, TenantId: 78, AccountId: 615, SourceId: 351,
		SourceChatId: "-1003763752459",
	}, event); err != nil {
		t.Fatalf("terminal target attempted more window work: %v", err)
	}
}

func TestCollectLateBoundDisplayStillNeedsRepair(t *testing.T) {
	event := gdb.Record{
		"id":                    gvar.New(int64(100)),
		"source_message_id":     gvar.New(int64(200)),
		"status":                gvar.New(sysin.CollectEventStatusProcessed),
		"material_role":         gvar.New(collectMaterialRoleDisplay),
		"material_group_status": gvar.New("paired"),
	}
	if !collectMaterialLateVerificationRepairCandidate(event) {
		t.Fatal("late-bound processed display must retain its repair path")
	}
	// With no time fields, a late-bound display should reach input validation,
	// rather than be silently acknowledged as an ordinary terminal event.
	if err := (&sSysPublish{}).processTargetCollectEventWindow(
		context.Background(), collectProcessQueuePayload{EventId: 100}, event,
	); err == nil {
		t.Fatal("late-bound display was incorrectly discarded")
	}
}
