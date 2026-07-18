// Package model
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package model

import "github.com/gogf/gf/v2/os/gtime"

type State struct {
	Bots      map[string]*BotConfig     `json:"bots"`
	Users     map[int64]*UserRecord     `json:"users"`
	Bindings  map[string]*BindingRecord `json:"bindings"`
	Plugins   map[string]*PluginConfig  `json:"plugins"`
	Settings  *Settings                 `json:"settings"`
	Global    *GlobalConfig             `json:"global"`
	UpdatedAt *gtime.Time               `json:"updatedAt"`
}

type BotConfig struct {
	Id             int64                    `json:"id"`
	Key            string                   `json:"key"`
	Role           string                   `json:"role"`
	MemberId       int64                    `json:"memberId"`
	Token          string                   `json:"token"`
	DisplayName    string                   `json:"displayName"`
	Username       string                   `json:"username"`
	WebhookSecret  string                   `json:"webhookSecret"`
	WebhookPath    string                   `json:"webhookPath"`
	RuntimeStatus  string                   `json:"runtimeStatus"`
	RuntimeMessage string                   `json:"runtimeMessage"`
	Enabled        bool                     `json:"enabled"`
	AutoPull       bool                     `json:"autoPull"`
	AutoForward    bool                     `json:"autoForward"`
	ReviewEnabled  bool                     `json:"reviewEnabled"`
	Plugins        map[string]*PluginConfig `json:"plugins"`
	CreatedBy      int64                    `json:"createdBy"`
	CreatedAt      *gtime.Time              `json:"createdAt"`
	UpdatedAt      *gtime.Time              `json:"updatedAt"`
}

type UserRecord struct {
	TelegramID   int64       `json:"telegramId"`
	BotKey       string      `json:"botKey"`
	Username     string      `json:"username"`
	FirstName    string      `json:"firstName"`
	LastName     string      `json:"lastName"`
	LanguageCode string      `json:"languageCode"`
	IsBot        bool        `json:"isBot"`
	MemberLevel  int         `json:"memberLevel"`
	Points       float64     `json:"points"`
	Status       int         `json:"status"`
	LastActiveAt *gtime.Time `json:"lastActiveAt"`
	CreatedAt    *gtime.Time `json:"createdAt"`
	UpdatedAt    *gtime.Time `json:"updatedAt"`
}

type BindingRecord struct {
	ID              int64          `json:"id"`
	Key             string         `json:"key"`
	BotKey          string         `json:"botKey"`
	SourceURL       string         `json:"sourceUrl"`
	SourceToken     string         `json:"sourceToken"`
	ReviewChatID    int64          `json:"reviewChatId"`
	PublishChatID   int64          `json:"publishChatId"`
	LastPullID      int64          `json:"lastPullId"`
	LastCursor      string         `json:"lastCursor"`
	Status          string         `json:"status"`
	AutoPush        bool           `json:"autoPush"`
	VerifyEnabled   bool           `json:"verifyEnabled"`
	LocationEnabled bool           `json:"locationEnabled"`
	PluginState     map[string]any `json:"pluginState"`
	CreatedAt       *gtime.Time    `json:"createdAt"`
	UpdatedAt       *gtime.Time    `json:"updatedAt"`
}

type Settings struct {
	AllowVerify   bool   `json:"allowVerify"`
	AllowLocation bool   `json:"allowLocation"`
	MemberVerify  string `json:"memberVerify"`
	MemberPoints  string `json:"memberPoints"`
	SignFollow    bool   `json:"signFollow"`
}

type GlobalConfig struct {
	TelegramProxy string `json:"telegramProxy"`
}

type PluginConfig struct {
	Key              string                `json:"key"`
	Name             string                `json:"name"`
	Subtitle         string                `json:"subtitle"`
	Description      string                `json:"description"`
	Category         string                `json:"category"`
	Enabled          bool                  `json:"enabled"`
	UserEnabled      bool                  `json:"userEnabled"`
	Paid             bool                  `json:"paid"`
	Price            string                `json:"price"`
	ExpireDays       int                   `json:"expireDays"`
	VisibleInBinding bool                  `json:"visibleInBinding"`
	BindingActions   []PluginBindingAction `json:"bindingActions"`
	Sort             int                   `json:"sort"`
	Settings         map[string]any        `json:"settings"`
}

type PluginBindingAction struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Kind      string `json:"kind"`
	Placement string `json:"placement"`
	Visible   bool   `json:"visible"`
	AdminOnly bool   `json:"adminOnly"`
	Default   any    `json:"default"`
	Callback  string `json:"callback"`
}

type Runtime struct {
	BotKey  string `json:"botKey"`
	Enabled bool   `json:"enabled"`
}

func NewState() *State {
	return &State{}
}

func (s *State) Normalize() {
	if s.Bots == nil {
		s.Bots = map[string]*BotConfig{}
	}
	if s.Users == nil {
		s.Users = map[int64]*UserRecord{}
	}
	if s.Bindings == nil {
		s.Bindings = map[string]*BindingRecord{}
	}
	for _, item := range s.Bindings {
		if item != nil && item.PluginState == nil {
			item.PluginState = map[string]any{}
		}
	}
	s.Plugins = NormalizePluginConfigs(s.Plugins)
	for _, bot := range s.Bots {
		if bot == nil {
			continue
		}
		if bot.Role == "finance" {
			bot.Role = "official"
		}
		if bot.Role == "" {
			bot.Role = "user"
		}
		if bot.Plugins != nil {
			bot.Plugins = NormalizePluginConfigs(bot.Plugins)
		}
	}
	if s.Settings == nil {
		s.Settings = &Settings{}
	}
	if s.Global == nil {
		s.Global = &GlobalConfig{}
	}
	if s.UpdatedAt == nil {
		s.UpdatedAt = gtime.Now()
	}
}

func NormalizePluginConfigs(plugins map[string]*PluginConfig) map[string]*PluginConfig {
	if plugins == nil {
		plugins = map[string]*PluginConfig{}
	}
	defaults := DefaultPluginConfigs()
	for key, def := range defaults {
		item := plugins[key]
		if item == nil {
			plugins[key] = def
			continue
		}
		mergePluginDefault(item, def)
	}
	return plugins
}

func mergePluginDefault(item, def *PluginConfig) {
	if item == nil || def == nil {
		return
	}
	if item.Key == "" {
		item.Key = def.Key
	}
	if item.Name == "" {
		item.Name = def.Name
	}
	if item.Subtitle == "" {
		item.Subtitle = def.Subtitle
	}
	if item.Description == "" {
		item.Description = def.Description
	}
	if item.Category == "" {
		item.Category = def.Category
	}
	if item.Price == "" {
		item.Price = def.Price
	}
	if item.Sort == 0 {
		item.Sort = def.Sort
	}
	hadBindingActions := len(item.BindingActions) > 0
	item.BindingActions = mergePluginBindingActions(item.BindingActions, def.BindingActions)
	if !hadBindingActions {
		item.VisibleInBinding = def.VisibleInBinding
	}
	if item.Settings == nil {
		item.Settings = map[string]any{}
	}
	for key, value := range def.Settings {
		if _, ok := item.Settings[key]; !ok {
			item.Settings[key] = value
		}
	}
	if item.Key == "menu" {
		normalizeDefaultMenuSettings(item.Settings)
	}
}

func mergePluginBindingActions(actions, defaults []PluginBindingAction) []PluginBindingAction {
	if len(actions) == 0 {
		return append([]PluginBindingAction(nil), defaults...)
	}
	seen := make(map[string]struct{}, len(actions))
	out := append([]PluginBindingAction(nil), actions...)
	for _, action := range actions {
		if action.Key != "" {
			seen[action.Key] = struct{}{}
		}
	}
	for _, action := range defaults {
		if action.Key == "" {
			continue
		}
		if _, ok := seen[action.Key]; ok {
			continue
		}
		out = append(out, action)
	}
	return out
}

func (s *State) Clone() *State {
	if s == nil {
		return NewState()
	}
	out := NewState()
	*out = *s
	out.Normalize()
	return out
}

func DefaultPluginConfigs() map[string]*PluginConfig {
	return map[string]*PluginConfig{
		"collector": {
			Key:              "collector",
			Name:             "采集插件",
			Subtitle:         "BangChat 绑定采集",
			Description:      "支持配置采集、审核发布、笔记编号、验证视频和位置私聊解锁入口。",
			Category:         "采集",
			Enabled:          true,
			UserEnabled:      true,
			Paid:             false,
			Price:            "0",
			Sort:             10,
			VisibleInBinding: true,
			BindingActions: []PluginBindingAction{
				{
					Key:       "revealInBot",
					Label:     "验证/位置机器人查看",
					Kind:      "toggle",
					Placement: "panel",
					Visible:   true,
					AdminOnly: false,
					Default:   false,
					Callback:  "collector:reveal_links",
				},
				{
					Key:       "autoPull",
					Label:     "自动拉取",
					Kind:      "toggle",
					Placement: "panel",
					Visible:   true,
					AdminOnly: false,
					Default:   true,
					Callback:  "collector:auto_pull",
				},
				{
					Key:       "mergeVerifyInGroup",
					Label:     "验证视频合并",
					Kind:      "toggle",
					Placement: "panel",
					Visible:   true,
					AdminOnly: false,
					Default:   true,
					Callback:  "collector:merge_verify_group",
				},
			},
			Settings: map[string]any{
				"autoPull":           true,
				"menuVisible":        false,
				"command":            "/pull",
				"commands":           []any{"/bind", "/bind_review", "/bind_publish", "/pull"},
				"showVerifyLink":     true,
				"showLocationLink":   true,
				"mergeVerifyInGroup": true,
				"bindNotify":         false,
				"revealInBot":        false,
				"footer":             "",
				"bindHelpText":       "请发送 /bind <BangChat链接>。绑定后可在下方直接配置当前频道插件。",
				"quickBindText":      "绑定已保存：{source}\n\n发送 pull 或 拉取 可立即拉取。\n发送 pull 10 或 拉取 10 可拉取最近 10 条消息。",
				"reviewBindText":     "绑定已保存，稍后可在公开频道绑定发布入口。",
				"publishBindText":    "发布频道绑定成功。",
				"pullingText":        "开始采集，请稍候...",
				"verifyLinkText":     "📒 点击查看验证视频",
				"locationLinkText":   "📌 点击查看位置",
				"captionTemplate":    "编号：<code>{code}</code>\n\n{verify_link}\n{location_link}\n\n<b>{title}</b>\n{text}\n\n{footer}",
				"emptyFooterTipText": "当前未配置页脚。发送 /设置页脚 <内容> 可以设置每条笔记底部文案。",
			},
		},
		"welcome": {
			Key:         "welcome",
			Name:        "欢迎语插件",
			Subtitle:    "/start 入口欢迎语",
			Description: "首次关注与 /start 入口，可挂载底部菜单等插件能力。",
			Category:    "基础",
			Enabled:     true,
			UserEnabled: true,
			Paid:        false,
			Price:       "0",
			Sort:        15,
			Settings: map[string]any{
				"welcomeText":    "欢迎使用<b>懒羊羊TGGo</b>",
				"mountedPlugins": []any{"menu"},
				"mountMenu":      true,
			},
		},
		"menu": {
			Key:         "menu",
			Name:        "底部菜单插件",
			Subtitle:    "Reply Keyboard 按钮菜单",
			Description: "统一管理底部按钮布局，按钮可回复文本、打开链接或挂载插件功能。",
			Category:    "基础",
			Enabled:     true,
			UserEnabled: true,
			Paid:        false,
			Price:       "0",
			Sort:        16,
			Settings: map[string]any{
				"menuVisible":        true,
				"showPluginCommands": false,
				"buttons":            defaultUserBotMenuButtons(),
				"userButtons":        defaultUserBotMenuButtons(),
				"officialButtons":    defaultOfficialBotMenuButtons(),
			},
		},
		"review": {
			Key:         "review",
			Name:        "审核插件",
			Subtitle:    "审核群操作按钮",
			Description: "审核群消息、审批通过、查看位置和查看验证按钮。",
			Category:    "审核",
			Enabled:     true,
			UserEnabled: true,
			Paid:        false,
			Price:       "0",
			Sort:        20,
			Settings:    map[string]any{"allowVerify": true, "allowLocation": true, "menuVisible": false, "command": "/review"},
		},
		"signin": {
			Key:         "signin",
			Name:        "签到插件",
			Subtitle:    "签到和关注校验",
			Description: "用户签到、关注频道校验和人机验证。",
			Category:    "增长",
			Enabled:     false,
			UserEnabled: true,
			Paid:        false,
			Price:       "0",
			Sort:        30,
			Settings: map[string]any{
				"followRequired": false,
				"menuVisible":    true,
				"command":        "/sign",
				"commands":       []any{"/sign", "/签到"},
				"promptText":     "请先关注以下频道，再点击验证按钮完成签到。",
				"finishText":     "全部验证并签到",
				"openText":       "打开频道",
				"verifyText":     "验证关注",
				"successText":    "签到成功，感谢关注。",
				"failText":       "请先完成频道关注后再签到。",
				"rewardPoints":   0,
				"channels":       []any{},
			},
		},
		"footer": {
			Key:              "footer",
			Name:             "自定义底部插件",
			Subtitle:         "统一底部宣传文案",
			Description:      "给公开内容追加底部文案，支持全局替换和按频道覆盖。",
			Category:         "内容",
			Enabled:          true,
			UserEnabled:      true,
			Paid:             false,
			Price:            "0",
			Sort:             31,
			VisibleInBinding: true,
			BindingActions: []PluginBindingAction{
				{
					Key:       "useFooter",
					Label:     "使用页脚",
					Kind:      "switch",
					Placement: "panel",
					Visible:   true,
					AdminOnly: false,
					Default:   true,
					Callback:  "footer:toggle",
				},
				{
					Key:       "editFooter",
					Label:     "编辑页脚",
					Kind:      "button",
					Placement: "config",
					Visible:   true,
					AdminOnly: true,
					Default:   false,
					Callback:  "footer:edit",
				},
			},
			Settings: map[string]any{
				"menuVisible": false,
				"footerText":  "",
				"footerMode":  "append",
				"replaceAll":  false,
				"scope":       "public",
				"template":    "{footer}",
			},
		},
		"points": {
			Key:         "points",
			Name:        "积分中心插件",
			Subtitle:    "积分展示和规则",
			Description: "积分余额、积分记录、积分兑换和解锁入口。",
			Category:    "增长",
			Enabled:     false,
			UserEnabled: false,
			Paid:        false,
			Price:       "0",
			Sort:        32,
			Settings: map[string]any{
				"menuVisible": false,
				"command":     "/points",
				"commands":    []any{"/points", "/积分"},
				"balanceText": "当前积分：{points}",
				"ruleText":    "",
				"pointName":   "积分",
				"refreshText": "刷新余额",
				"signText":    "去签到",
			},
		},
		"profile": {
			Key:         "profile",
			Name:        "个人中心插件",
			Subtitle:    "账户资料、积分和邀请",
			Description: "展示账号名称、等级、积分、签到统计、邀请链接和邀请奖励。",
			Category:    "增长",
			Enabled:     true,
			UserEnabled: true,
			Paid:        false,
			Price:       "0",
			Sort:        33,
			Settings: map[string]any{
				"menuVisible":          true,
				"command":              "/profile",
				"commands":             []any{"/profile", "/个人中心"},
				"pointName":            "积分",
				"refreshText":          "刷新",
				"signText":             "签到",
				"inviteRewardPoints":   0,
				"inviteStartText":      "欢迎使用机器人。",
				"inviteRecordedText":   "邀请关系已记录，欢迎使用机器人。",
				"memberClosedText":     "未开通",
				"memberOpenedText":     "已开通",
				"memberExpireFallback": "未配置",
			},
		},
		"rights": {
			Key:         "rights",
			Name:        "内容权益插件",
			Subtitle:    "验证视频和位置解锁",
			Description: "控制验证视频、位置、会员和积分的查看门槛。",
			Category:    "变现",
			Enabled:     false,
			UserEnabled: false,
			Paid:        true,
			Price:       "0",
			Sort:        34,
			Settings: map[string]any{
				"menuVisible":        false,
				"verifyMode":         "none",
				"locationMode":       "none",
				"showVerifyButton":   true,
				"showLocationButton": true,
				"verifyButtonText":   "📒 点击查看验证视频",
				"locationButtonText": "📌 点击查看位置",
				"memberOnly":         false,
				"pointsCost":         0,
				"privateText":        "请先完成会员或积分校验后查看隐藏内容。",
			},
		},
		"map": {
			Key:         "map",
			Name:        "地图查看插件",
			Subtitle:    "高德/百度/腾讯入口",
			Description: "在机器人私聊中展示地图按钮和地址信息。",
			Category:    "内容",
			Enabled:     false,
			UserEnabled: false,
			Paid:        false,
			Price:       "0",
			Sort:        35,
			Settings: map[string]any{
				"menuVisible":      false,
				"providerButtons":  []any{"amap", "baidu", "tencent"},
				"coordType":        "auto",
				"showVenueMessage": true,
				"showLocationCard": true,
				"addressTemplate":  "{title}\n{address}",
			},
		},
		"member": {
			Key:         "member",
			Name:        "会员中心插件",
			Subtitle:    "会员和到期管理",
			Description: "会员身份、订阅到期、续费入口和会员权益说明。",
			Category:    "变现",
			Enabled:     false,
			UserEnabled: false,
			Paid:        true,
			Price:       "99",
			Sort:        41,
			Settings: map[string]any{
				"menuVisible":  false,
				"command":      "/member",
				"commands":     []any{"/member"},
				"durationDays": 30,
				"renewText":    "会员已开通，可在到期前续费。",
				"benefitText":  "会员可查看隐藏内容、验证视频和位置。",
			},
		},
		"help": {
			Key:         "help",
			Name:        "帮助插件",
			Subtitle:    "自定义帮助文本",
			Description: "点击底部帮助按钮后返回后台配置的帮助内容。",
			Category:    "基础",
			Enabled:     true,
			UserEnabled: true,
			Paid:        false,
			Price:       "0",
			Sort:        50,
			Settings: map[string]any{
				"menuVisible": false,
				"command":     "/help",
				"commands":    []any{"/help", "/帮助"},
				"helpText":    "请联系管理员获取帮助。",
			},
		},
	}
}

func defaultOfficialBotMenuButtons() []any {
	return []any{
		[]any{
			map[string]any{"text": "创建机器人", "action": "reply", "value": "请发送你的 Telegram Bot Token，系统会自动创建并绑定你的专属机器人。"},
			map[string]any{"text": "邀请赚积分", "action": "reply", "value": "邀请功能暂未开放。"},
		},
		[]any{
			map[string]any{"text": "个人中心", "action": "plugin", "value": "/profile"},
			map[string]any{"text": "签到", "action": "plugin", "value": "/sign"},
		},
		[]any{
			map[string]any{"text": "会员中心", "action": "plugin", "value": "/member"},
			map[string]any{"text": "反馈技术", "action": "reply", "value": "请联系管理员反馈使用问题。"},
		},
		[]any{
			map[string]any{"text": "帮助", "action": "plugin", "value": "/help"},
			map[string]any{"text": "管理员设置", "action": "plugin", "value": "管理员配置", "adminOnly": true},
		},
	}
}

func defaultUserBotMenuButtons() []any {
	return []any{
		[]any{
			map[string]any{"text": "签到", "action": "plugin", "value": "/sign"},
			map[string]any{"text": "个人中心", "action": "plugin", "value": "/profile"},
		},
		[]any{
			map[string]any{"text": "邀请赚积分", "action": "reply", "value": "邀请功能暂未开放。"},
			map[string]any{"text": "帮助", "action": "plugin", "value": "/help"},
		},
		[]any{
			map[string]any{"text": "管理员设置", "action": "plugin", "value": "管理员配置", "adminOnly": true},
		},
	}
}

func normalizeDefaultMenuSettings(settings map[string]any) {
	if settings == nil {
		return
	}
	if buttonRowsContainText(settings["userButtons"], "会员充值", "管理员配置") {
		settings["userButtons"] = defaultUserBotMenuButtons()
	}
	if buttonRowsContainText(settings["buttons"], "会员充值", "管理员配置") {
		settings["buttons"] = defaultUserBotMenuButtons()
	}
	migrateMenuButton(settings["officialButtons"])
	migrateMenuButton(settings["userButtons"])
	migrateMenuButton(settings["buttons"])
}

func buttonRowsContainText(raw any, texts ...string) bool {
	targets := map[string]struct{}{}
	for _, text := range texts {
		targets[text] = struct{}{}
	}
	rows, ok := raw.([]any)
	if !ok {
		return false
	}
	for _, rowRaw := range rows {
		row, ok := rowRaw.([]any)
		if !ok {
			continue
		}
		for _, itemRaw := range row {
			item, ok := itemRaw.(map[string]any)
			if !ok {
				continue
			}
			text, _ := item["text"].(string)
			if _, ok = targets[text]; ok {
				return true
			}
		}
	}
	return false
}

func migrateMenuButton(raw any) {
	rows, ok := raw.([]any)
	if !ok {
		return
	}
	for _, rowRaw := range rows {
		row, ok := rowRaw.([]any)
		if !ok {
			continue
		}
		for _, itemRaw := range row {
			item, ok := itemRaw.(map[string]any)
			if !ok {
				continue
			}
			text, _ := item["text"].(string)
			switch text {
			case "积分中心":
				item["text"] = "个人中心"
				item["action"] = "plugin"
				item["value"] = "/profile"
			case "个人中心":
				item["action"] = "plugin"
				item["value"] = "/profile"
			case "帮助":
				item["action"] = "plugin"
				item["value"] = "/help"
			}
		}
	}
}
