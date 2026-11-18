package sys

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestProfileCycleOverrideChannelModes(t *testing.T) {
	for _, mode := range []string{"time", "batch", ""} {
		t.Run(mode, func(t *testing.T) {
			channel := profileCycleChannelConfig{
				Id: 11, TenantId: 7, Enabled: 1, Mode: mode, Days: 15,
				PublishTime: "09:30", Status: 1, Direction: "up", Version: 9,
			}
			config := applyProfileCycleOverride(channel, profileCycleOverridePolicy{Enabled: true, Days: 3, Version: 12})
			if !profileCycleChannelUsable(config) || config.Mode != "time" || config.Days != 3 || config.PublishTime != "" {
				t.Fatalf("override did not select independent time policy: %+v", config)
			}
			if config.OverrideVersion != 12 || !config.OverrideEnabled || config.Version != 9 {
				t.Fatalf("channel/profile versions must remain independent: %+v", config)
			}
			inherited := applyProfileCycleOverride(channel, profileCycleOverridePolicy{Days: 3, Version: 13})
			if inherited.Mode != channel.Mode || inherited.Days != 15 || inherited.PublishTime != "09:30" || inherited.OverrideEnabled || inherited.OverrideVersion != 13 {
				t.Fatalf("disabled override must inherit the channel unchanged: %+v", inherited)
			}
			if mode == "batch" && profileCycleChannelUsable(inherited) {
				t.Fatal("inherited batch policy must not become a time plan")
			}
		})
	}
}

func TestProfileCycleOverrideRetainsChannelGates(t *testing.T) {
	for _, channel := range []profileCycleChannelConfig{
		{Id: 1, Enabled: 0, Mode: "batch", Status: 1, Direction: "up"},
		{Id: 1, Enabled: 1, Mode: "batch", Status: 2, Direction: "up"},
		{Id: 1, Enabled: 1, Mode: "time", Status: 1, Direction: "down"},
		{Id: 0, Enabled: 1, Mode: "time", Status: 1, Direction: "up"},
	} {
		config := applyProfileCycleOverride(channel, profileCycleOverridePolicy{Enabled: true, Days: 2, Version: 4})
		if profileCycleChannelUsable(config) {
			t.Fatalf("override bypassed channel eligibility: %+v", config)
		}
	}
}

func TestProfileCycleOverrideUsesLastSuccessWithoutSpread(t *testing.T) {
	sentAt := gtime.New(time.Date(2026, time.November, 1, 22, 41, 17, 0, time.FixedZone("UTC+8", 8*3600)))
	channel := profileCycleChannelConfig{Mode: "batch", Days: 15, PublishTime: "09:30"}
	config := applyProfileCycleOverride(channel, profileCycleOverridePolicy{Enabled: true, Days: 3})
	expected := sentAt.AddDate(0, 0, 3)
	// Different IDs must not introduce either the channel clock or its spread.
	for _, profileID := range []int64{1, 29, 987654} {
		actual := calculateScheduledProfileCycleAt(config.Days, config.PublishTime, sentAt, profileID, 19, false)
		if actual.Timestamp() != expected.Timestamp() {
			t.Fatalf("profile %d next=%s expected last success + 3 days=%s", profileID, actual, expected)
		}
	}
	// Increasing days must move eligibility past a previously due schedule.
	longer := applyProfileCycleOverride(channel, profileCycleOverridePolicy{Enabled: true, Days: 20, Version: 2})
	actual := calculateScheduledProfileCycleAt(longer.Days, longer.PublishTime, sentAt, 29, 19, false)
	if actual.Timestamp() != sentAt.AddDate(0, 0, 20).Timestamp() {
		t.Fatalf("new generation retained an earlier interval: %s", actual)
	}
}

func TestProfileCycleRepairCheckpointRetainsLaterPages(t *testing.T) {
	checkpoint := profileCycleRepairCheckpoint{Channel: -55, Version: 8, Cursor: 200}
	checkpoint.selectTarget(-55, 8)
	if checkpoint.Cursor != 200 {
		t.Fatal("retry of the same generation lost its later-page cursor")
	}
	checkpoint.ProfileVersion = 11
	checkpoint.selectProfileVersion(11)
	if checkpoint.Cursor != 200 {
		t.Fatal("same profile version lost committed mapping progress")
	}
	checkpoint.selectProfileVersion(12)
	if checkpoint.Cursor != 0 {
		t.Fatal("new state version must restart even before its Outbox callback changes the dirty token")
	}
	checkpoint.Cursor = 200
	checkpoint.selectTarget(-55, 9)
	if checkpoint.Cursor != 0 || checkpoint.Channel != -55 || checkpoint.Version != 9 {
		t.Fatalf("configuration update must restart the profile from its first page: %+v", checkpoint)
	}
	checkpoint.Cursor = 21
	checkpoint.selectTarget(55, 9)
	if checkpoint.Cursor != 0 || checkpoint.Channel != 55 {
		t.Fatalf("channel and profile repair members collided: %+v", checkpoint)
	}
}

func TestProfileCycleProjectionAcknowledgesUnavailablePlans(t *testing.T) {
	now := gtime.New("2026-11-18 12:00:00")
	row := profileCycleRescheduleRow{RelationId: 1, JobId: 2, TenantId: 3, AccountId: 4, ProfileId: 5, ChannelId: 6}
	policy := profileCycleOverridePolicy{Enabled: true, Days: 2, Version: 17}
	job := telegramJobRecord{Id: 2, Status: JobStatusSent, SentAt: now.AddDate(0, 0, -5)}
	channel := profileCycleChannelConfig{Id: 6, TenantId: 3, Enabled: 1, Status: 1, Direction: "up", Mode: "batch", Version: 8}
	plan := calculateProfileCycleProjection(row, channel, policy, job, now, false)
	if plan.Enabled != 1 || plan.NextAt.Timestamp() != job.SentAt.AddDate(0, 0, 2).Timestamp() {
		t.Fatalf("overdue override must retain the exact past due time: %+v", plan)
	}
	for _, disabled := range []profileCycleChannelConfig{
		{Id: 6, TenantId: 3},
		{Id: 6, TenantId: 3, Enabled: 1, Status: 2, Direction: "up"},
		{Id: 6, TenantId: 3, Enabled: 1, Status: 1, Direction: "down"},
	} {
		plan = calculateProfileCycleProjection(row, disabled, policy, job, now, false)
		if plan.Enabled != 0 || plan.NextAt != nil || plan.Config.OverrideVersion != 17 || !plan.Config.OverrideEnabled {
			t.Fatalf("ineligible channel did not clear and acknowledge current policy: %+v", plan)
		}
	}
	for _, unavailable := range []telegramJobRecord{
		{},
		{Id: 2, Status: JobStatusPending, SentAt: job.SentAt},
		{Id: 2, Status: JobStatusSent},
	} {
		plan = calculateProfileCycleProjection(row, channel, policy, unavailable, now, false)
		if plan.Enabled != 0 || plan.NextAt != nil || plan.Config.OverrideVersion != 17 {
			t.Fatalf("missing success must still acknowledge version without a schedule: %+v", plan)
		}
	}
}

func TestProfileCycleProjectionBatchSQLBudget(t *testing.T) {
	for _, size := range []int{1, 20, 500} {
		plans := make([]profileCycleProjection, size)
		for i := range plans {
			plans[i].Row = profileCycleRescheduleRow{RelationId: int64(i + 1), JobId: int64(i + 1000)}
			plans[i].Config = profileCycleChannelConfig{OverrideVersion: 9, OverrideEnabled: true}
		}
		jobSQL, mappingSQL, acknowledgementSQL, args := profileCycleProjectionStatements(plans)
		if len(args) != size*13 || strings.Count(jobSQL, "?") != len(args) || strings.Count(mappingSQL, "?") != len(args) || strings.Count(acknowledgementSQL, "?") != len(args) {
			t.Fatalf("page %d lost typed batch parameters: args=%d", size, len(args))
		}
		if strings.Count(jobSQL, "UPDATE ") != 1 || strings.Count(mappingSQL, "UPDATE ") != 1 || strings.Count(acknowledgementSQL, "UPDATE ") != 1 {
			t.Fatalf("page %d must use three bounded writes including post-index acknowledgement", size)
		}
		if strings.Contains(mappingSQL, "SET applied_profile_cycle_version=") || strings.Contains(mappingSQL, ",applied_profile_cycle_version=") || !strings.Contains(acknowledgementSQL, "ps.cycle_override_version=v.profile_version") {
			t.Fatal("applied version must only advance after successful indexing and current-state CAS")
		}
		if !strings.Contains(jobSQL, "COALESCE(j.profile_cycle_version,0)<=v.profile_version") ||
			!strings.Contains(mappingSQL, "COALESCE(cp.applied_profile_cycle_version,0)<=v.profile_version") ||
			!strings.Contains(mappingSQL, "cycle_override_enabled=v.override_enabled") {
			t.Fatal("batch writes must protect version and override boolean together")
		}
	}
	// Guard the actual execution path: exactly three batch reads and two writes;
	// adding database work to a row loop fails even if the SQL builder stays flat.
	file, err := parser.ParseFile(token.NewFileSet(), "profile_cycle_override_policy.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var body *ast.BlockStmt
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "rebuildProfileCycleMappings" {
			body = fn.Body
		}
	}
	if body == nil {
		t.Fatal("batch execution function missing")
	}
	calls := 0
	isSQL := func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return false
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		switch selector.Sel.Name {
		case "Scan", "Exec", "GetAll", "GetOne", "Value", "Count":
			return true
		}
		return false
	}
	ast.Inspect(body, func(node ast.Node) bool {
		if isSQL(node) {
			calls++
		}
		var loop *ast.BlockStmt
		switch node := node.(type) {
		case *ast.RangeStmt:
			loop = node.Body
		case *ast.ForStmt:
			loop = node.Body
		}
		if loop != nil {
			ast.Inspect(loop, func(child ast.Node) bool {
				if isSQL(child) {
					t.Error("database call inside batch row loop")
				}
				return true
			})
		}
		return true
	})
	if calls != 6 {
		t.Fatalf("business SQL calls per page=%d, expected 6 including post-index CAS", calls)
	}
}

func TestProfileCycleOverrideVipExpiryUsesFreeInterval(t *testing.T) {
	if days := profileCycleDaysForPermission(2, true, 15); days != 2 {
		t.Fatalf("VIP override days=%d", days)
	}
	freeDays := profileCycleDaysForPermission(2, false, 15)
	if freeDays != 15 {
		t.Fatalf("expired VIP kept privileged interval: %d", freeDays)
	}
	now := gtime.New("2026-11-18 12:00:00")
	row := profileCycleDueRow{ProfileId: 5, ChannelId: 6, SentAt: now.AddDate(0, 0, -3), ProfileCycleVersion: 7}
	config := profileCycleChannelConfig{Id: 6, TenantId: 3, Enabled: 1, Status: 1, Direction: "up", Mode: "batch"}
	config = applyProfileCycleOverride(config, profileCycleOverridePolicy{Enabled: true, Days: freeDays, Version: 7})
	if !profileCycleDueNeedsRebuild(row, config, now, false) {
		t.Fatal("expired VIP's old two-day plan dispatched before free interval")
	}
}

func TestProfileCycleRebuildIndexErrorsAreRetained(t *testing.T) {
	var failure error
	ctx := context.WithValue(context.Background(), profileCycleIndexErrorKey{}, &failure)
	redisFailure := errors.New("isolated Redis write failure")
	wakeFailure := errors.New("isolated account wake failure")
	recordProfileCycleIndexError(ctx, redisFailure)
	recordProfileCycleIndexError(ctx, wakeFailure)
	if !errors.Is(failure, redisFailure) || !errors.Is(failure, wakeFailure) {
		t.Fatal("strict index context swallowed a Redis or wake error")
	}
	// Existing best-effort callers do not need the strict context.
	recordProfileCycleIndexError(context.Background(), redisFailure)
}
