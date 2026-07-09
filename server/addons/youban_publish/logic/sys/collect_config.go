package sys

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const publishConfigGroupCollect = "collect"
const publishConfigKeyCollectEnabled = "collectEnabled"
const publishConfigKeyCollectRealtimePushDelaySec = "realtimePushDelaySec"

func (s *sSysPublish) CollectConfig(ctx context.Context) (*sysin.CollectConfigModel, error) {
	storage := defaultCollectConfigStorage()
	if err := NewSysConfig().scanConfigGroup(ctx, publishConfigGroupCollect, storage); err != nil {
		return nil, err
	}
	return &sysin.CollectConfigModel{
		Enabled:              storage.CollectEnabled,
		RealtimePushDelaySec: normalizeCollectRealtimePushDelaySec(storage.RealtimePushDelaySec),
	}, nil
}

func (s *sSysPublish) CollectConfigSave(ctx context.Context, in *sysin.CollectConfigSaveInp) error {
	if in == nil {
		in = &sysin.CollectConfigSaveInp{Enabled: 1}
	}
	if err := in.Filter(ctx); err != nil {
		return err
	}
	return NewSysConfig().updateConfigGroup(ctx, publishConfigGroupCollect, g.Map{
		publishConfigKeyCollectEnabled:              in.Enabled,
		publishConfigKeyCollectRealtimePushDelaySec: in.RealtimePushDelaySec,
	})
}

func (s *sSysPublish) collectGlobalEnabled(ctx context.Context) bool {
	conf, err := s.CollectConfig(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "读取采集总开关失败：%+v", err)
		return true
	}
	return conf == nil || conf.Enabled == 1
}

func defaultCollectConfig() *sysin.CollectConfigModel {
	return &sysin.CollectConfigModel{Enabled: 1, RealtimePushDelaySec: 60}
}

type collectConfigStorage struct {
	CollectEnabled       int `json:"collectEnabled" dc:"采集总开关"`
	RealtimePushDelaySec int `json:"realtimePushDelaySec" dc:"实时采集推送延迟秒数"`
}

func defaultCollectConfigStorage() *collectConfigStorage {
	return &collectConfigStorage{CollectEnabled: 1, RealtimePushDelaySec: 60}
}

func normalizeCollectRealtimePushDelaySec(seconds int) int {
	if seconds < 0 {
		return 0
	}
	if seconds > 600 {
		return 600
	}
	return seconds
}
