// Package profilecycle 提供资料级循环批次规划能力。
package profilecycle

import (
	"sort"
	"time"
)

// Channel 描述资料当前可用的频道循环配置。
type Channel struct {
	ID          int64
	Enabled     bool
	Status      int
	Direction   string
	Mode        string
	Interval    time.Duration
	PublishTime string
}

// Profile 描述需要计算循环批次的资料及其频道配置。
type Profile struct {
	ID        int64
	TenantID  int64
	AccountID int64
	Channels  []Channel
	LastSent  time.Time
}

// ChannelTask 是同一资料循环批次中的频道子任务。
type ChannelTask struct {
	ProfileID int64
	ChannelID int64
	DueAt     time.Time
}

// RunPlan 是资料级循环批次计划；同一资料的频道任务共享 RunID。
type RunPlan struct {
	ProfileID int64
	TenantID  int64
	AccountID int64
	RunID     string
	DueAt     time.Time
	Tasks     []ChannelTask
}

// BuildPlan 为一个资料生成一轮同步循环计划。
// 无效频道会被排除；没有有效频道时返回 nil，避免创建空批次。
func BuildPlan(profile Profile, runID string, now time.Time) *RunPlan {
	if profile.ID <= 0 || runID == "" {
		return nil
	}
	channels := make([]Channel, 0, len(profile.Channels))
	for _, channel := range profile.Channels {
		if channel.ID <= 0 || !channel.Enabled || channel.Status != 1 || channel.Direction != "up" || channel.Mode == "batch" || channel.Interval <= 0 {
			continue
		}
		channels = append(channels, channel)
	}
	if len(channels) == 0 {
		return nil
	}
	sort.Slice(channels, func(i, j int) bool { return channels[i].ID < channels[j].ID })
	dueAt := profile.LastSent.Add(channels[0].Interval)
	if dueAt.Before(now) {
		dueAt = now
	}
	plan := &RunPlan{ProfileID: profile.ID, TenantID: profile.TenantID, AccountID: profile.AccountID, RunID: runID, DueAt: dueAt, Tasks: make([]ChannelTask, 0, len(channels))}
	for _, channel := range channels {
		plan.Tasks = append(plan.Tasks, ChannelTask{ProfileID: profile.ID, ChannelID: channel.ID, DueAt: dueAt})
	}
	return plan
}
