package sys

import (
	"context"
	"os"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"hotgo/utility/runrole"
)

const (
	publishRuntimeRoleAll              = "all"
	publishRuntimeRoleWeb              = "web"
	publishRuntimeRoleAccount          = "account"
	publishRuntimeRoleScheduler        = "scheduler"
	publishRuntimeRoleWorker           = "worker"
	publishRuntimeRolePushWorker       = "push-worker"
	publishRuntimeRoleMediaWorker      = "media-worker"
	publishRuntimeRoleBackgroundWorker = "background-worker"
)

type publishRuntimeConfig struct {
	Roles                []string
	Account              bool
	Scheduler            bool
	PushWorker           bool
	MediaWorker          bool
	BackgroundWorker     bool
	CollectorWorker      bool
	CollectProcessWorker bool
	BackgroundQueue      bool
	// RedisStreamGray 仅允许在 Push Worker 上开启 Redis Stream 灰度消费者。
	RedisStreamGray bool
}

func loadPublishRuntimeConfig(ctx context.Context) publishRuntimeConfig {
	roles := runrole.Roles(ctx)
	values := make([]string, 0, len(roles)*2)
	for _, role := range roles {
		switch role {
		case runrole.All:
			return parsePublishRuntimeRoles([]string{publishRuntimeRoleAll})
		case runrole.Web:
			values = append(values, publishRuntimeRoleWeb)
		case runrole.Worker:
			values = append(values, publishRuntimeRoleWorker)
		case runrole.PushWorker:
			values = append(values, publishRuntimeRolePushWorker)
		case runrole.MediaWorker:
			values = append(values, publishRuntimeRoleMediaWorker)
		case runrole.BackgroundWorker:
			values = append(values, publishRuntimeRoleBackgroundWorker)
		case runrole.Account:
			values = append(values, publishRuntimeRoleAccount)
		case runrole.CollectorWorker:
			// 采集 Worker 负责消费采集分组、媒体窗口和发布前处理队列。
			// Telegram Collector 插件与本插件使用同一运行角色，必须同时启用后台队列。
			values = append(values, runrole.CollectorWorker)
		case runrole.CollectProcessWorker:
			values = append(values, runrole.CollectProcessWorker)
		case runrole.Scheduler:
			values = append(values, publishRuntimeRoleScheduler)
		case runrole.Runtime:
			values = append(values, publishRuntimeRoleAccount, publishRuntimeRoleScheduler)
		}
	}
	config := parsePublishRuntimeRoles(values)
	// 灰度开关默认关闭，且不能在非 Push Worker 角色启动，避免调度器重复消费。
	// The channel ZSET transport is the sole TG publish transport.  The
	// historical Redis Stream gray bridge dispatches back into Asynq and must
	// never run alongside it, otherwise the same Outbox job can have two
	// consumers.
	config.RedisStreamGray = config.PushWorker && !telegramChannelQueueEnabled() && (g.Cfg().MustGet(ctx, "youbanPublish.redisStream.grayEnabled", false).Bool() || strings.EqualFold(strings.TrimSpace(os.Getenv("YOUBAN_PUBLISH_PUSH_ENGINE")), "unified_stream"))
	return config
}

func (s *sSysPublish) RuntimeRoleEnabled(ctx context.Context, role string) bool {
	config := loadPublishRuntimeConfig(ctx)
	switch strings.ToLower(strings.TrimSpace(role)) {
	case publishRuntimeRoleAccount:
		return config.Account
	case publishRuntimeRoleScheduler:
		return config.Scheduler
	case publishRuntimeRolePushWorker:
		return config.PushWorker
	case publishRuntimeRoleMediaWorker:
		return config.MediaWorker
	case publishRuntimeRoleBackgroundWorker:
		return config.BackgroundWorker
	case runrole.CollectorWorker:
		return config.CollectorWorker
	case runrole.CollectProcessWorker:
		return config.CollectProcessWorker
	case publishRuntimeRoleWorker:
		return config.PushWorker && config.MediaWorker && config.BackgroundWorker
	case publishRuntimeRoleWeb:
		return !config.Account && !config.Scheduler && !config.PushWorker && !config.MediaWorker && !config.BackgroundWorker && !config.CollectorWorker && !config.CollectProcessWorker
	case publishRuntimeRoleAll:
		return config.Account && config.Scheduler && config.PushWorker && config.MediaWorker && config.BackgroundWorker
	default:
		return false
	}
}

func parsePublishRuntimeRoles(values []string) publishRuntimeConfig {
	roles := normalizePublishRuntimeRoles(values)
	config := publishRuntimeConfig{Roles: roles}
	for _, role := range roles {
		switch role {
		case publishRuntimeRoleAll:
			config.Account = true
			config.Scheduler = true
			config.PushWorker = true
			config.MediaWorker = true
			config.BackgroundWorker = true
			config.CollectorWorker = true
			config.BackgroundQueue = true
		case publishRuntimeRoleWeb:
		case publishRuntimeRoleAccount:
			config.Account = true
		case publishRuntimeRoleScheduler:
			config.Scheduler = true
		case publishRuntimeRoleWorker:
			config.PushWorker = true
			config.MediaWorker = true
			config.BackgroundWorker = true
			config.BackgroundQueue = true
		case publishRuntimeRolePushWorker:
			config.PushWorker = true
		case publishRuntimeRoleMediaWorker:
			config.MediaWorker = true
		case publishRuntimeRoleBackgroundWorker:
			config.BackgroundWorker = true
			config.BackgroundQueue = true
		case runrole.CollectorWorker:
			config.CollectorWorker = true
		case runrole.CollectProcessWorker:
			config.CollectProcessWorker = true
		}
	}
	return config
}

func normalizePublishRuntimeRoles(values []string) []string {
	if len(values) == 0 {
		return []string{publishRuntimeRoleAll}
	}
	set := make(map[string]struct{})
	for _, value := range values {
		for _, role := range strings.FieldsFunc(strings.ToLower(strings.TrimSpace(value)), func(r rune) bool {
			return r == ',' || r == ';' || r == '|'
		}) {
			role = strings.TrimSpace(role)
			if isPublishRuntimeRole(role) {
				set[role] = struct{}{}
			}
		}
	}
	if _, ok := set[publishRuntimeRoleAll]; ok {
		return []string{publishRuntimeRoleAll}
	}
	roles := make([]string, 0, len(set))
	for role := range set {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	return roles
}

func isPublishRuntimeRole(role string) bool {
	switch role {
	case publishRuntimeRoleAll,
		publishRuntimeRoleWeb,
		publishRuntimeRoleAccount,
		publishRuntimeRoleScheduler,
		publishRuntimeRoleWorker,
		publishRuntimeRolePushWorker,
		publishRuntimeRoleMediaWorker,
		publishRuntimeRoleBackgroundWorker,
		runrole.CollectorWorker,
		runrole.CollectProcessWorker:
		return true
	default:
		return false
	}
}
