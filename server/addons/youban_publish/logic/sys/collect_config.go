package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

const publishConfigGroupCollect = "collect"
const publishConfigKeyCollectEnabled = "collectEnabled"
const publishConfigKeyCollectPushEnabled = "collectPushEnabled"
const publishConfigKeyCollectRealtimePushDelaySec = "realtimePushDelaySec"
const collectConfigCacheKey = "youban_publish:collect:config:v1"
const collectConfigCacheTTL = 15 * time.Second

func (s *sSysPublish) CollectConfig(ctx context.Context) (*sysin.CollectConfigModel, error) {
	if value, err := cache.Instance().Get(ctx, collectConfigCacheKey); err == nil && !value.IsNil() {
		config := defaultCollectConfig()
		if scanErr := value.Scan(config); scanErr == nil {
			return config, nil
		}
	}
	storage := defaultCollectConfigStorage()
	if err := NewSysConfig().scanConfigGroup(ctx, publishConfigGroupCollect, storage); err != nil {
		return nil, err
	}
	config := &sysin.CollectConfigModel{
		Enabled:              storage.CollectEnabled,
		PushEnabled:          storage.CollectPushEnabled,
		RealtimePushDelaySec: normalizeCollectRealtimePushDelaySec(storage.RealtimePushDelaySec),
	}
	_ = cache.Instance().Set(ctx, collectConfigCacheKey, config, collectConfigCacheTTL)
	return config, nil
}

func (s *sSysPublish) CollectConfigSave(ctx context.Context, in *sysin.CollectConfigSaveInp) error {
	if in == nil {
		in = &sysin.CollectConfigSaveInp{Enabled: 1}
	}
	if err := in.Filter(ctx); err != nil {
		return err
	}
	pushEnabled := 1
	if in.PushEnabled != nil {
		pushEnabled = *in.PushEnabled
	} else if current, currentErr := s.CollectConfig(ctx); currentErr == nil && current != nil {
		pushEnabled = current.PushEnabled
	}
	err := NewSysConfig().updateConfigGroup(ctx, publishConfigGroupCollect, g.Map{
		publishConfigKeyCollectEnabled:              in.Enabled,
		publishConfigKeyCollectPushEnabled:          pushEnabled,
		publishConfigKeyCollectRealtimePushDelaySec: in.RealtimePushDelaySec,
	})
	if err == nil {
		_, _ = cache.Instance().Remove(ctx, collectConfigCacheKey)
	}
	return err
}

func (s *sSysPublish) collectGlobalEnabled(ctx context.Context) bool {
	conf, err := s.CollectConfig(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "读取采集总开关失败：%+v", err)
		return true
	}
	return conf == nil || conf.Enabled == 1
}

func (s *sSysPublish) collectPushEnabled(ctx context.Context) bool {
	conf, err := s.CollectConfig(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "读取采集推送总开关失败：%+v", err)
		return true
	}
	return conf == nil || conf.PushEnabled == 1
}

func defaultCollectConfig() *sysin.CollectConfigModel {
	return &sysin.CollectConfigModel{Enabled: 1, PushEnabled: 1, RealtimePushDelaySec: 600}
}

type collectConfigStorage struct {
	CollectEnabled       int `json:"collectEnabled" dc:"采集总开关"`
	CollectPushEnabled   int `json:"collectPushEnabled" dc:"采集推送总开关"`
	RealtimePushDelaySec int `json:"realtimePushDelaySec" dc:"实时采集推送延迟秒数"`
}

func defaultCollectConfigStorage() *collectConfigStorage {
	return &collectConfigStorage{CollectEnabled: 1, CollectPushEnabled: 1, RealtimePushDelaySec: 600}
}

func normalizeCollectRealtimePushDelaySec(seconds int) int {
	if seconds < 0 {
		return 0
	}
	if seconds > 0 && seconds < 600 {
		return 600
	}
	if seconds > 600 {
		return 600
	}
	return seconds
}
