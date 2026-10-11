package sys

import (
	"slices"
	"testing"
)

func TestPublishTaskLogRetentionTargetsExecutionLogsOnly(t *testing.T) {
	want := []publishTaskLogRetentionTarget{
		{table: publishTgJobLogTable, index: "idx_ybp_tg_job_log_created"},
		{table: publishCollectEventLogTable, index: "idx_ybp_collect_event_log_created"},
		{table: publishCollectHistoryLogTable, index: "idx_ybp_collect_history_log_created"},
		{table: publishCycleRunLogTable, index: "idx_ybp_cycle_run_log_created"},
		{table: importRunLogTable, index: "idx_ybp_import_run_log_created"},
	}
	if len(publishTaskLogRetentionTables) != len(want) {
		t.Fatalf("unexpected task-log retention targets: %#v", publishTaskLogRetentionTables)
	}
	for _, target := range want {
		if !slices.Contains(publishTaskLogRetentionTables, target) {
			t.Errorf("task execution log table/index %q/%q is not covered by retention", target.table, target.index)
		}
	}
	for _, protected := range []string{
		publishSuccessRecordTable,
		"hg_youban_publish_tenant_vip_log",
		"hg_youban_publish_image_quota_ledger",
		"hg_content_import_run",
		"hg_content_import_checkpoint",
	} {
		for _, target := range publishTaskLogRetentionTables {
			if target.table != protected {
				continue
			}
			t.Errorf("business/audit record table %q must not be included in task-log retention", protected)
		}
	}
}
