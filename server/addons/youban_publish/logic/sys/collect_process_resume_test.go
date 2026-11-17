package sys

import (
	"context"
	"errors"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestCollectProcessResumeClassifiedDisplayRoutes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    string
		role      string
		group     string
		hasVerify bool
		probeErr  error
		wantProbe bool
		wantRoute bool
		wantWait  bool
		wantErr   bool
	}{
		{name: "paired_media_ready", status: sysin.CollectEventStatusMediaReady, role: collectMaterialRoleDisplay, group: "paired", hasVerify: true, wantProbe: true, wantRoute: true},
		{name: "paired_missing_verify", status: sysin.CollectEventStatusMediaReady, role: collectMaterialRoleDisplay, group: "paired", wantProbe: true, wantWait: true, wantErr: true},
		{name: "paired_probe_failure", status: sysin.CollectEventStatusMediaReady, role: collectMaterialRoleDisplay, group: "paired", probeErr: errors.New("database unavailable"), wantProbe: true, wantErr: true},
		{name: "paired_group_collecting", status: sysin.CollectEventStatusGroupCollect, role: collectMaterialRoleDisplay, group: "paired", hasVerify: true, wantProbe: true, wantRoute: true},
		{name: "paired_prechecked", status: sysin.CollectEventStatusPrechecked, role: collectMaterialRoleDisplay, group: "paired", hasVerify: true, wantProbe: true, wantRoute: true},
		{name: "paired_media_pending_keeps_ready_checks", status: sysin.CollectEventStatusMediaPending, role: collectMaterialRoleDisplay, group: "paired", hasVerify: true, wantProbe: true, wantRoute: true},
		{name: "complete_without_verify", status: sysin.CollectEventStatusMediaReady, role: collectMaterialRoleDisplay, group: "complete", wantRoute: true},
		{name: "pending_status", status: sysin.CollectEventStatusPending, role: collectMaterialRoleDisplay, group: "paired", hasVerify: true},
		{name: "pending_group", status: sysin.CollectEventStatusMediaReady, role: collectMaterialRoleDisplay, group: "pending"},
		{name: "waiting_verify", status: sysin.CollectEventStatusMediaReady, role: collectMaterialRoleDisplay, group: collectMaterialGroupWaitingVerify},
		{name: "waiting_order", status: sysin.CollectEventStatusWaitingOrder, role: collectMaterialRoleDisplay, group: "paired", hasVerify: true},
		{name: "unmatched", status: sysin.CollectEventStatusMediaReady, role: collectMaterialRoleDisplay, group: "unmatched"},
		{name: "collecting", status: sysin.CollectEventStatusGroupCollect, role: collectMaterialRoleDisplay, group: collectMaterialGroupCollecting},
		{name: "empty_group", status: sysin.CollectEventStatusMediaReady, role: collectMaterialRoleDisplay},
		{name: "unknown_group", status: sysin.CollectEventStatusMediaReady, role: collectMaterialRoleDisplay, group: "future_status"},
		{name: "pending_role", status: sysin.CollectEventStatusMediaReady, role: collectMaterialRolePending, group: "paired", hasVerify: true},
		{name: "verify_role", status: sysin.CollectEventStatusMediaReady, role: collectMaterialRoleVerify, group: "paired", hasVerify: true},
		{name: "ignored", status: sysin.CollectEventStatusIgnored, role: collectMaterialRoleDisplay, group: "paired", hasVerify: true},
		{name: "processed_keeps_existing_late_repair_gate", status: sysin.CollectEventStatusProcessed, role: collectMaterialRoleDisplay, group: "paired", hasVerify: true},
		{name: "dispatched", status: sysin.CollectEventStatusDispatched, role: collectMaterialRoleDisplay, group: "complete"},
		{name: "failed_keeps_existing_recovery_policy", status: sysin.CollectEventStatusFailed, role: collectMaterialRoleDisplay, group: "paired", hasVerify: true},
		{name: "unknown_status", status: "future_status", role: collectMaterialRoleDisplay, group: "complete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := gdb.Record{
				"id": g.NewVar(16245411), "tenant_id": g.NewVar(78),
				"account_id": g.NewVar(0), "source_id": g.NewVar(351),
				"source_chat_id": g.NewVar("-1003763752459"),
				"status": g.NewVar(tc.status), "material_role": g.NewVar(tc.role),
				"material_group_status": g.NewVar(tc.group),
			}
			type marker struct{}
			ctx := context.WithValue(context.Background(), marker{}, "same-attempt")
			calls := 0
			direct, err := collectProcessResumeClassifiedDisplay(ctx, event, func(callCtx context.Context, got gdb.Record) (bool, error) {
				calls++
				if callCtx.Value(marker{}) != "same-attempt" || got["id"].Int64() != 16245411 ||
					got["account_id"].Int64() != 0 || got["source_id"].Int64() != 351 {
					t.Fatal("verify probe must preserve context and stored event identity")
				}
				return tc.hasVerify, tc.probeErr
			})
			if direct != tc.wantRoute || (err != nil) != tc.wantErr {
				t.Fatalf("direct=%v err=%v; want direct=%v error=%v", direct, err, tc.wantRoute, tc.wantErr)
			}
			wantCalls := 0
			if tc.wantProbe {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatalf("verify probes=%d, want %d", calls, wantCalls)
			}
			var retry *collectProcessRetryError
			if errors.As(err, &retry) != tc.wantWait {
				t.Fatalf("paired missing must wait without display-only fallback: %v", err)
			}
			if tc.probeErr != nil && !errors.Is(err, tc.probeErr) {
				t.Fatalf("database probe error must propagate: %v", err)
			}
			if event["status"].String() != tc.status || event["account_id"].Int64() != 0 {
				t.Fatal("routing must not mutate event state or account")
			}
		})
	}
}

func TestCollectProcessResumePairedRequiresProbe(t *testing.T) {
	event := gdb.Record{
		"material_role": g.NewVar(collectMaterialRoleDisplay),
		"status": g.NewVar(sysin.CollectEventStatusMediaReady),
		"material_group_status": g.NewVar("paired"),
	}
	if direct, err := collectProcessResumeClassifiedDisplay(context.Background(), event, nil); direct || err == nil {
		t.Fatal("paired marker alone must never authorize direct processing")
	}
}
