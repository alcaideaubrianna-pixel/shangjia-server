package sys

import (
	"context"
	"sort"
	"strings"

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
	Roles            []string
	Account          bool
	Scheduler        bool
	PushWorker       bool
	MediaWorker      bool
	BackgroundWorker bool
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
		case runrole.Runtime:
			values = append(values, publishRuntimeRoleAccount, publishRuntimeRoleScheduler)
		}
	}
	return parsePublishRuntimeRoles(values)
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
	case publishRuntimeRoleWorker:
		return config.PushWorker && config.MediaWorker && config.BackgroundWorker
	case publishRuntimeRoleWeb:
		return !config.Account && !config.Scheduler && !config.PushWorker && !config.MediaWorker && !config.BackgroundWorker
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
		case publishRuntimeRoleWeb:
		case publishRuntimeRoleAccount:
			config.Account = true
		case publishRuntimeRoleScheduler:
			config.Scheduler = true
		case publishRuntimeRoleWorker:
			config.PushWorker = true
			config.MediaWorker = true
			config.BackgroundWorker = true
		case publishRuntimeRolePushWorker:
			config.PushWorker = true
		case publishRuntimeRoleMediaWorker:
			config.MediaWorker = true
		case publishRuntimeRoleBackgroundWorker:
			config.BackgroundWorker = true
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
		publishRuntimeRoleBackgroundWorker:
		return true
	default:
		return false
	}
}
