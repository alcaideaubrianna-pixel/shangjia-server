package sys

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_feiniu_sync/model/input/sysin"
)

const (
	sourceIndexReadyTTL      = 24 * time.Hour
	sourceIndexPermissionTTL = 24 * time.Hour
	sourceIndexFailureRetry  = 30 * time.Minute
)

type sourceIndexState struct {
	status    string
	expiresAt time.Time
}

var sourceIndexStates = struct {
	sync.RWMutex
	items map[string]sourceIndexState
}{items: make(map[string]sourceIndexState)}

func (s *sSysSync) ensureSourceIndexes(ctx context.Context, db gdb.DB, dbType, sourceKey string) {
	if sourceKey == "" {
		return
	}
	if _, ok := sourceIndexStateGet(sourceKey); ok {
		return
	}
	sourceIndexStateSet(sourceKey, sourceIndexState{status: "running", expiresAt: time.Now().Add(sourceIndexFailureRetry)})

	permissionDenied := false
	for _, statement := range sourceIndexStatements(dbType) {
		if _, err := db.Ctx(ctx).Exec(ctx, statement); err != nil {
			if isIgnorableSQLError(err) {
				continue
			}
			if isSourceIndexPermissionError(err) {
				permissionDenied = true
				continue
			}
			sourceIndexStateSet(sourceKey, sourceIndexState{status: "retry", expiresAt: time.Now().Add(sourceIndexFailureRetry)})
			g.Log().Warningf(ctx, "创建 FeiNiu 源库索引失败，已跳过 sql:%s err:%+v", statement, err)
			return
		}
	}

	if permissionDenied {
		sourceIndexStateSet(sourceKey, sourceIndexState{status: "permission_denied", expiresAt: time.Now().Add(sourceIndexPermissionTTL)})
		g.Log().Warningf(ctx, "FeiNiu 源库账号没有建索引权限，已跳过源库索引创建；同步仍会继续 source:%s", sourceKey)
		return
	}
	sourceIndexStateSet(sourceKey, sourceIndexState{status: "ready", expiresAt: time.Now().Add(sourceIndexReadyTTL)})
}

func sourceIndexStatements(dbType string) []string {
	statements := []string{
		"CREATE INDEX idx_yfs_src_note_status_id ON tg_content_note (status, note_id)",
		"CREATE INDEX idx_yfs_src_source_note_id ON tg_content_source (note_id)",
		"CREATE INDEX idx_yfs_src_channel_id ON tg_channel (channel_id)",
	}
	if normalizeDBType(dbType) != "pgsql" {
		return statements
	}
	return []string{
		"CREATE INDEX IF NOT EXISTS idx_yfs_src_note_status_id ON tg_content_note (status, note_id)",
		"CREATE INDEX IF NOT EXISTS idx_yfs_src_source_note_id ON tg_content_source (note_id)",
		"CREATE INDEX IF NOT EXISTS idx_yfs_src_channel_id ON tg_channel (channel_id)",
	}
}

func sourceIndexKey(in *sysin.ConfigSaveInp) string {
	if in == nil {
		return ""
	}
	return strings.Join([]string{
		normalizeDBType(in.DbType), strings.TrimSpace(in.DbHost),
		strings.TrimSpace(in.DbName), strings.TrimSpace(in.DbUser),
		strconv.Itoa(in.DbPort),
	}, "|")
}

func sourceIndexStateGet(key string) (sourceIndexState, bool) {
	now := time.Now()
	sourceIndexStates.RLock()
	state, ok := sourceIndexStates.items[key]
	sourceIndexStates.RUnlock()
	if !ok || now.After(state.expiresAt) {
		if ok {
			sourceIndexStates.Lock()
			delete(sourceIndexStates.items, key)
			sourceIndexStates.Unlock()
		}
		return sourceIndexState{}, false
	}
	return state, true
}

func sourceIndexStateSet(key string, state sourceIndexState) {
	sourceIndexStates.Lock()
	sourceIndexStates.items[key] = state
	sourceIndexStates.Unlock()
}

func isSourceIndexPermissionError(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, phrase := range []string{
		"index command denied", "command denied", "permission denied",
		"access denied", "not enough privileges", "must be owner",
	} {
		if strings.Contains(msg, phrase) {
			return true
		}
	}
	return false
}
