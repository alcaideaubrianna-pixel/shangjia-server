package sys

import (
	"context"
	"errors"
	"testing"
	"time"

	"hotgo/internal/library/collectobserve"
)

func TestCollectLifecycleLogEligibilityAndTrustedClock(t *testing.T) {
	now := time.Unix(1794960000, 0)
	for _, tc := range []struct {
		name              string
		created, topLevel bool
		err               error
		received          time.Time
		sample            string
	}{
		{"valid", true, true, nil, now.Add(-time.Hour), "valid"},
		{"missing", true, true, nil, time.Time{}, "missing_received_at"},
		{"future", true, true, nil, now.Add(time.Second), "future_received_at"},
		{"reuse", false, true, nil, now, ""},
		{"nested", true, false, nil, now, ""},
		{"rollback", true, true, errors.New("rollback"), now, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := collectobserve.With(context.Background(), collectobserve.Metadata{
				SourceID: 351, EventID: 123, ChatID: "-1003763589169",
				SourceType: "bot", Mode: "history", ReceivedAt: tc.received,
			})
			fields := collectProfileLifecycleFields(ctx, 99, tc.created, tc.topLevel, tc.err, now)
			if tc.sample == "" {
				if fields != nil {
					t.Fatal("non-creation or uncommitted result emitted lifecycle")
				}
				return
			}
			if fields["lifecycleSample"] != tc.sample || fields["sourceId"] != int64(351) {
				t.Fatalf("wrong lifecycle correlation: %v", fields)
			}
			if tc.sample == "valid" {
				if fields["lifecycleSeconds"] != float64(3600) || fields["slow"] != true {
					t.Fatalf("historical backlog duration lost: %v", fields)
				}
			} else if _, exists := fields["lifecycleSeconds"]; exists {
				t.Fatal("invalid clock fabricated zero lifecycle")
			}
		})
	}
}
