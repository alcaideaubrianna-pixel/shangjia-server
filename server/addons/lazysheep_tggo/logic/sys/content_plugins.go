// Package sys
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package sys

import (
	"context"
	"strings"

	"hotgo/addons/lazysheep_tggo/model"
)

const (
	collectorAutoPullStateKey      = "collector.autoPull"
	collectorAutoPullStoppedAtKey  = "collector.autoPullStoppedAt"
	collectorAutoPullStopReasonKey = "collector.autoPullStopReason"
	collectorBindOperatorIDKey     = "collector.bindOperatorId"
)

func (s *sLazySheepTGGo) defaultBindingPluginState(ctx context.Context, botKey string) map[string]any {
	state := map[string]any{}
	plugins := s.collectorPlugins(ctx, botKey)
	if cfg := plugins["footer"]; cfg != nil && cfg.Enabled && cfg.VisibleInBinding {
		state["footer.useFooter"] = true
	}
	if cfg := plugins["collector"]; cfg != nil && cfg.Enabled {
		state["collector.revealInBot"] = false
		state[collectorMergeVerifyGroupStateKey] = true
		state[collectorAutoPullStateKey] = true
	}
	return state
}

func resolveContentFooter(plugins map[string]*model.PluginConfig, settings map[string]any, bindingState map[string]any) string {
	if cfg := plugins["footer"]; cfg != nil && !cfg.Enabled {
		return ""
	}
	if !pluginStateBoolValue(bindingState, "footer.useFooter", true) {
		return ""
	}
	if text := pluginSettingString(bindingState, "footer.text", ""); text != "" {
		return text
	}
	if cfg := plugins["footer"]; cfg != nil && cfg.Enabled {
		if text := pluginSettingString(cfg.Settings, "footerText", ""); text != "" {
			return text
		}
	}
	if text := pushSettingString(settings, "footer", ""); text != "" {
		return text
	}
	if text := pluginSettingString(settings, "footerText", ""); text != "" {
		return text
	}
	return ""
}

func pluginStateBoolValue(settings map[string]any, key string, fallback bool) bool {
	if settings == nil {
		return fallback
	}
	if v, ok := settings[key].(bool); ok {
		return v
	}
	return fallback
}

func pluginSettingString(settings map[string]any, key, fallback string) string {
	if settings == nil {
		return fallback
	}
	if v, ok := settings[key].(string); ok {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return fallback
}
