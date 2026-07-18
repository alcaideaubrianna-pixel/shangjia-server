package sys

import (
	"context"
	"fmt"
	"hash/crc32"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/lazysheep_tggo/model"
	lsysin "hotgo/addons/lazysheep_tggo/model/input/sysin"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/hgrds/lock"
)

var autoPullLoopOnce sync.Once

const (
	autoPullTopic             = "lazysheep_tggo:auto_pull"
	autoPullScheduleLockKey   = "lazysheep_tggo:auto_pull:schedule:"
	autoPullBindingsCacheKey  = "lazysheep_tggo:auto_pull:bindings"
	autoPullBindingsCacheTTL  = 30 * time.Second
	autoPullScheduleInterval  = time.Second
	autoPullDispatchDelayBase = 3 * time.Second
	autoPullRunLockTTL        = 5 * time.Minute
	autoPullAuthCooldownTTL   = 30 * time.Minute
	autoPullIdleDisableAfter  = 30 * time.Minute
	autoPullActivityTTL       = time.Hour
)

func (s *sLazySheepTGGo) StartAutoPullLoop(ctx context.Context) {
	autoPullLoopOnce.Do(func() {
		go s.runAutoPullLoop(ctx)
	})
}

func (s *sLazySheepTGGo) runAutoPullLoop(ctx context.Context) {
	ticker := time.NewTicker(autoPullScheduleInterval)
	defer ticker.Stop()
	g.Log().Info(ctx, "懒羊羊TGGo自动拉取调度器已启动")
	for {
		select {
		case <-ctx.Done():
			g.Log().Info(ctx, "懒羊羊TGGo自动拉取调度器已停止")
			return
		case <-ticker.C:
			s.dispatchAutoPullSlot(ctx, time.Now())
		}
	}
}

func (s *sLazySheepTGGo) dispatchAutoPullSlot(ctx context.Context, now time.Time) {
	slot := now.Second()
	mutex := lock.NewConfig(2*time.Second, 100*time.Millisecond).Mutex(autoPullScheduleLockKey + fmt.Sprintf("%d", slot))
	if err := mutex.TryLock(ctx); err != nil {
		if !gerror.Is(err, lock.ErrLockFailed) {
			g.Log().Warningf(ctx, "自动拉取调度加锁失败 err:%+v", err)
		}
		return
	}
	defer func() {
		if err := mutex.Unlock(ctx); err != nil && !gerror.Is(err, lock.ErrNotExist) {
			g.Log().Warningf(ctx, "自动拉取调度解锁失败 err:%+v", err)
		}
	}()
	bindings, err := s.autoPullBindings(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "加载自动拉取配置失败 err:%+v", err)
		return
	}
	dispatched := 0
	for _, binding := range bindings {
		if autoPullSlot(binding.Key) != slot {
			continue
		}
		if autoPullBindingCoolingDown(ctx, binding.BotKey, binding.Key) {
			continue
		}
		chatID := autoPullChatID(binding)
		if chatID == 0 {
			g.Log().Warningf(ctx, "自动拉取跳过，缺少目标会话 bot:%s binding:%s", binding.BotKey, binding.Key)
			continue
		}
		task := &lsysin.AutoPullTask{
			BotKey:     binding.BotKey,
			BindingKey: binding.Key,
			SourceURL:  binding.SourceURL,
			ChatID:     chatID,
			Slot:       slot,
			QueuedAt:   now.Format(time.RFC3339),
		}
		if err := delayOrPushQueue(ctx, autoPullTopic, task, int64(autoPullDispatchDelayBase.Seconds())); err != nil {
			g.Log().Warningf(ctx, "自动拉取任务入队失败 bot:%s binding:%s err:%+v", binding.BotKey, binding.Key, err)
			continue
		}
		dispatched++
	}
	if dispatched > 0 {
		g.Log().Debugf(ctx, "自动拉取调度完成 slot:%d dispatched:%d", slot, dispatched)
	}
}

func (s *sLazySheepTGGo) autoPullBindings(ctx context.Context) ([]*model.BindingRecord, error) {
	val, err := cache.Instance().GetOrSetFuncLock(ctx, autoPullBindingsCacheKey, func(ctx context.Context) (interface{}, error) {
		state, err := s.GetState(ctx)
		if err != nil {
			return nil, err
		}
		bindings := make([]*model.BindingRecord, 0)
		for _, binding := range state.Bindings {
			if !bindingAutoPullEnabled(binding) {
				continue
			}
			bindings = append(bindings, binding)
		}
		return bindings, nil
	}, autoPullBindingsCacheTTL)
	if err != nil {
		return nil, err
	}
	var bindings []*model.BindingRecord
	if err := val.Scan(&bindings); err != nil {
		return nil, err
	}
	if bindings == nil {
		bindings = []*model.BindingRecord{}
	}
	return bindings, nil
}

func clearAutoPullBindingsCache(ctx context.Context) {
	if _, err := cache.Instance().Remove(ctx, autoPullBindingsCacheKey); err != nil {
		g.Log().Warningf(ctx, "清理自动拉取配置缓存失败 err:%+v", err)
	}
}

func (s *sLazySheepTGGo) HandleAutoPullTask(ctx context.Context, task *lsysin.AutoPullTask) error {
	if task == nil || task.BotKey == "" || task.BindingKey == "" {
		return nil
	}
	mutex := lock.NewConfig(autoPullRunLockTTL, time.Second).Mutex(autoPullRunLockKey(task.BotKey, task.BindingKey))
	if err := mutex.TryLock(ctx); err != nil {
		if gerror.Is(err, lock.ErrLockFailed) {
			g.Log().Debugf(ctx, "自动拉取任务跳过，已有执行中 bot:%s binding:%s", task.BotKey, task.BindingKey)
			return nil
		}
		return err
	}
	defer func() {
		if err := mutex.Unlock(ctx); err != nil && !gerror.Is(err, lock.ErrNotExist) {
			g.Log().Warningf(ctx, "自动拉取执行解锁失败 bot:%s binding:%s err:%+v", task.BotKey, task.BindingKey, err)
		}
	}()
	taskCtx := withPullTrace(ctx, shortHash(fmt.Sprintf("auto:%s:%s:%d", task.BotKey, task.BindingKey, time.Now().UnixNano())))
	_, err := s.PullNow(taskCtx, &lsysin.PullInp{
		BotKey:    task.BotKey,
		SourceURL: task.SourceURL,
		ChatID:    task.ChatID,
		Limit:     0,
		Auto:      true,
	})
	if err != nil && isBangchatAuthExpiredError(err) {
		markAutoPullAuthCooldown(ctx, task, err)
		return nil
	}
	return err
}

func bindingAutoPullEnabled(binding *model.BindingRecord) bool {
	if binding == nil || binding.Status != "enabled" {
		return false
	}
	return pluginStateBoolValue(binding.PluginState, collectorAutoPullStateKey, true)
}

func autoPullSlot(bindingKey string) int {
	if bindingKey == "" {
		return 0
	}
	return int(parseUintHash(bindingKey) % 60)
}

func parseUintHash(text string) uint32 {
	return crc32.ChecksumIEEE([]byte(text))
}

func autoPullRunLockKey(botKey, bindingKey string) string {
	return "lazysheep_tggo:auto_pull:run:" + shortHash(botKey+":"+bindingKey)
}

func autoPullAuthCooldownKey(botKey, bindingKey string) string {
	return "lazysheep_tggo:auto_pull:auth_cooldown:" + shortHash(botKey+":"+bindingKey)
}

func autoPullActivityKey(botKey, bindingKey string) string {
	return "lazysheep_tggo:auto_pull:last_new:" + shortHash(botKey+":"+bindingKey)
}

func autoPullBindingCoolingDown(ctx context.Context, botKey, bindingKey string) bool {
	val, err := cache.Instance().Get(ctx, autoPullAuthCooldownKey(botKey, bindingKey))
	return err == nil && !val.IsNil() && val.String() != ""
}

func markAutoPullAuthCooldown(ctx context.Context, task *lsysin.AutoPullTask, err error) {
	if task == nil || err == nil {
		return
	}
	key := autoPullAuthCooldownKey(task.BotKey, task.BindingKey)
	if autoPullBindingCoolingDown(ctx, task.BotKey, task.BindingKey) {
		return
	}
	_ = cache.Instance().Set(ctx, key, err.Error(), autoPullAuthCooldownTTL)
	g.Log().Warningf(ctx, "自动拉取暂停，BangChat登录态失效 bot:%s binding:%s cooldown:%s err:%+v", task.BotKey, task.BindingKey, autoPullAuthCooldownTTL, err)
}

func isBangchatAuthExpiredError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "user no login") ||
		strings.Contains(text, "not login") ||
		strings.Contains(text, "unauthorized") ||
		strings.Contains(text, "401")
}

func (s *sLazySheepTGGo) updateAutoPullIdleState(ctx context.Context, binding *model.BindingRecord, hasNew bool) {
	if binding == nil || strings.TrimSpace(binding.BotKey) == "" || strings.TrimSpace(binding.Key) == "" {
		return
	}
	key := autoPullActivityKey(binding.BotKey, binding.Key)
	now := time.Now()
	if hasNew {
		_ = cache.Instance().Set(ctx, key, now.Unix(), autoPullActivityTTL)
		return
	}
	val, err := cache.Instance().Get(ctx, key)
	if err != nil || val.IsNil() || val.Int64() <= 0 {
		_ = cache.Instance().Set(ctx, key, now.Unix(), autoPullActivityTTL)
		return
	}
	lastNewAt := time.Unix(val.Int64(), 0)
	if now.Sub(lastNewAt) < autoPullIdleDisableAfter {
		return
	}
	if err = s.disableBindingAutoPull(ctx, binding.Key, fmt.Sprintf("连续 %s 没有新笔记", autoPullIdleDisableAfter)); err != nil {
		g.Log().Warningf(ctx, "自动关闭空闲拉取失败 bot:%s binding:%s err:%+v", binding.BotKey, binding.Key, err)
		return
	}
	_, _ = cache.Instance().Remove(ctx, key)
}

func (s *sLazySheepTGGo) disableBindingAutoPull(ctx context.Context, bindingKey string, reason string) error {
	state, err := s.GetState(ctx)
	if err != nil {
		return err
	}
	binding := state.Bindings[bindingKey]
	if binding == nil {
		return nil
	}
	if binding.PluginState == nil {
		binding.PluginState = map[string]any{}
	}
	if !pluginStateBoolValue(binding.PluginState, collectorAutoPullStateKey, true) {
		return nil
	}
	binding.PluginState[collectorAutoPullStateKey] = false
	binding.PluginState[collectorAutoPullStoppedAtKey] = time.Now().Format("2006-01-02 15:04:05")
	binding.PluginState[collectorAutoPullStopReasonKey] = reason
	if err = s.saveState(ctx, state); err != nil {
		return err
	}
	clearAutoPullBindingsCache(ctx)
	g.Log().Warningf(ctx, "自动拉取已关闭 bot:%s binding:%s reason:%s", binding.BotKey, binding.Key, reason)
	return nil
}

func (s *sLazySheepTGGo) UpdateBindingAutoPull(ctx context.Context, in *lsysin.BindingAutoPullControlInp) error {
	if in == nil || strings.TrimSpace(in.BindingKey) == "" {
		return gerror.New("绑定不能为空")
	}
	bindingKey := strings.TrimSpace(in.BindingKey)
	state, err := s.GetState(ctx)
	if err != nil {
		return err
	}
	binding := state.Bindings[bindingKey]
	if binding == nil {
		return gerror.New("绑定不存在")
	}
	if binding.PluginState == nil {
		binding.PluginState = map[string]any{}
	}
	binding.PluginState[collectorAutoPullStateKey] = in.AutoPull
	if in.AutoPull {
		delete(binding.PluginState, collectorAutoPullStoppedAtKey)
		delete(binding.PluginState, collectorAutoPullStopReasonKey)
		_ = cache.Instance().Set(ctx, autoPullActivityKey(binding.BotKey, binding.Key), time.Now().Unix(), autoPullActivityTTL)
		_, _ = cache.Instance().Remove(ctx, autoPullAuthCooldownKey(binding.BotKey, binding.Key))
	} else {
		binding.PluginState[collectorAutoPullStoppedAtKey] = time.Now().Format("2006-01-02 15:04:05")
		binding.PluginState[collectorAutoPullStopReasonKey] = "后台手动关闭"
		_, _ = cache.Instance().Remove(ctx, autoPullActivityKey(binding.BotKey, binding.Key))
	}
	if err = s.saveState(ctx, state); err != nil {
		return err
	}
	clearAutoPullBindingsCache(ctx)
	return nil
}

func autoPullChatID(binding *model.BindingRecord) int64 {
	if binding == nil {
		return 0
	}
	if binding.AutoPush && binding.PublishChatID != 0 {
		return binding.PublishChatID
	}
	if binding.ReviewChatID != 0 {
		return binding.ReviewChatID
	}
	return binding.PublishChatID
}
