package profilecycle

import (
	"testing"
	"time"
)

func TestBuildPlanSynchronizesProfileChannels(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.Local)
	plan := BuildPlan(Profile{ID: 7, TenantID: 1, AccountID: 2, LastSent: now.Add(-24 * time.Hour), Channels: []Channel{
		{ID: 20, Enabled: true, Status: 1, Direction: "up", Mode: "time", Interval: 24 * time.Hour},
		{ID: 10, Enabled: true, Status: 1, Direction: "up", Mode: "time", Interval: 48 * time.Hour},
		{ID: 30, Enabled: false, Status: 1, Direction: "up", Mode: "time", Interval: 24 * time.Hour},
	}}, "run-7", now)
	if plan == nil || len(plan.Tasks) != 2 {
		t.Fatalf("plan=%+v", plan)
	}
	if plan.Tasks[0].ChannelID != 10 || !plan.Tasks[0].DueAt.Equal(plan.Tasks[1].DueAt) {
		t.Fatalf("tasks are not synchronized: %+v", plan.Tasks)
	}
}

func TestBuildPlanLargeReplay(t *testing.T) {
	now := time.Now()
	count := 10000
	planned := 0
	for i := 1; i <= count; i++ {
		plan := BuildPlan(Profile{ID: int64(i), LastSent: now.Add(-48 * time.Hour), Channels: []Channel{
			{ID: int64(i*2 + 1), Enabled: true, Status: 1, Direction: "up", Mode: "time", Interval: 24 * time.Hour},
			{ID: int64(i*2 + 2), Enabled: true, Status: 1, Direction: "up", Mode: "time", Interval: 24 * time.Hour},
		}}, "replay", now)
		if plan == nil || len(plan.Tasks) != 2 || !plan.Tasks[0].DueAt.Equal(plan.Tasks[1].DueAt) {
			t.Fatalf("profile %d plan invalid: %+v", i, plan)
		}
		planned += len(plan.Tasks)
	}
	if planned != count*2 {
		t.Fatalf("planned=%d want=%d", planned, count*2)
	}
}
