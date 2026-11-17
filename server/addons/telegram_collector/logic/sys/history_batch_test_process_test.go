package sys

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"testing"
	"time"
)

// GoFrame's default-group/instance caches are process-global. Run opt-in SQL
// fixtures in child test processes so they cannot alter other queue tests.
func collectorBatchPostgresChild(t *testing.T) bool {
	t.Helper()
	if os.Getenv("YOUBAN_COLLECTOR_BATCH_TEST_PORT") != "55489" {
		t.Fatal("requires dedicated loopback PostgreSQL 55489")
	}
	if os.Getenv("YOUBAN_COLLECTOR_BATCH_CHILD") == "1" {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+regexp.QuoteMeta(t.Name())+"$", "-test.v")
	cmd.Env = append(os.Environ(), "YOUBAN_COLLECTOR_BATCH_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated SQL fixture: %v\n%s", err, out)
	}
	t.Logf("isolated SQL fixture passed:\n%s", out)
	return false
}
