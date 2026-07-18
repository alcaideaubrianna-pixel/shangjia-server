package sys

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/lazysheep_tggo/model"
)

var runningPullCancels sync.Map

func registerRunningPull(ctx context.Context, botKey string, binding *model.BindingRecord) (context.Context, func()) {
	if binding == nil {
		return ctx, func() {}
	}
	runCtx, cancel := context.WithCancel(ctx)
	key := runningPullKey(botKey, binding.Key)
	runningPullCancels.Store(key, cancel)
	return runCtx, func() {
		runningPullCancels.Delete(key)
		cancel()
	}
}

func cancelRunningPull(botKey string, binding *model.BindingRecord) bool {
	if binding == nil {
		return false
	}
	raw, ok := runningPullCancels.Load(runningPullKey(botKey, binding.Key))
	if !ok {
		return false
	}
	if cancel, ok := raw.(context.CancelFunc); ok {
		cancel()
	}
	return true
}

func runningPullKey(botKey, bindingKey string) string {
	return strings.TrimSpace(botKey) + ":" + strings.TrimSpace(bindingKey)
}

func (s *sLazySheepTGGo) PauseBindingWork(ctx context.Context, botKey string, chatID int64) (string, error) {
	binding, err := s.findBinding(ctx, botKey, "", chatID)
	if err != nil {
		return "", err
	}
	if binding == nil {
		return "", gerror.New("当前频道还没有绑定关系")
	}
	runningCanceled := cancelRunningPull(botKey, binding)
	if err = s.pauseBindingAutoPull(ctx, binding.Key, "用户命令暂停"); err != nil {
		return "", err
	}
	stoppedTasks, err := s.stopPushTasksForBinding(ctx, botKey, binding, chatID, "用户命令暂停")
	if err != nil {
		return "", err
	}
	parts := []string{"当前频道采集已暂停，可以修改配置后重新拉取。"}
	if runningCanceled {
		parts = append(parts, "已取消正在执行的采集任务。")
	}
	parts = append(parts, fmt.Sprintf("已停止 %d 个待推送任务。", stoppedTasks))
	return strings.Join(parts, "\n"), nil
}

func (s *sLazySheepTGGo) pauseBindingAutoPull(ctx context.Context, bindingKey string, reason string) error {
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
	binding.PluginState[collectorAutoPullStateKey] = false
	binding.PluginState[collectorAutoPullStoppedAtKey] = gtime.Now().Format("Y-m-d H:i:s")
	binding.PluginState[collectorAutoPullStopReasonKey] = reason
	if err = s.saveState(ctx, state); err != nil {
		return err
	}
	clearAutoPullBindingsCache(ctx)
	return nil
}

func (s *sLazySheepTGGo) stopPushTasksForBinding(ctx context.Context, botKey string, binding *model.BindingRecord, chatID int64, reason string) (int64, error) {
	if binding == nil {
		return 0, nil
	}
	if err := s.ensurePushQueueTable(ctx); err != nil {
		return 0, err
	}
	targetChatID := pushTargetChatID(binding, chatID)
	mod := g.DB().Model("hg_addon_lazysheep_tggo_push_queue").
		Where("bot_key", botKey).
		Where("binding_key", binding.Key).
		WhereIn("status", g.Slice{pushTaskStatusReady, pushTaskStatusDoing, pushTaskStatusRetry})
	if targetChatID != 0 {
		mod = mod.Where("chat_id", targetChatID)
	}
	result, err := mod.Data(g.Map{
		"status":      pushTaskStatusDead,
		"last_error":  strings.TrimSpace(reason),
		"finished_at": gtime.Now(),
		"updated_at":  gtime.Now(),
	}).Update()
	if err != nil {
		return 0, gerror.Wrap(err, "停止频道推送任务失败")
	}
	return result.RowsAffected()
}
