package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"

	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/hgrds/lock"
)

const (
	telegramAntiScanHashHistoryLimit = 12
	telegramAntiScanHashHistoryTTL   = 90 * 24 * time.Hour
)

var telegramAntiScanHistoryLocker = lock.NewConfig(5*time.Second, 50*time.Millisecond)

type telegramAntiScanHash struct {
	PHash uint64 `json:"pHash"`
	DHash uint64 `json:"dHash"`
}

type telegramAntiScanHashHistory struct {
	Items []telegramAntiScanHash `json:"items"`
}

func telegramAntiScanHistoryKey(media *telegramMediaItem, kind string) string {
	if media == nil || media.Id <= 0 {
		return ""
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "image"
	}
	return fmt.Sprintf("%s%d:%s", publishconsts.AntiScanHashHistoryCacheKeyPrefix, media.Id, kind)
}

func loadTelegramAntiScanHashHistory(ctx context.Context, key string) []telegramAntiScanHash {
	if key == "" || !cache.Initialized() {
		return nil
	}
	value, err := cache.Instance().Get(ctx, key)
	if err != nil || value.IsNil() {
		if err != nil {
			g.Log().Warningf(ctx, "读取防扫图历史Hash失败 key:%s err:%+v", key, err)
		}
		return nil
	}
	var history telegramAntiScanHashHistory
	if err = value.Scan(&history); err != nil {
		g.Log().Warningf(ctx, "解析防扫图历史Hash失败 key:%s err:%+v", key, err)
		return nil
	}
	return history.Items
}

func appendTelegramAntiScanHashHistory(ctx context.Context, key string, item telegramAntiScanHash) {
	if key == "" || !cache.Initialized() || item.PHash == 0 && item.DHash == 0 {
		return
	}
	write := func() {
		history := loadTelegramAntiScanHashHistory(ctx, key)
		items := mergeTelegramAntiScanHashHistory(history, item, telegramAntiScanHashHistoryLimit)
		if err := cache.Instance().Set(ctx, key, telegramAntiScanHashHistory{Items: items}, telegramAntiScanHashHistoryTTL); err != nil {
			g.Log().Warningf(ctx, "保存防扫图历史Hash失败 key:%s err:%+v", key, err)
		}
	}
	if g.Cfg().MustGet(ctx, "cache.adapter").String() != "redis" {
		write()
		return
	}
	mutex := telegramAntiScanHistoryLocker.Mutex(publishconsts.AntiScanHashHistoryLockKeyPrefix + key)
	lockCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := mutex.Lock(lockCtx); err != nil {
		g.Log().Warningf(ctx, "获取防扫图历史Hash锁失败，跳过缓存回写 key:%s err:%+v", key, err)
		return
	}
	defer func() { _ = mutex.Unlock(context.Background()) }()
	write()
}

func mergeTelegramAntiScanHashHistory(history []telegramAntiScanHash, item telegramAntiScanHash, limit int) []telegramAntiScanHash {
	if limit <= 0 {
		return nil
	}
	items := make([]telegramAntiScanHash, 0, limit)
	items = append(items, item)
	for _, existing := range history {
		if existing == item {
			continue
		}
		items = append(items, existing)
		if len(items) >= limit {
			break
		}
	}
	return items
}
