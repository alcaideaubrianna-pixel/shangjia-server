package sys

import (
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
)

func TestTelegramJobOutboxRecordFromRecordAllowsMissingLegacyOutbox(t *testing.T) {
	if row := telegramJobOutboxRecordFromRecord(nil); row.Id != 0 {
		t.Fatalf("missing legacy outbox must remain empty, got id=%d", row.Id)
	}
	record := gdb.Record{
		"id":     gvar.New(int64(12)),
		"job_id": gvar.New(int64(34)),
	}
	row := telegramJobOutboxRecordFromRecord(record)
	if row.Id != 12 || row.JobId != 34 {
		t.Fatalf("unexpected outbox record: %+v", row)
	}
}

func TestTelegramOutboxJobStillNeedsDispatch(t *testing.T) {
	tests := []struct {
		name           string
		status         string
		dispatchStatus string
		want           bool
	}{
		{name: "pending idle", status: "pending", dispatchStatus: tgDispatchStatusIdle, want: true},
		{name: "retry empty", status: "failed_retry", dispatchStatus: "", want: true},
		{name: "unknown idle", status: "unknown", dispatchStatus: tgDispatchStatusIdle, want: true},
		{name: "queued", status: "pending", dispatchStatus: tgDispatchStatusQueued, want: false},
		{name: "processing", status: "pending", dispatchStatus: tgDispatchStatusProcessing, want: false},
		{name: "sent", status: "sent", dispatchStatus: tgDispatchStatusDone, want: false},
		{name: "failed", status: "failed", dispatchStatus: tgDispatchStatusDone, want: false},
		{name: "superseded", status: "superseded", dispatchStatus: tgDispatchStatusDone, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := telegramOutboxJobStillNeedsDispatch(test.status, test.dispatchStatus); got != test.want {
				t.Fatalf("telegramOutboxJobStillNeedsDispatch(%q, %q)=%t, want %t", test.status, test.dispatchStatus, got, test.want)
			}
		})
	}
}
