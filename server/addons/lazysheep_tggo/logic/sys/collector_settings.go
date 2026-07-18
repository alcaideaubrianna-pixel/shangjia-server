package sys

import "hotgo/addons/lazysheep_tggo/model"

const collectorRevealLinksStateKey = "collector.revealInBot"
const collectorMergeVerifyGroupStateKey = "collector.mergeVerifyInGroup"

func collectorRevealLinksEnabled(plugins map[string]*model.PluginConfig, bindingState map[string]any) bool {
	if bindingState != nil {
		if v, ok := bindingState[collectorRevealLinksStateKey].(bool); ok {
			return v
		}
	}
	if cfg := plugins["collector"]; cfg != nil && cfg.Settings != nil {
		if v, ok := cfg.Settings["revealInBot"].(bool); ok {
			return v
		}
	}
	return false
}

func collectorMergeVerifyGroupEnabled(plugins map[string]*model.PluginConfig, bindingState map[string]any) bool {
	if bindingState != nil {
		if v, ok := bindingState[collectorMergeVerifyGroupStateKey].(bool); ok {
			return v
		}
	}
	return true
}

func withBindingCollectorSettings(settings map[string]any, plugins map[string]*model.PluginConfig, bindingState map[string]any) map[string]any {
	out := make(map[string]any, len(settings)+1)
	for key, value := range settings {
		out[key] = value
	}
	out["mergeVerifyInGroup"] = collectorMergeVerifyGroupEnabled(plugins, bindingState)
	return out
}
