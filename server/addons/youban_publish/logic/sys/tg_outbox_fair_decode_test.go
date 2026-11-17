package sys

import (
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
)

func TestTelegramOutboxFairDecodePreservesDatabaseIdentity(t *testing.T) {
	record := gdb.Record{
		"id": gvar.New(int64(101)), "job_id": gvar.New(int64(3399912)),
		"tenant_id": gvar.New(int64(25)), "account_id": gvar.New(int64(435)),
		"channel_id": gvar.New(int64(102)), "priority": gvar.New(50),
		"attempt_count": gvar.New(2), "status": gvar.New("pending"),
		"available_at": gvar.New("2026-11-18 05:31:30.123456"),
		"cursor_at":    gvar.New("2026-11-18 05:31:30.123456"),
	}
	rows, err := telegramOutboxFairCandidatesFromRecords(gdb.Result{record})
	if err != nil || len(rows) != 1 {
		t.Fatalf("decode rows=%v err=%v", rows, err)
	}
	row := rows[0]
	if row.Id != 101 || row.JobId != 3399912 || row.TenantId != 25 ||
		row.AccountId != 435 || row.ChannelId != 102 || row.Priority != 50 ||
		row.AttemptCount != 2 || row.Status != telegramOutboxPending ||
		row.AvailableAt == nil || row.CursorAt != "2026-11-18 05:31:30.123456" {
		t.Fatalf("database identity was lost: %+v", row)
	}
}

func TestTelegramOutboxFairDecodeRejectsInvalidRowsBeforeCursorCommit(t *testing.T) {
	valid := gdb.Record{
		"id": gvar.New(101), "job_id": gvar.New(202),
		"tenant_id": gvar.New(25), "channel_id": gvar.New(102),
		"cursor_at": gvar.New("2026-11-18 05:31:30.123456"),
	}
	for _, field := range []string{"id", "job_id", "tenant_id", "channel_id", "cursor_at"} {
		t.Run(field, func(t *testing.T) {
			invalid := gdb.Record{}
			for key, value := range valid {
				invalid[key] = value
			}
			invalid[field] = gvar.New(nil)
			rows, err := telegramOutboxFairCandidatesFromRecords(gdb.Result{valid, invalid})
			if err == nil || rows != nil {
				t.Fatalf("invalid row could advance cursor or dispatch zero: rows=%v err=%v", rows, err)
			}
		})
	}
}
