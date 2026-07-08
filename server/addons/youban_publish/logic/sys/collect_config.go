package sys

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const publishConfigGroupCollect = "collect"

func (s *sSysPublish) CollectConfig(ctx context.Context) (*sysin.CollectConfigModel, error) {
	conf := defaultCollectConfig()
	if err := NewSysConfig().scanConfigGroup(ctx, publishConfigGroupCollect, conf); err != nil {
		return nil, err
	}
	return conf, nil
}

func (s *sSysPublish) CollectConfigSave(ctx context.Context, in *sysin.CollectConfigSaveInp) error {
	if in == nil {
		in = &sysin.CollectConfigSaveInp{Enabled: 1}
	}
	if err := in.Filter(ctx); err != nil {
		return err
	}
	return NewSysConfig().updateConfigGroup(ctx, publishConfigGroupCollect, g.Map{
		"enabled": in.Enabled,
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
	return &sysin.CollectConfigModel{Enabled: 1}
}
