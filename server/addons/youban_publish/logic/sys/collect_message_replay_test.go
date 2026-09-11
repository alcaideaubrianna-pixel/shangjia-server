package sys

import (
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestCollectReplayUpdateFieldsResetsTerminalGroupingState(t *testing.T) {
	event := gdb.Record{
		"status":                   gvar.New(sysin.CollectEventStatusIgnored),
		"processed_at":             gvar.New(gtime.Now()),
		"material_role":            gvar.New(collectMaterialRoleDisplay),
		"material_parent_event_id": gvar.New(int64(12)),
		"material_group_status":    gvar.New("paired"),
	}
	update := collectReplayUpdateFields(event, &CollectMessage{SourceGroupedId: "album"}, gtime.Now())

	if got := update["status"]; got != sysin.CollectEventStatusGroupCollect {
		t.Fatalf("status = %v, want %s", got, sysin.CollectEventStatusGroupCollect)
	}
	if got := update["material_role"]; got != collectMaterialRolePending {
		t.Fatalf("material_role = %v, want %s", got, collectMaterialRolePending)
	}
	if got := update["material_parent_event_id"]; got != 0 {
		t.Fatalf("material_parent_event_id = %v, want 0", got)
	}
	if got := update["material_group_status"]; got != "" {
		t.Fatalf("material_group_status = %v, want empty", got)
	}
	if _, ok := update["processed_at"]; !ok || update["processed_at"] != nil {
		t.Fatalf("processed_at = %v, want explicit nil", update["processed_at"])
	}
}

func TestCollectReplayUpdateFieldsKeepsActiveGroupingState(t *testing.T) {
	event := gdb.Record{"status": gvar.New(sysin.CollectEventStatusGroupCollect)}
	update := collectReplayUpdateFields(event, &CollectMessage{SourceGroupedId: "album"}, gtime.Now())

	for _, field := range []string{"processed_at", "material_role", "material_parent_event_id", "material_group_status"} {
		if _, ok := update[field]; ok {
			t.Fatalf("active event update unexpectedly contains %s", field)
		}
	}
}
