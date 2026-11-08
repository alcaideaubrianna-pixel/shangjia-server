package profilecycle

import (
	"testing"
	"time"
)

// TestProductionShapeReplay220734 使用 DBX 只读样本的频道配置形状回放。
// 线上该资料的四个频道 next_cycle_at 相差约 10 分钟；资料级规划必须合并为同一 due_at。
func TestProductionShapeReplay220734(t *testing.T) {
	now := time.Date(2026, 11, 8, 16, 0, 0, 0, time.Local)
	plan := BuildPlan(Profile{
		ID: 220734, TenantID: 9, AccountID: 17, LastSent: now.Add(-72 * time.Hour),
		Channels: []Channel{
			{ID: 29, Enabled: true, Status: 1, Direction: "up", Mode: "time", Interval: 72 * time.Hour, PublishTime: "09:10"},
			{ID: 30, Enabled: true, Status: 1, Direction: "up", Mode: "time", Interval: 72 * time.Hour, PublishTime: "09:10"},
			{ID: 31, Enabled: true, Status: 1, Direction: "up", Mode: "time", Interval: 72 * time.Hour, PublishTime: "09:10"},
			{ID: 167, Enabled: true, Status: 1, Direction: "up", Mode: "time", Interval: 72 * time.Hour, PublishTime: "09:00"},
		},
	}, "production-replay-220734", now)
	if plan == nil || len(plan.Tasks) != 4 {
		t.Fatalf("plan=%+v", plan)
	}
	for _, task := range plan.Tasks {
		if !task.DueAt.Equal(plan.DueAt) {
			t.Fatalf("profile 220734 task channel=%d due=%s plan=%s", task.ChannelID, task.DueAt, plan.DueAt)
		}
	}
}

func TestProductionShapeReplay220468MixedChannels(t *testing.T) {
	now := time.Date(2026, 11, 8, 16, 0, 0, 0, time.Local)
	plan := BuildPlan(Profile{ID: 220468, TenantID: 3, AccountID: 8, LastSent: now.Add(-365 * 24 * time.Hour), Channels: []Channel{
		{ID: 7, Enabled: true, Status: 1, Direction: "up", Mode: "time", Interval: 365 * 24 * time.Hour},
		{ID: 8, Enabled: false, Status: 1, Direction: "up", Mode: "time", Interval: 365 * 24 * time.Hour},
		{ID: 9, Enabled: false, Status: 1, Direction: "up", Mode: "time", Interval: 365 * 24 * time.Hour},
		{ID: 19, Enabled: true, Status: 1, Direction: "up", Mode: "time", Interval: 365 * 24 * time.Hour},
		{ID: 41, Enabled: false, Status: 1, Direction: "up", Mode: "time", Interval: 365 * 24 * time.Hour},
		{ID: 79, Enabled: true, Status: 1, Direction: "up", Mode: "time", Interval: 365 * 24 * time.Hour},
	}}, "production-replay-220468", now)
	if plan == nil || len(plan.Tasks) != 3 {
		t.Fatalf("mixed channel plan=%+v", plan)
	}
	for _, task := range plan.Tasks {
		if task.ChannelID != 7 && task.ChannelID != 19 && task.ChannelID != 79 {
			t.Fatalf("disabled channel included: %d", task.ChannelID)
		}
	}
}
