package sys

import (
	"testing"
	"time"
)

func TestMediaFileCachePrunerClaim(t *testing.T) {
	var pruner mediaFileCachePruner
	now := time.Unix(1000, 0)
	interval := 5 * time.Minute

	if !pruner.claim(now, interval) {
		t.Fatal("first prune must be accepted")
	}
	if pruner.claim(now.Add(interval), interval) {
		t.Fatal("running prune must not overlap")
	}

	pruner.release()
	if pruner.claim(now.Add(interval-time.Second), interval) {
		t.Fatal("prune within the interval must be skipped")
	}
	if !pruner.claim(now.Add(interval), interval) {
		t.Fatal("prune after the interval must be accepted")
	}
}
