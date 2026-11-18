package sys

import (
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
)

func TestTelegramChannelBlockingJobIncludesUnknownAndExcludesWaitingStates(t *testing.T) {
	condition := telegramChannelBlockingJobSQL("j")
	if !strings.Contains(condition, "'unknown'") || !strings.Contains(condition, "'reconciling'") {
		t.Fatalf("unknown/reconciling jobs must block channel order: %s", condition)
	}
	for _, state := range []string{"'pending'", "'failed_retry'", "'retry_wait'"} {
		if strings.Contains(condition, state) {
			t.Fatalf("waiting state %s must not block channel order: %s", state, condition)
		}
	}
}

func TestTelegramChannelBlockingJobSQLParentheses(t *testing.T) {
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql"})
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"", "j", "earlier"} {
		condition, _ := db.Model(publishTgJobTable).Builder().
			Where("id <> ?", 2084649).
			Where(telegramChannelBlockingJobSQL(alias)).
			Where("channel_id", 3976).
			Where("(created_at < ? OR (created_at = ? AND id < ?))", "2026-10-16 14:00:29", "2026-10-16 14:00:29", 2084649).
			Build()
		depth := 0
		for _, char := range condition {
			switch char {
			case '(':
				depth++
			case ')':
				depth--
			}
			if depth < 0 {
				t.Fatalf("unexpected closing parenthesis: %s", condition)
			}
		}
		if depth != 0 {
			t.Fatalf("unclosed parentheses (%d): %s", depth, condition)
		}
	}
}

func TestTelegramChannelBlockingJobIgnoresStaleMissingExecution(t *testing.T) {
	condition := telegramChannelBlockingJobStateSQL("j")
	for _, fragment := range []string{
		"j.status IN ('sending','confirming','unknown','reconciling')",
		"j.dispatch_status = 'processing'",
		"COALESCE(j.status, '') NOT IN ('sent','failed','superseded','cancelled','deleted')",
	} {
		if !strings.Contains(condition, fragment) {
			t.Fatalf("unified blocking condition missing %q in %s", fragment, condition)
		}
	}
}

func TestTelegramChannelExecutionStateIsUnified(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		phase    string
		dispatch string
		want     telegramChannelExecutionState
	}{
		{"pending waits", "pending", "", "idle", telegramChannelExecutionWaiting},
		{"failed retry waits", "failed_retry", "", "idle", telegramChannelExecutionWaiting},
		{"retry wait waits", "retry_wait", "", "idle", telegramChannelExecutionWaiting},
		{"sending blocks", "sending", "", "processing", telegramChannelExecutionBlocking},
		{"confirming blocks", "confirming", "", "processing", telegramChannelExecutionBlocking},
		{"unknown blocks", "unknown", "", "idle", telegramChannelExecutionBlocking},
		{"reconciling blocks", "reconciling", "", "idle", telegramChannelExecutionBlocking},
		{"processing claim blocks", "pending", "", "processing", telegramChannelExecutionBlocking},
		{"terminal releases", "failed", "", "idle", telegramChannelExecutionTerminal},
		{"failed stale verify releases", "failed", "verify_sending", "done", telegramChannelExecutionTerminal},
		{"sent stale display releases", "sent", "display_sending", "processing", telegramChannelExecutionTerminal},
		{"superseded stale cleanup releases", "superseded", "cleanup_sending", "processing", telegramChannelExecutionTerminal},
		{"cancelled stale verify releases", "cancelled", "verify_confirming", "processing", telegramChannelExecutionTerminal},
		{"deleted stale phase releases", "deleted", "reconciling", "processing", telegramChannelExecutionTerminal},
		{"unknown stale verify still blocks", "unknown", "verify_sending", "done", telegramChannelExecutionBlocking},
		{"reconciling stale verify still blocks", "reconciling", "verify_sending", "done", telegramChannelExecutionBlocking},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := telegramChannelJobExecutionState(test.status, test.phase, test.dispatch); got != test.want {
				t.Fatalf("state=%q phase=%q dispatch=%q got=%q want=%q", test.status, test.phase, test.dispatch, got, test.want)
			}
		})
	}
}

func TestTelegramAllChannelHeadQueriesExcludeFailedDisplays(t *testing.T) {
	for name, query := range map[string]string{
		"send gate":        telegramChannelBlockingJobSQL("j"),
		"outbox":           telegramDueOutboxPostgresSQL(),
		"pending recovery": telegramPendingRecoveryPostgresSQL(),
		"earlier recovery": telegramPendingRecoveryChannelHeadSQL("j", "earlier"),
	} {
		if strings.Contains(query, "status='failed'") && !strings.Contains(query, "failed_display_message") {
			t.Fatalf("%s adds an unqualified terminal failure blocker: %s", name, query)
		}
		if strings.Contains(query, "channel_display") {
			t.Fatalf("%s contains obsolete channel_display condition: %s", name, query)
		}
		activeOrder := strings.Contains(query, "'sending','confirming'") ||
			strings.Contains(query, "'pending','failed_retry'") ||
			strings.Contains(query, "'pending','failed_retry','confirming','sending'") ||
			(strings.Contains(query, "'pending','confirming','sending'") && strings.Contains(query, "status='failed_retry'"))
		if name == "pending recovery" {
			// Recovery intentionally selects non-head stale jobs; active send
			// ordering is enforced later by the channel lease.
			continue
		}
		if !activeOrder {
			t.Fatalf("%s no longer preserves active job order: %s", name, query)
		}
	}
}
