package runrole

import (
	"context"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/gogf/gf/v2/frame/g"
)

const (
	All     = "all"
	Web     = "web"
	Worker  = "worker"
	Runtime = "runtime"
)

var processRoles struct {
	sync.RWMutex
	values []string
}

func Set(values ...string) {
	processRoles.Lock()
	processRoles.values = normalize(values)
	processRoles.Unlock()
}

func Roles(ctx context.Context) []string {
	processRoles.RLock()
	values := append([]string(nil), processRoles.values...)
	processRoles.RUnlock()
	if len(values) > 0 {
		return values
	}
	if value, ok := os.LookupEnv("YOUBAN_RUNTIME_ROLES"); ok {
		return normalize([]string{value})
	}
	if value, ok := os.LookupEnv("YOUBAN_PUBLISH_RUNTIME_ROLES"); ok {
		return normalizeLegacy([]string{value})
	}
	value, err := g.Cfg().Get(ctx, "runtime.roles")
	if err == nil && value != nil && !value.IsNil() {
		if roles := normalize(value.Strings()); len(roles) > 0 {
			return roles
		}
	}
	return []string{All}
}

func Enabled(ctx context.Context, role string) bool {
	role = canonical(role)
	for _, current := range Roles(ctx) {
		if current == All || current == role {
			return true
		}
	}
	return false
}

func normalize(values []string) []string {
	set := make(map[string]struct{})
	for _, value := range values {
		for _, item := range strings.FieldsFunc(strings.ToLower(strings.TrimSpace(value)), func(r rune) bool {
			return r == ',' || r == ';' || r == '|'
		}) {
			if role := canonical(item); role != "" {
				set[role] = struct{}{}
			}
		}
	}
	if _, ok := set[All]; ok {
		return []string{All}
	}
	roles := make([]string, 0, len(set))
	for role := range set {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	return roles
}

func normalizeLegacy(values []string) []string {
	mapped := make([]string, 0, len(values))
	for _, value := range values {
		for _, item := range strings.FieldsFunc(strings.ToLower(strings.TrimSpace(value)), func(r rune) bool {
			return r == ',' || r == ';' || r == '|'
		}) {
			switch strings.TrimSpace(item) {
			case All, Web, Worker:
				mapped = append(mapped, item)
			case "account", "scheduler":
				mapped = append(mapped, Runtime)
			case "push-worker", "media-worker", "background-worker":
				mapped = append(mapped, Worker)
			}
		}
	}
	return normalize(mapped)
}

func canonical(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case All:
		return All
	case Web:
		return Web
	case Worker, "push-worker", "media-worker", "background-worker":
		return Worker
	case Runtime, "account", "scheduler", "bot":
		return Runtime
	default:
		return ""
	}
}
