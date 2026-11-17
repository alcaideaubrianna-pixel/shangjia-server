package sys

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTelegramWaitingJobsHaveNoAgeCutoffInDispatchOrChannelOrder(t *testing.T) {
	for _, target := range []struct {
		file string
		name string
	}{
		{"tg_outbox.go", "dueTelegramStreamOutboxRows"},
		{"tg_channel_order.go", "telegramChannelHasEarlierActiveJob"},
	} {
		t.Run(target.name, func(t *testing.T) {
			source, err := os.ReadFile(target.file)
			if err != nil {
				t.Fatal(err)
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, target.file, source, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Name.Name != target.name {
					continue
				}
				body := strings.ToLower(string(source[fset.Position(fn.Pos()).Offset:fset.Position(fn.End()).Offset]))
				// Queue age is not expiry. Both interactive full-push and
				// bulk jobs must remain eligible after waiting ten minutes.
				for _, forbidden := range []string{"updated_at", "headtimeout"} {
					if strings.Contains(body, forbidden) {
						t.Fatalf("%s must not filter waiting jobs by age (%s)", target.name, forbidden)
					}
				}
				return
			}
			t.Fatalf("missing function %s", target.name)
		})
	}
}

func TestTelegramRecoveryDoesNotAbandonStalePendingChannelHeads(t *testing.T) {
	source, err := os.ReadFile("tg_job_recovery.go")
	if err != nil {
		t.Fatal(err)
	}
	// Reject restoration of either the age-only cleanup or its automatic
	// invocation. Explicit business expiry remains a separate policy.
	if strings.Contains(string(source), "supersedeStalePendingChannelHeads") {
		t.Fatal("normal pending jobs must be recovered, not superseded for queue age")
	}
	// Recovery must continue through the existing bounded query.
	query := telegramPendingRecoveryPostgresSQL()
	if !strings.Contains(query, "LIMIT ?") {
		t.Fatal("pending recovery must retain its batch limit")
	}
}

func TestTelegramRecoveryAndDispatchLocksUseFixedLeases(t *testing.T) {
	for _, file := range []string{"tg_job_recovery.go", "tg_channel_order.go"} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(source), "WithoutWatchDog()") {
			t.Fatalf("%s must not renew a recovery or dispatch lease indefinitely", file)
		}
	}
}

func TestTelegramJobClaimRejectsExpiredDeadline(t *testing.T) {
	source, err := os.ReadFile("tg_send_repo.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "NotExpiredAt: now") {
		t.Fatal("job claim must fence queue tasks whose durable send deadline expired")
	}
}

func TestTelegramTimeoutCleanupClaimsLegacyNullSchedule(t *testing.T) {
	if !strings.Contains(telegramLegacyTimeoutCleanupClaimSQL, "timeout_cleanup_at IS NULL") {
		t.Fatal("timeout cleanup must include an indexed legacy NULL lane")
	}
	if telegramChannelDispatchLeaseTTL != 30*time.Second {
		t.Fatalf("unexpected dispatch lease TTL: %s", telegramChannelDispatchLeaseTTL)
	}
}
