package sys

import (
	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"hotgo/addons/youban_publish/model/input/sysin"
	"testing"
)

func TestCollectProcessProductionTerminalStatesAreIdempotent(t *testing.T) {
	fixtures := []struct {
		name, status, role, message string
		finished                    bool
	}{
		{"线上已处理展示", sysin.CollectEventStatusProcessed, collectMaterialRoleDisplay, "", true},
		{"线上已处理验证", sysin.CollectEventStatusProcessed, collectMaterialRoleVerify, "", true},
		{"线上已忽略普通消息", sysin.CollectEventStatusIgnored, collectMaterialRolePending, "", true},
		{"线上已忽略重复展示", sysin.CollectEventStatusIgnored, collectMaterialRoleDisplay, "资料库已存在相同资料", true},
		{"等待验证仍可恢复", sysin.CollectEventStatusIgnored, collectMaterialRoleVerify, collectMaterialVerifyUnmatchedMessage, false},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			got := collectProcessEventFinished(gdb.Record{"status": gvar.New(fixture.status), "material_role": gvar.New(fixture.role), "error_message": gvar.New(fixture.message)})
			if got != fixture.finished {
				t.Fatalf("finished=%v want %v", got, fixture.finished)
			}
		})
	}
}

func TestCollectProcessPayloadMustMatchProductionEventOwner(t *testing.T) {
	event := gdb.Record{"tenant_id": gvar.New(78), "account_id": gvar.New(615), "source_id": gvar.New(351)}
	if !collectProcessEventMatchesPayload(event, collectProcessQueuePayload{TenantId: 78, AccountId: 615, SourceId: 351}) {
		t.Fatal("matching production owner rejected")
	}
	if collectProcessEventMatchesPayload(event, collectProcessQueuePayload{TenantId: 78, AccountId: 615, SourceId: 350}) {
		t.Fatal("different source accepted")
	}
}

func TestCollectProcessV2SeparatesRealtimeRecoveryAndLegacy(t *testing.T) {
	realtime := collectProcessQueuePayload{TenantId: 1, AccountId: 2, SourceId: 3, EventId: 1}
	recovery := realtime
	recovery.Recovery = true
	if collectProcessQueueName(realtime) == collectProcessQueueName(recovery) {
		t.Fatal("realtime and recovery source queues must be isolated")
	}
	if collectProcessQueueName(realtime) == tgQueueNameCollectProcessRealtime {
		t.Fatal("source event must not use the legacy global realtime queue")
	}
}
