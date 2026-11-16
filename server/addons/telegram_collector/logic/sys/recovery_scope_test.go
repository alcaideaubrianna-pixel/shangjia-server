package sys

import (
	"context"
	"testing"
)

func TestCollectorRecoveryScopeRawBotBoundary(t *testing.T) {
	for _, member := range []string{"1:0:0", "1:2:0", "1:2:3"} {
		scope, err := parseCollectorRecoveryScope(member)
		if err != nil || scope.key() != member {
			t.Fatalf("valid scope rejected: %s %v", member, err)
		}
	}
	for _, member := range []string{"0:0:0", "1:-1:0", "1:0:-1", "1:2", "a:0:0"} {
		if _, err := parseCollectorRecoveryScope(member); err == nil {
			t.Fatalf("invalid scope accepted: %s", member)
		}
	}
}
func TestEventRecoveryRejectsUnscopedBeforeDatabase(t *testing.T) {
	if _, err := eventRecoveryPage(context.Background(), "received", eventRecoveryCursor{}, 50); err == nil {
		t.Fatal("unscoped query accepted")
	}
	if err := (&sCollector{}).resetExpiredEventLeases(context.Background(), 50, collectorRecoveryScope{}); err == nil {
		t.Fatal("invalid lease scope accepted")
	}
}
