package sys

import (
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
)

func TestTelegramChannelBlockingJobExcludesUnknownAndUnavailableAccounts(t *testing.T) {
	condition := telegramChannelBlockingJobSQL("j")
	if strings.Contains(condition, "'unknown'") {
		t.Fatalf("unknown reconciliation jobs must not block normal channel order: %s", condition)
	}
	if !strings.Contains(condition, "status='authorized'") || !strings.Contains(condition, "session_key") {
		t.Fatalf("channel blocking condition must require an available account: %s", condition)
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

func TestTelegramAllChannelHeadQueriesExcludeFailedDisplays(t *testing.T) {
	for name, query := range map[string]string{
		"send gate":        telegramChannelBlockingJobSQL("j"),
		"outbox":           telegramDueOutboxPostgresSQL(),
		"pending recovery": telegramPendingRecoveryPostgresSQL(),
		"earlier recovery": telegramPendingRecoveryChannelHeadSQL("j", "earlier"),
	} {
		if strings.Contains(query, "status='failed'") || strings.Contains(query, "channel_display") {
			t.Fatalf("%s still lets terminal display failures block the channel: %s", name, query)
		}
		if !strings.Contains(query, "'pending','failed_retry','confirming','sending'") {
			t.Fatalf("%s no longer preserves active job order: %s", name, query)
		}
	}
}
