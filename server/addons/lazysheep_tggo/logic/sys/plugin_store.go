// Package sys
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package sys

import (
	"context"
	"encoding/json"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/lazysheep_tggo/global"
	"hotgo/addons/lazysheep_tggo/model"
	"hotgo/internal/dao"
)

func (s *sLazySheepTGGo) loadPlugins(ctx context.Context, state *model.State) error {
	if state == nil {
		return nil
	}
	cols := dao.SysAddonsConfig.Columns()
	val, err := dao.SysAddonsConfig.Ctx(ctx).
		Fields(cols.Value).
		Where(cols.AddonName, global.GetSkeleton().Name).
		Where(cols.Group, "plugins").
		Where(cols.Key, "items").
		Value()
	if err != nil {
		return gerror.Wrap(err, "获取TG能力插件配置失败")
	}
	if val.IsNil() || val.String() == "" {
		return nil
	}
	var plugins map[string]*model.PluginConfig
	if err = json.Unmarshal([]byte(val.String()), &plugins); err != nil {
		return gerror.Wrap(err, "解析TG能力插件配置失败")
	}
	state.Plugins = plugins
	return nil
}

func (s *sLazySheepTGGo) loadGlobal(ctx context.Context, state *model.State) error {
	if state == nil {
		return nil
	}
	cols := dao.SysAddonsConfig.Columns()
	val, err := dao.SysAddonsConfig.Ctx(ctx).
		Fields(cols.Value).
		Where(cols.AddonName, global.GetSkeleton().Name).
		Where(cols.Group, "global").
		Where(cols.Key, "settings").
		Value()
	if err != nil {
		return gerror.Wrap(err, "获取TG全局配置失败")
	}
	if val.IsNil() || val.String() == "" {
		return nil
	}
	var cfg model.GlobalConfig
	if err = json.Unmarshal([]byte(val.String()), &cfg); err != nil {
		return gerror.Wrap(err, "解析TG全局配置失败")
	}
	state.Global = &cfg
	return nil
}

func (s *sLazySheepTGGo) loadBotPlugins(ctx context.Context, state *model.State) error {
	if state == nil {
		return nil
	}
	cols := dao.SysAddonsConfig.Columns()
	for key, item := range state.Bots {
		if item == nil {
			continue
		}
		item.Plugins = clonePluginConfigs(state.Plugins)
		val, err := dao.SysAddonsConfig.Ctx(ctx).
			Fields(cols.Value).
			Where(cols.AddonName, global.GetSkeleton().Name).
			Where(cols.Group, "bot_plugins").
			Where(cols.Key, key).
			Value()
		if err != nil {
			return gerror.Wrap(err, "获取机器人插件配置失败")
		}
		if val.IsNil() || val.String() == "" {
			continue
		}
		var plugins map[string]*model.PluginConfig
		if err = json.Unmarshal([]byte(val.String()), &plugins); err != nil {
			return gerror.Wrap(err, "解析机器人插件配置失败")
		}
		mergePluginConfigs(item.Plugins, plugins)
	}
	return nil
}

func (s *sLazySheepTGGo) savePlugins(ctx context.Context, plugins map[string]*model.PluginConfig) error {
	if plugins == nil {
		plugins = model.DefaultPluginConfigs()
	}
	data, err := json.Marshal(plugins)
	if err != nil {
		return gerror.Wrap(err, "编码TG能力插件配置失败")
	}
	return s.upsertAddonConfig(ctx, "plugins", "items", "TG能力插件配置", string(data), 10, "懒羊羊TGGo内部能力插件列表与开关")
}

func (s *sLazySheepTGGo) saveGlobal(ctx context.Context, cfg *model.GlobalConfig) error {
	if cfg == nil {
		cfg = &model.GlobalConfig{}
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return gerror.Wrap(err, "编码TG全局配置失败")
	}
	return s.upsertAddonConfig(ctx, "global", "settings", "TG全局配置", string(data), 20, "懒羊羊TGGo全局运行配置")
}

func (s *sLazySheepTGGo) saveBotPlugins(ctx context.Context, bots map[string]*model.BotConfig) error {
	for key, item := range bots {
		if item == nil || item.Plugins == nil {
			continue
		}
		data, err := json.Marshal(item.Plugins)
		if err != nil {
			return gerror.Wrap(err, "编码机器人插件配置失败")
		}
		if err = s.upsertAddonConfig(ctx, "bot_plugins", key, "机器人插件配置", string(data), 30, "单个机器人插件开关与配置"); err != nil {
			return err
		}
	}
	return nil
}

func (s *sLazySheepTGGo) savePluginConfig(ctx context.Context, state *model.State) error {
	if state == nil {
		state = model.NewState()
	}
	state.Normalize()
	if err := s.savePlugins(ctx, state.Plugins); err != nil {
		return err
	}
	if err := s.clearBotPluginOverrides(ctx, []string{"welcome", "menu"}); err != nil {
		return err
	}
	if err := s.SyncAllBots(ctx); err != nil {
		return gerror.Wrap(err, "同步机器人插件配置失败")
	}
	return nil
}

func (s *sLazySheepTGGo) clearBotPluginOverrides(ctx context.Context, pluginKeys []string) error {
	if len(pluginKeys) == 0 {
		return nil
	}
	cols := dao.SysAddonsConfig.Columns()
	var rows []struct {
		Id    int64  `json:"id"`
		Value string `json:"value"`
	}
	if err := dao.SysAddonsConfig.Ctx(ctx).
		Fields(cols.Id, cols.Value).
		Where(cols.AddonName, global.GetSkeleton().Name).
		Where(cols.Group, "bot_plugins").
		Scan(&rows); err != nil {
		return gerror.Wrap(err, "获取机器人插件覆盖配置失败")
	}
	for _, row := range rows {
		var plugins map[string]*model.PluginConfig
		if err := json.Unmarshal([]byte(row.Value), &plugins); err != nil {
			return gerror.Wrap(err, "解析机器人插件覆盖配置失败")
		}
		changed := false
		for _, key := range pluginKeys {
			if _, ok := plugins[key]; ok {
				delete(plugins, key)
				changed = true
			}
		}
		if !changed {
			continue
		}
		data, err := json.Marshal(plugins)
		if err != nil {
			return gerror.Wrap(err, "编码机器人插件覆盖配置失败")
		}
		if _, err = dao.SysAddonsConfig.Ctx(ctx).
			Where(cols.Id, row.Id).
			Data(g.Map{cols.Value: string(data), cols.UpdatedAt: gtime.Now()}).
			Update(); err != nil {
			return gerror.Wrap(err, "清理机器人插件覆盖配置失败")
		}
	}
	return nil
}

func (s *sLazySheepTGGo) upsertAddonConfig(ctx context.Context, group, key, name, value string, sort int, tip string) error {
	cols := dao.SysAddonsConfig.Columns()
	row := g.Map{
		cols.AddonName:    global.GetSkeleton().Name,
		cols.Group:        group,
		cols.Name:         name,
		cols.Type:         "string",
		cols.Key:          key,
		cols.Value:        value,
		cols.DefaultValue: "{}",
		cols.Sort:         sort,
		cols.Tip:          tip,
		cols.IsDefault:    1,
		cols.Status:       1,
		cols.UpdatedAt:    gtime.Now(),
	}
	existing, err := dao.SysAddonsConfig.Ctx(ctx).
		Fields(cols.Id).
		Where(cols.AddonName, global.GetSkeleton().Name).
		Where(cols.Group, group).
		Where(cols.Key, key).
		Value()
	if err != nil {
		return gerror.Wrap(err, "查询TG插件配置失败")
	}
	if existing.IsNil() {
		row[cols.CreatedAt] = gtime.Now()
		_, err = dao.SysAddonsConfig.Ctx(ctx).Data(row).Insert()
	} else {
		_, err = dao.SysAddonsConfig.Ctx(ctx).Where(cols.Id, existing.Int64()).Data(row).Update()
	}
	if err != nil {
		return gerror.Wrap(err, "保存TG插件配置失败")
	}
	return nil
}

func clonePluginConfigs(items map[string]*model.PluginConfig) map[string]*model.PluginConfig {
	out := map[string]*model.PluginConfig{}
	mergePluginConfigs(out, model.DefaultPluginConfigs())
	mergePluginConfigs(out, items)
	return model.NormalizePluginConfigs(out)
}

func mergePluginConfigs(dst, src map[string]*model.PluginConfig) {
	for key, item := range src {
		if item == nil {
			continue
		}
		base := dst[key]
		copied := *item
		if base != nil {
			copied.BindingActions = base.BindingActions
			copied.VisibleInBinding = base.VisibleInBinding
			if len(item.BindingActions) > 0 {
				copied.BindingActions = item.BindingActions
				copied.VisibleInBinding = item.VisibleInBinding
			}
		}
		copied.Settings = map[string]any{}
		if base != nil {
			for settingKey, settingValue := range base.Settings {
				copied.Settings[settingKey] = settingValue
			}
		}
		for settingKey, settingValue := range item.Settings {
			copied.Settings[settingKey] = settingValue
		}
		dst[key] = &copied
	}
}
