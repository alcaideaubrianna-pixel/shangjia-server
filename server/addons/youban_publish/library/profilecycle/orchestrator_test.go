package profilecycle

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	profile Profile
	plans   []*RunPlan
	fail    bool
}

func (s *fakeStore) LoadProfile(context.Context, string) (Profile, error) { return s.profile, nil }
func (s *fakeStore) SavePlan(_ context.Context, plan *RunPlan) error {
	if s.fail {
		return errors.New("persist failed")
	}
	s.plans = append(s.plans, plan)
	return nil
}

func TestOrchestratorKeepsIndexWhenPersistenceFails(t *testing.T) {
	index := &fakeIndex{keys: []string{"run-1"}}
	store := &fakeStore{fail: true, profile: Profile{ID: 1, Channels: []Channel{{ID: 10, Enabled: true, Status: 1, Direction: "up", Mode: "time", Interval: time.Hour}}}}
	orchestrator := &Orchestrator{Index: index, Store: store}
	if _, err := orchestrator.ScheduleDue(context.Background(), time.Now(), 10); err == nil {
		t.Fatal("persistence failure must be returned")
	}
	if len(index.removed) != 0 {
		t.Fatalf("failed run must remain indexed: %v", index.removed)
	}
}

type fakeIndex struct {
	keys    []string
	removed []string
}

func (i *fakeIndex) PopDue(context.Context, time.Time, int) ([]string, error) { return i.keys, nil }
func (i *fakeIndex) Remove(_ context.Context, key string) error {
	i.removed = append(i.removed, key)
	return nil
}

func TestOrchestratorPersistsSynchronizedPlan(t *testing.T) {
	index := &fakeIndex{keys: []string{"run-1"}}
	store := &fakeStore{profile: Profile{ID: 1, Channels: []Channel{{ID: 10, Enabled: true, Status: 1, Direction: "up", Mode: "time", Interval: time.Hour}, {ID: 11, Enabled: true, Status: 1, Direction: "up", Mode: "time", Interval: time.Hour}}}}
	orchestrator := &Orchestrator{Index: index, Store: store}
	processed, err := orchestrator.ScheduleDue(context.Background(), time.Now(), 10)
	if err != nil || processed != 1 || len(store.plans) != 1 || len(store.plans[0].Tasks) != 2 || len(index.removed) != 1 {
		t.Fatalf("processed=%d plans=%d removed=%v err=%v", processed, len(store.plans), index.removed, err)
	}
	if !store.plans[0].Tasks[0].DueAt.Equal(store.plans[0].Tasks[1].DueAt) {
		t.Fatal("profile channels must share due_at")
	}
}
