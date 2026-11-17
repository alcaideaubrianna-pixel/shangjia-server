package collectobserve

import (
	"context"
	"testing"
	"time"
)

func TestMetadataBranchesAreIsolated(t *testing.T) {
	parent := With(context.Background(), Metadata{
		TenantID: 78, SourceID: 351, EventID: 100, RuleID: 3,
		SourceType: "bot", Mode: "history", ReceivedAt: time.Unix(1700000000, 0),
	})
	left := With(parent, Metadata{RuleID: 4, ProfileID: 123})
	right := With(parent, Metadata{RuleID: 5})
	if From(parent).RuleID != 3 || From(left).RuleID != 4 || From(right).RuleID != 5 || From(right).ProfileID != 0 {
		t.Fatal("branch identity leaked to another context")
	}
	nextEvent := From(With(left, Metadata{EventID: 101}))
	if nextEvent.RuleID != 0 || nextEvent.ProfileID != 0 {
		t.Fatal("new event inherited previous event's branch/profile")
	}
	if nextEvent.SourceID != 351 || nextEvent.Mode != "unknown" || !nextEvent.ReceivedAt.IsZero() {
		t.Fatal("enrichment lost collection identity")
	}
}

func TestMetadataDimensionsAreBounded(t *testing.T) {
	ctx := With(nil, Metadata{SourceType: "token-or-channel", Mode: "arbitrary-user-string"})
	meta := From(ctx)
	if meta.SourceType != "unknown" || meta.Mode != "unknown" {
		t.Fatal("unbounded metric dimensions")
	}
	if From(context.Background()) != (Metadata{}) {
		t.Fatal("empty context should have no identity")
	}
	if ModeForHistory(true) != "history" || ModeForHistory(false) != "realtime" {
		t.Fatal("incorrect collection mode")
	}
}
