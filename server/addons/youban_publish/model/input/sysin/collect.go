package sysin

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/model/input/form"
)

const (
	CollectSourceTypeAccount = "account"
	CollectSourceTypeBot     = "bot"
	CollectSourceTypeFollow  = "follow"

	CollectEventStatusPending   = "pending"
	CollectEventStatusProcessed = "processed"
	CollectEventStatusIgnored   = "ignored"
	CollectEventStatusFailed    = "failed"

	CollectReviewStatusPending  = "pending"
	CollectReviewStatusApproved = "approved"
	CollectReviewStatusRejected = "rejected"

	CollectDispatchStatusPending   = "pending"
	CollectDispatchStatusReviewing = "reviewing"
	CollectDispatchStatusSent      = "sent"
	CollectDispatchStatusSkipped   = "skipped"
	CollectDispatchStatusFailed    = "failed"
)

type CollectSourceListInp struct {
	form.PageReq
	Keyword    string `json:"keyword" dc:"关键词"`
	SourceType string `json:"sourceType" dc:"来源类型"`
	Status     int    `json:"status" dc:"状态"`
}

type CollectSourceModel struct {
	Id              int64       `json:"id" dc:"ID"`
	TenantId        int64       `json:"tenantId" dc:"租户ID"`
	AccountId       int64       `json:"accountId" dc:"账号ID"`
	SourceType      string      `json:"sourceType" dc:"来源类型"`
	Title           string      `json:"title" dc:"名称"`
	SourceChatId    string      `json:"sourceChatId" dc:"频道/群聊ID"`
	SourceUsername  string      `json:"sourceUsername" dc:"用户名"`
	TgAccountId     int64       `json:"tgAccountId" dc:"协议号ID"`
	BotId           int64       `json:"botId" dc:"机器人ID"`
	FollowAccountId int64       `json:"followAccountId" dc:"关注账号ID"`
	CollectEnabled  int         `json:"collectEnabled" dc:"采集开关"`
	RuleIds         []int64     `json:"ruleIds" dc:"绑定规则ID"`
	Status          int         `json:"status" dc:"状态"`
	EventTotal      int64       `json:"eventTotal" dc:"事件总数"`
	SuccessTotal    int64       `json:"successTotal" dc:"成功数"`
	FailedTotal     int64       `json:"failedTotal" dc:"失败数"`
	LastEventAt     *gtime.Time `json:"lastEventAt" dc:"最后事件时间"`
	Remark          string      `json:"remark" dc:"备注"`
	CreatedAt       *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt       *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type CollectSourceSaveInp struct {
	Id              int64   `json:"id" dc:"ID"`
	SourceType      string  `json:"sourceType" dc:"来源类型"`
	Title           string  `json:"title" dc:"名称"`
	SourceChatId    string  `json:"sourceChatId" dc:"频道/群聊ID"`
	SourceUsername  string  `json:"sourceUsername" dc:"用户名"`
	TgAccountId     int64   `json:"tgAccountId" dc:"协议号ID"`
	BotId           int64   `json:"botId" dc:"机器人ID"`
	FollowAccountId int64   `json:"followAccountId" dc:"关注账号ID"`
	CollectEnabled  int     `json:"collectEnabled" dc:"采集开关"`
	RuleIds         []int64 `json:"ruleIds" dc:"绑定规则ID"`
	Remark          string  `json:"remark" dc:"备注"`
	Status          int     `json:"status" dc:"状态"`
}

func (in *CollectSourceSaveInp) Filter(ctx context.Context) error {
	in.SourceType = strings.TrimSpace(in.SourceType)
	in.Title = strings.TrimSpace(in.Title)
	in.SourceChatId = strings.TrimSpace(in.SourceChatId)
	in.SourceUsername = strings.TrimSpace(in.SourceUsername)
	in.Remark = strings.TrimSpace(in.Remark)
	if in.SourceType == "" {
		in.SourceType = CollectSourceTypeBot
	}
	if in.SourceType != CollectSourceTypeAccount && in.SourceType != CollectSourceTypeBot && in.SourceType != CollectSourceTypeFollow {
		return gerror.New("采集源类型不合法")
	}
	if in.Title == "" {
		return gerror.New("采集源名称不能为空")
	}
	if in.CollectEnabled != 1 {
		in.CollectEnabled = 0
	}
	if in.Status == 0 {
		in.Status = 1
	}
	return nil
}

type CollectRuleListInp struct {
	form.PageReq
	Keyword       string `json:"keyword" dc:"关键词"`
	GlobalEnabled int    `json:"globalEnabled" dc:"全局开关"`
	Status        int    `json:"status" dc:"状态"`
}

type CollectRuleModel struct {
	Id                   int64       `json:"id" dc:"ID"`
	TenantId             int64       `json:"tenantId" dc:"租户ID"`
	AccountId            int64       `json:"accountId" dc:"账号ID"`
	Name                 string      `json:"name" dc:"名称"`
	GlobalEnabled        int         `json:"globalEnabled" dc:"全局应用"`
	TargetChannelIdJson  string      `json:"targetChannelIdJson" dc:"目标频道JSON"`
	BotIdJson            string      `json:"botIdJson" dc:"BOT JSON"`
	ReviewEnabled        int         `json:"reviewEnabled" dc:"审核开关"`
	DedupeEnabled        int         `json:"dedupeEnabled" dc:"去重开关"`
	DedupeDays           int         `json:"dedupeDays" dc:"去重天数"`
	KeywordJson          string      `json:"keywordJson" dc:"关键词JSON"`
	TagJson              string      `json:"tagJson" dc:"标签JSON"`
	ReplaceJson          string      `json:"replaceJson" dc:"替换JSON"`
	BlockTextJson        string      `json:"blockTextJson" dc:"屏蔽文本JSON"`
	BlockLink            int         `json:"blockLink" dc:"屏蔽链接"`
	BlockUsername        int         `json:"blockUsername" dc:"屏蔽用户名"`
	BlockPlainText       int         `json:"blockPlainText" dc:"屏蔽纯文本"`
	MinMediaCountEnabled int         `json:"minMediaCountEnabled" dc:"媒体数量开关"`
	MinMediaCount        int         `json:"minMediaCount" dc:"最少媒体数"`
	ShowUniqueNo         int         `json:"showUniqueNo" dc:"显示唯一编号"`
	HeaderEnabled        int         `json:"headerEnabled" dc:"前置文案开关"`
	HeaderMarkdown       string      `json:"headerMarkdown" dc:"前置文案"`
	FooterEnabled        int         `json:"footerEnabled" dc:"后置文案开关"`
	FooterMarkdown       string      `json:"footerMarkdown" dc:"后置文案"`
	Sort                 int         `json:"sort" dc:"排序"`
	Status               int         `json:"status" dc:"状态"`
	CreatedAt            *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt            *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type CollectRuleSaveInp struct {
	Id                   int64  `json:"id" dc:"ID"`
	Name                 string `json:"name" dc:"名称"`
	GlobalEnabled        int    `json:"globalEnabled" dc:"全局应用"`
	TargetChannelIdJson  string `json:"targetChannelIdJson" dc:"目标频道JSON"`
	BotIdJson            string `json:"botIdJson" dc:"BOT JSON"`
	BackupChannelId      int64  `json:"backupChannelId" dc:"备份群ID"`
	ReviewEnabled        int    `json:"reviewEnabled" dc:"审核开关"`
	DedupeEnabled        int    `json:"dedupeEnabled" dc:"去重开关"`
	DedupeDays           int    `json:"dedupeDays" dc:"去重天数"`
	KeywordJson          string `json:"keywordJson" dc:"关键词JSON"`
	TagJson              string `json:"tagJson" dc:"标签JSON"`
	ReplaceJson          string `json:"replaceJson" dc:"替换JSON"`
	BlockTextJson        string `json:"blockTextJson" dc:"屏蔽文本JSON"`
	BlockLink            int    `json:"blockLink" dc:"屏蔽链接"`
	BlockUsername        int    `json:"blockUsername" dc:"屏蔽用户名"`
	BlockPlainText       int    `json:"blockPlainText" dc:"屏蔽纯文本"`
	MinMediaCountEnabled int    `json:"minMediaCountEnabled" dc:"媒体数量开关"`
	MinMediaCount        int    `json:"minMediaCount" dc:"最少媒体数"`
	ShowUniqueNo         int    `json:"showUniqueNo" dc:"显示唯一编号"`
	HeaderEnabled        int    `json:"headerEnabled" dc:"前置文案开关"`
	HeaderMarkdown       string `json:"headerMarkdown" dc:"前置文案"`
	FooterEnabled        int    `json:"footerEnabled" dc:"后置文案开关"`
	FooterMarkdown       string `json:"footerMarkdown" dc:"后置文案"`
	Sort                 int    `json:"sort" dc:"排序"`
	Status               int    `json:"status" dc:"状态"`
}

func (in *CollectRuleSaveInp) Filter(ctx context.Context) error {
	in.Name = strings.TrimSpace(in.Name)
	in.TargetChannelIdJson = strings.TrimSpace(in.TargetChannelIdJson)
	in.BotIdJson = strings.TrimSpace(in.BotIdJson)
	if in.Name == "" {
		return gerror.New("规则名称不能为空")
	}
	if emptyCollectJSON(in.TargetChannelIdJson) {
		return gerror.New("目标频道不能为空")
	}
	if emptyCollectJSON(in.BotIdJson) {
		return gerror.New("推送BOT不能为空")
	}
	if in.DedupeDays <= 0 || in.DedupeDays > 7 {
		in.DedupeDays = 7
	}
	if in.MinMediaCount <= 0 {
		in.MinMediaCount = 2
	}
	if in.Status == 0 {
		in.Status = 1
	}
	return nil
}

func emptyCollectJSON(value string) bool {
	value = strings.TrimSpace(value)
	return value == "" || value == "[]" || value == "{}" || value == "null"
}

type IdsInp struct {
	Ids []int64 `json:"ids" v:"required#请选择数据" dc:"ID列表"`
}

type CollectStatusInp struct {
	Id      int64 `json:"id" v:"required|min:1#ID不能为空|ID不能为空" dc:"ID"`
	Enabled int   `json:"enabled" dc:"开关"`
	Status  int   `json:"status" dc:"状态"`
}

type CollectSourceDownInp struct {
	Id int64 `json:"id" v:"required|min:1#采集源ID不能为空|采集源ID不能为空" dc:"采集源ID"`
}
