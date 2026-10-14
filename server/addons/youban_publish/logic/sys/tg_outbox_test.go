package sys

import (
	"strings"
	"testing"
	"time"

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

func TestTelegramDueOutboxPostgresSQLSelectsChannelHeads(t *testing.T) {
	query := telegramDueOutboxPostgresSQL()
	for _, fragment := range []string{"DISTINCT ON", "j.tenant_id", "j.channel_id", "target_chat_id", "ORDER BY available_at,id"} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("channel-head query missing %q: %s", fragment, query)
		}
	}
}

func TestTelegramOutboxQueryTimeoutIsBounded(t *testing.T) {
	if telegramOutboxQueryTimeout <= 0 || telegramOutboxQueryTimeout > 5*time.Second {
		t.Fatalf("unexpected outbox query timeout: %s", telegramOutboxQueryTimeout)
	}
}

func TestTelegramOutboxRetryDelayIsBounded(t *testing.T) {
	if got := telegramOutboxRetryDelay(0); got != 3*time.Second {
		t.Fatalf("initial retry delay = %s", got)
	}
	if got := telegramOutboxRetryDelay(100); got != time.Minute {
		t.Fatalf("maximum retry delay = %s", got)
	}
}

func TestFairTelegramOutboxRowsSelectsOnePerChannel(t *testing.T) {
	candidates := []telegramJobOutboxRecord{
		{Id: 1, TenantId: 1, ChannelId: 10},
		{Id: 2, TenantId: 1, ChannelId: 10},
		{Id: 3, TenantId: 1, ChannelId: 20},
	}
	rows := fairTelegramOutboxRows(candidates, 10)
	if len(rows) != 2 || rows[0].Id != 1 || rows[1].Id != 3 {
		t.Fatalf("unexpected fair outbox rows: %+v", rows)
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
