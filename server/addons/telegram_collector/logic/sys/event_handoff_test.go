package sys

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestEventHandoffOrderAndRetryOwnership(t *testing.T) {
	failure := errors.New("dependency failure")
	for _, tc := range []struct {
		name    string
		mode    eventHandoffMode
		failAt  string
		want    []string
		wantErr bool
	}{
		{"direct", eventHandoffDirect, "", []string{"save", "ready", "process", "observe"}, false},
		{"save failure belongs to event", eventHandoffDirect, "save", []string{"save"}, true},
		{"ready failure prevents inline", eventHandoffDirect, "ready", []string{"save", "ready"}, true},
		{"handler failure belongs to delivery", eventHandoffDirect, "process", []string{"save", "ready", "process", "observe"}, false},
		{"panic belongs to delivery", eventHandoffDirect, "panic", []string{"save", "ready", "process", "observe"}, false},
		{"queued compatibility", eventHandoffQueued, "", []string{"save", "enqueue", "ready"}, false},
		{"enqueue failure belongs to event", eventHandoffQueued, "enqueue", []string{"save", "enqueue"}, true},
		{"queued ready failure", eventHandoffQueued, "ready", []string{"save", "enqueue", "ready"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			step := func(name string) error {
				calls = append(calls, name)
				if tc.failAt == name {
					return failure
				}
				return nil
			}
			delivery := &sysin.CollectorDelivery{}
			actions := eventHandoffActions{
				save:  func(context.Context, *sysin.CollectorDelivery) (int64, error) { return 42, step("save") },
				ready: func(context.Context) error { return step("ready") },
				enqueue: func(_ context.Context, id int64) error {
					if id != 42 {
						t.Fatal("lost delivery ID")
					}
					return step("enqueue")
				},
				process: func(ctx context.Context, id int64) error {
					if id != 42 || delivery.ID != 42 {
						t.Fatal("lost persisted delivery identity")
					}
					if _, ok := ctx.Deadline(); !ok {
						t.Fatal("missing inline deadline")
					}
					if tc.failAt == "panic" {
						calls = append(calls, "process")
						panic("handler failure")
					}
					return step("process")
				},
				observe: func(_ context.Context, _ int64, err error) {
					calls = append(calls, "observe")
					if (tc.failAt == "process" || tc.failAt == "panic") && err == nil {
						t.Fatal("failure not observed")
					}
				},
			}
			err := executeEventHandoff(context.Background(), tc.mode, 10*time.Second, delivery, actions)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v", err)
			}
			if !reflect.DeepEqual(calls, tc.want) {
				t.Fatalf("calls=%v want=%v", calls, tc.want)
			}
		})
	}
}

func TestEventHandoffTimeoutBelongsToDelivery(t *testing.T) {
	var observed error
	actions := eventHandoffActions{
		save:    func(context.Context, *sysin.CollectorDelivery) (int64, error) { return 42, nil },
		ready:   func(context.Context) error { return nil },
		process: func(ctx context.Context, _ int64) error { <-ctx.Done(); return ctx.Err() },
		observe: func(_ context.Context, _ int64, err error) { observed = err },
	}
	err := executeEventHandoff(context.Background(), eventHandoffDirect, time.Millisecond, &sysin.CollectorDelivery{}, actions)
	if err != nil || !errors.Is(observed, context.DeadlineExceeded) {
		t.Fatalf("event error=%v observed=%v", err, observed)
	}
}

func TestEventHandoffModes(t *testing.T) {
	for _, value := range []string{"direct", "queued"} {
		if _, err := parseEventHandoffMode(value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []string{"", "DIRECT", "unknown"} {
		if _, err := parseEventHandoffMode(value); err == nil {
			t.Fatalf("accepted mode %q", value)
		}
	}
}

func TestEventHandoffHistoryRemainsQueued(t *testing.T) {
	for _, sourceType := range []string{sysin.SourceTypeBot, sysin.SourceTypeAccount} {
		t.Run(sourceType, func(t *testing.T) {
			var calls []string
			actions := eventHandoffActions{
				save: func(context.Context, *sysin.CollectorDelivery) (int64, error) {
					calls = append(calls, "save")
					return 42, nil
				},
				enqueue: func(context.Context, int64) error { calls = append(calls, "enqueue"); return nil },
				ready:   func(context.Context) error { calls = append(calls, "ready"); return nil },
				process: func(context.Context, int64) error { t.Fatal("history must not execute inline"); return nil },
				observe: func(context.Context, int64, error) { t.Fatal("history must use queued lifecycle") },
			}
			delivery := &sysin.CollectorDelivery{History: true, SourceType: sourceType}
			if err := executeEventHandoff(context.Background(), eventHandoffDirect, 10*time.Second, delivery, actions); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(calls, []string{"save", "enqueue", "ready"}) {
				t.Fatalf("calls=%v", calls)
			}
		})
	}
}

func TestEventHandoffIsolationSelection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mode     eventHandoffMode
		history  bool
		priority int
		want     eventHandoffMode
		reason   string
	}{
		{"realtime direct", eventHandoffDirect, false, sysin.EventPriorityRealtime, eventHandoffDirect, ""},
		{"urgent direct", eventHandoffDirect, false, sysin.EventPriorityUrgent, eventHandoffDirect, ""},
		{"urgent history queued", eventHandoffDirect, true, sysin.EventPriorityUrgent, eventHandoffQueued, "history_isolated"},
		{"low priority queued", eventHandoffDirect, false, sysin.EventPriorityNormal, eventHandoffQueued, "history_isolated"},
		{"configured queued", eventHandoffQueued, false, sysin.EventPriorityUrgent, eventHandoffQueued, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mode, reason := selectEventHandoffMode(tc.mode, tc.history, tc.priority)
			if mode != tc.want || reason != tc.reason {
				t.Fatalf("mode=%s reason=%s", mode, reason)
			}
		})
	}
}
