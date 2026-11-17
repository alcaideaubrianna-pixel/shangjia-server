package sys

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
)

// Pure fixtures only: these tests never initialize Redis, open a database, or
// enqueue tasks. The visitor models the existing enqueue/reservation boundary.
func botMediaRecoveryTestRow(id int64, at string, eligible bool) gdb.Record {
	return gdb.Record{
		"id": gvar.New(id), "recovery_updated_at": gvar.New(at), "recovery_eligible": gvar.New(eligible),
	}
}

func TestBotMediaRecoveryPageAdvancesPastIneligiblePrefix(t *testing.T) {
	cursor := collectBotMediaRecoveryCursor{Fence: "2026-11-18 10:00:00.999999"}
	rows := make(gdb.Result, 0, collectBotMediaRecoveryPageSize)
	for id := int64(1); id <= collectBotMediaRecoveryPageSize; id++ {
		rows = append(rows, botMediaRecoveryTestRow(id, "2026-11-18 09:00:00.123456", false))
	}
	visits := 0
	visit := func(gdb.Record) error { visits++; return nil }
	advanced, err := walkCollectBotMediaRecoveryPage(&cursor, rows, visit)
	if err != nil || advanced != collectBotMediaRecoveryPageSize || cursor.ID != 12 || visits != 0 {
		t.Fatalf("ineligible prefix did not advance: cursor=%+v advanced=%d visits=%d err=%v", cursor, advanced, visits, err)
	}
	// A due event after a full skipped page must still get its turn.
	advanced, err = walkCollectBotMediaRecoveryPage(&cursor, gdb.Result{
		botMediaRecoveryTestRow(13, "2026-11-18 09:00:00.123456", true),
	}, visit)
	if err != nil || advanced != 1 || cursor.ID != 13 || visits != 1 {
		t.Fatalf("later eligible event starved: cursor=%+v visits=%d err=%v", cursor, visits, err)
	}
}

func TestBotMediaRecoveryPageFailureRetainsPreviousCandidate(t *testing.T) {
	cursor := collectBotMediaRecoveryCursor{Fence: "2026-11-18 10:00:00.999999"}
	wantErr := errors.New("enqueue unavailable")
	rows := gdb.Result{
		botMediaRecoveryTestRow(1, "2026-11-18 09:00:00.123456", false),
		botMediaRecoveryTestRow(2, "2026-11-18 09:00:00.123456", true),
		botMediaRecoveryTestRow(3, "2026-11-18 09:00:00.123457", true),
		botMediaRecoveryTestRow(4, "2026-11-18 09:00:00.123458", true),
	}
	var visited []int64
	advanced, err := walkCollectBotMediaRecoveryPage(&cursor, rows, func(row gdb.Record) error {
		id := row["id"].Int64()
		visited = append(visited, id)
		if id == 3 {
			return wantErr
		}
		return nil
	})
	if !errors.Is(err, wantErr) || advanced != 2 || cursor.ID != 2 || !reflect.DeepEqual(visited, []int64{2, 3}) {
		t.Fatalf("advanced past failed enqueue: cursor=%+v visits=%v advanced=%d err=%v", cursor, visited, advanced, err)
	}
	// A subsequent pass starts with the failed event, not its successor.
	_, err = walkCollectBotMediaRecoveryPage(&cursor, rows[2:], func(row gdb.Record) error {
		visited = append(visited, row["id"].Int64())
		return nil
	})
	if err != nil || !reflect.DeepEqual(visited, []int64{2, 3, 3, 4}) || cursor.ID != 4 {
		t.Fatalf("failed event not retried: visits=%v cursor=%+v err=%v", visited, cursor, err)
	}
}

func TestBotMediaRecoveryFencePersistsWithExactMicroseconds(t *testing.T) {
	at := time.Date(2026, 11, 18, 10, 0, 10, 123456000, time.FixedZone("CST", 8*60*60))
	var cursor collectBotMediaRecoveryCursor
	cursor.begin(at)
	if cursor.Fence != "2026-11-18 10:00:00.123456" {
		t.Fatalf("fence lost wall clock or microseconds: %+v", cursor)
	}
	cursor.UpdatedAt, cursor.ID = "2026-11-18 09:00:00.123457", 7
	data, err := json.Marshal(cursor)
	if err != nil {
		t.Fatal(err)
	}
	var resumed collectBotMediaRecoveryCursor
	if err = json.Unmarshal(data, &resumed); err != nil {
		t.Fatal(err)
	}
	resumed.begin(at.Add(time.Hour))
	if resumed != cursor {
		t.Fatalf("resuming moved fixed fence or truncated timestamp: got=%+v want=%+v", resumed, cursor)
	}
	_, err = walkCollectBotMediaRecoveryPage(&resumed, gdb.Result{
		botMediaRecoveryTestRow(8, "2026-11-18 09:00:00.123458", false),
	}, func(gdb.Record) error { t.Fatal("skipped row visited"); return nil })
	if err != nil || resumed.UpdatedAt != "2026-11-18 09:00:00.123458" {
		t.Fatalf("microsecond keyset did not survive advancement: %+v err=%v", resumed, err)
	}
}

func TestBotMediaRecoveryCorruptCachedHintResetsPosition(t *testing.T) {
	for _, data := range []string{
		`{"fence":`,
		`{"fence":"bad","updatedAt":"2026-11-18 09:00:00.123456","id":7}`,
		`{"fence":"2026-11-18 10:00:00.123456","id":7}`,
		`{"fence":"2026-11-18 10:00:00.123456","updatedAt":"2026-11-18 09:00:00.123456","id":-1}`,
		`{"fence":"2026-11-18 10:00:00.123456","updatedAt":"2026-11-18 10:00:00.123457","id":7}`,
	} {
		cursor, err := parseCollectBotMediaRecoveryCursor(data)
		if err == nil || cursor != (collectBotMediaRecoveryCursor{}) {
			t.Fatalf("corrupt cached hint retained: cursor=%+v err=%v", cursor, err)
		}
		cursor.begin(time.Date(2026, 11, 18, 10, 0, 10, 123456000, time.UTC))
		if cursor.ID != 0 || cursor.UpdatedAt != "" || cursor.Fence != "2026-11-18 10:00:00.123456" {
			t.Fatalf("reset hint cannot begin a fresh sweep: %+v", cursor)
		}
	}
	valid := collectBotMediaRecoveryCursor{
		Fence: "2026-11-18 10:00:00.123456", UpdatedAt: "2026-11-18 09:00:00.123456", ID: 7,
	}
	data, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := parseCollectBotMediaRecoveryCursor(string(data))
	if err != nil || cursor != valid {
		t.Fatalf("valid cached hint reset: cursor=%+v err=%v", cursor, err)
	}
}

func TestBotMediaRecoveryRejectsCandidateOutsideFence(t *testing.T) {
	cursor := collectBotMediaRecoveryCursor{Fence: "2026-11-18 10:00:00.123456"}
	before := cursor
	advanced, err := walkCollectBotMediaRecoveryPage(&cursor, gdb.Result{
		botMediaRecoveryTestRow(1, "2026-11-18 10:00:00.123457", true),
	}, func(gdb.Record) error { t.Fatal("candidate outside fence visited"); return nil })
	if err == nil || advanced != 0 || cursor != before {
		t.Fatalf("candidate outside fence accepted: cursor=%+v advanced=%d err=%v", cursor, advanced, err)
	}
}

func TestBotMediaRecoveryPageRejectsMissingOrRegressingPosition(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   int64
		at   string
	}{
		{"missing_id", 0, "2026-11-18 09:00:00.123457"},
		{"negative_id", -1, "2026-11-18 09:00:00.123457"},
		{"missing_timestamp", 8, ""},
		{"duplicate", 7, "2026-11-18 09:00:00.123456"},
		{"lower_id_same_timestamp", 6, "2026-11-18 09:00:00.123456"},
		{"earlier_timestamp_higher_id", 8, "2026-11-18 09:00:00.123455"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cursor := collectBotMediaRecoveryCursor{
				Fence: "2026-11-18 10:00:00.123456", UpdatedAt: "2026-11-18 09:00:00.123456", ID: 7,
			}
			before := cursor
			advanced, err := walkCollectBotMediaRecoveryPage(&cursor, gdb.Result{
				botMediaRecoveryTestRow(tc.id, tc.at, true),
			}, func(gdb.Record) error { t.Fatal("invalid cursor position visited"); return nil })
			if err == nil || advanced != 0 || cursor != before {
				t.Fatalf("invalid position accepted: cursor=%+v advanced=%d err=%v", cursor, advanced, err)
			}
		})
	}
}

func TestBotMediaRecoveryWrapRevisitsSkippedFutureRetry(t *testing.T) {
	start := time.Date(2026, 11, 18, 10, 0, 10, 123456000, time.UTC)
	var cursor collectBotMediaRecoveryCursor
	cursor.begin(start)
	rows := gdb.Result{botMediaRecoveryTestRow(1, "2026-11-18 09:00:00.123456", false)}
	_, err := walkCollectBotMediaRecoveryPage(&cursor, rows, func(gdb.Record) error {
		t.Fatal("future retry must be skipped")
		return nil
	})
	if err != nil || cursor.finishPage(12, 12) {
		t.Fatalf("full page must preserve position: %+v err=%v", cursor, err)
	}
	if !cursor.finishPage(len(rows), 12) || cursor != (collectBotMediaRecoveryCursor{}) {
		t.Fatalf("drained sweep did not reset cursor/fence: %+v", cursor)
	}
	cursor.begin(start.Add(time.Minute))
	if cursor.Fence != "2026-11-18 10:01:00.123456" || cursor.ID != 0 {
		t.Fatalf("next sweep did not establish a fresh fence from the start: %+v", cursor)
	}
	// Retry eligibility changes without any parent event timestamp change.
	rows[0]["recovery_eligible"] = gvar.New(true)
	visits := 0
	_, err = walkCollectBotMediaRecoveryPage(&cursor, rows, func(gdb.Record) error { visits++; return nil })
	if err != nil || visits != 1 || cursor.ID != 1 {
		t.Fatalf("wrapped sweep lost previously ineligible event: visits=%d cursor=%+v err=%v", visits, cursor, err)
	}
}

func TestBotMediaRecoverySQLBoundsBeforePermissionAndRetry(t *testing.T) {
	cursor := collectBotMediaRecoveryCursor{
		Fence: "2026-11-18 10:00:00.123456", UpdatedAt: "2026-11-18 09:00:00.123457", ID: 17,
	}
	query, args := collectBotMediaRecoveryPageSQL(publishAccountScope{TenantID: 87, AccountID: 0, SourceID: 1237},
		cursor, 50, "2026-11-18 10:00:10.123456")
	boundary := strings.Index(query, " LIMIT ?) SELECT")
	if boundary < 0 || !strings.HasPrefix(query, "WITH candidates AS MATERIALIZED") {
		t.Fatal("candidate page must be materialized before qualification")
	}
	candidateSQL, qualificationSQL := query[:boundary], query[boundary:]
	if strings.Contains(query, "e.*") || strings.Contains(query, "raw_text") || strings.Contains(query, "media_json") {
		t.Fatalf("candidate page reads wide event data: %s", query)
	}
	projectionEnd := strings.Index(candidateSQL, " FROM ")
	for _, field := range []string{
		"e.id", "e.tenant_id", "e.account_id", "e.source_id", "e.tg_account_id",
		"e.received_at", "e.created_at", "e.status", "e.source_type", "e.updated_at",
	} {
		if projectionEnd < 0 || !strings.Contains(candidateSQL[:projectionEnd], field) {
			t.Fatalf("missing payload/eligibility/heartbeat field %q", field)
		}
	}
	for _, predicate := range []string{"EXISTS", "next_retry_at", "source_type=?"} {
		if strings.Contains(candidateSQL, predicate) {
			t.Fatalf("%s moved inside candidate scan: %s", predicate, candidateSQL)
		}
	}
	for _, predicate := range []string{
		"(e.updated_at,e.id)>(CAST(? AS timestamp),?)", "e.updated_at<=CAST(? AS timestamp)",
		"e.tenant_id=? AND e.account_id=? AND e.source_id=?", "ORDER BY e.updated_at,e.id",
		"e.status IN ('media_pending','media_ready','group_collecting','pending','failed')",
	} {
		if !strings.Contains(candidateSQL, predicate) {
			t.Fatalf("missing scoped/fenced keyset predicate %q: %s", predicate, candidateSQL)
		}
	}
	if !strings.Contains(qualificationSQL, enabledCollectSourceExistsSQL("e")) ||
		!strings.Contains(qualificationSQL, "m.account_id=e.account_id") ||
		!strings.Contains(qualificationSQL, "m.next_retry_at IS NULL OR m.next_retry_at<=CAST(? AS timestamp)") {
		t.Fatalf("lost existing source/account0/media eligibility: %s", qualificationSQL)
	}
	if strings.Count(query, "?") != len(args) || len(args) != 10 || args[6] != 12 || args[4] != cursor.UpdatedAt || args[5] != cursor.ID {
		t.Fatalf("wrong bounded page/keyset bindings: %v", args)
	}
}
