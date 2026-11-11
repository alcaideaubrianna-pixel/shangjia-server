package profilecycle

import (
	"testing"
	"time"
)

// TestBuildPlanPreservesPersistedDeadline 保证配置变更或进程重启不会用当前时间加周期覆盖已保存的发布时间。
func TestBuildPlanPreservesPersistedDeadline(t *testing.T) {
	now := time.Date(2026, 11, 11, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	due := now.Add(2 * time.Hour)
	profile := Profile{ID: 1, TenantID: 1, AccountID: 1, LastSent: now, DueAt: due, Channels: []Channel{{ID: 1, Enabled: true, Status: 1, Direction: "up", Mode: "time", Interval: 72 * time.Hour}}}
	plan := BuildPlan(profile, "persisted-clock", now)
	if plan == nil || !plan.DueAt.Equal(due) || len(plan.Tasks) != 1 || !plan.Tasks[0].DueAt.Equal(due) {
		t.Fatalf("persisted deadline changed: %+v", plan)
	}
}
