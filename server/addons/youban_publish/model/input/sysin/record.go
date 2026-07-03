package sysin

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/model/input/form"
)

type PublishRecordListInp struct {
	form.PageReq
	Action    string `json:"action" dc:"动作"`
	Keyword   string `json:"keyword" dc:"关键词"`
	ProfileId int64  `json:"profileId" dc:"资料ID"`
	Status    string `json:"status" dc:"状态"`
	TaskId    int64  `json:"taskId" dc:"任务ID"`
}

func (in *PublishRecordListInp) Filter(ctx context.Context) error {
	in.Action = strings.TrimSpace(in.Action)
	in.Keyword = strings.TrimSpace(in.Keyword)
	in.Status = strings.TrimSpace(in.Status)
	return nil
}

type PublishRecordModel struct {
	AccountId       int64       `json:"accountId" dc:"账号ID"`
	AccountName     string      `json:"accountName" dc:"账号名称"`
	Action          string      `json:"action" dc:"动作"`
	BotId           int64       `json:"botId" dc:"Bot ID"`
	BotName         string      `json:"botName" dc:"Bot名称"`
	BotUsername     string      `json:"botUsername" dc:"Bot用户名"`
	ChannelId       int64       `json:"channelId" dc:"频道ID"`
	ChannelTitle    string      `json:"channelTitle" dc:"频道名称"`
	ChannelUsername string      `json:"channelUsername" dc:"频道用户名"`
	CreatedAt       *gtime.Time `json:"createdAt" dc:"创建时间"`
	Id              int64       `json:"id" dc:"日志ID"`
	JobId           int64       `json:"jobId" dc:"TG任务ID"`
	Message         string      `json:"message" dc:"日志内容"`
	ProfileId       int64       `json:"profileId" dc:"资料ID"`
	Status          string      `json:"status" dc:"状态"`
	TargetChatId    string      `json:"targetChatId" dc:"目标Chat ID"`
	TaskId          int64       `json:"taskId" dc:"任务ID"`
	Title           string      `json:"title" dc:"资料标题"`
}

type DevPublishChainTestInp struct {
	ChannelIds            []int64  `json:"channelIds" dc:"指定频道ID，空则使用默认上架频道"`
	FilePaths             []string `json:"filePaths" dc:"本地测试文件路径"`
	IncludeScheduled      int      `json:"includeScheduled" dc:"是否创建定时测试：0否 1是"`
	ScheduledDelaySeconds int      `json:"scheduledDelaySeconds" dc:"定时延迟秒数"`
	SubmitNow             int      `json:"submitNow" dc:"是否立即提交推送队列：0否 1是"`
	TitleModes            []string `json:"titleModes" dc:"标题模式"`
	Variants              int      `json:"variants" dc:"测试资料数量"`
}

func (in *DevPublishChainTestInp) Filter(ctx context.Context) error {
	if in == nil {
		return gerror.New("测试参数不能为空")
	}
	in.FilePaths = uniqueTrimmedStrings(in.FilePaths)
	in.TitleModes = uniqueTrimmedStrings(in.TitleModes)
	in.ChannelIds = uniquePositiveInt64s(in.ChannelIds)
	if in.Variants <= 0 {
		in.Variants = 3
	}
	if in.Variants > 5 {
		return gerror.New("单次最多创建5条测试资料")
	}
	if in.SubmitNow == 0 && in.IncludeScheduled == 0 {
		in.SubmitNow = 1
	}
	if in.SubmitNow != 0 && in.SubmitNow != 1 {
		return gerror.New("立即提交配置不合法")
	}
	if in.IncludeScheduled != 0 && in.IncludeScheduled != 1 {
		return gerror.New("定时测试配置不合法")
	}
	if in.ScheduledDelaySeconds <= 0 {
		in.ScheduledDelaySeconds = 90
	}
	if in.ScheduledDelaySeconds > 3600 {
		return gerror.New("定时延迟不能超过3600秒")
	}
	return nil
}

type DevPublishChainTestModel struct {
	ChannelIds []int64                    `json:"channelIds" dc:"使用频道ID"`
	Items      []*DevPublishChainTestItem `json:"items" dc:"测试资料"`
}

type DevPublishChainTestItem struct {
	MediaIds  []int64 `json:"mediaIds" dc:"媒体ID"`
	ProfileId int64   `json:"profileId" dc:"资料ID"`
	PublishAt string  `json:"publishAt" dc:"定时发布时间"`
	Submitted bool    `json:"submitted" dc:"是否已提交队列"`
	TaskId    int64   `json:"taskId" dc:"任务ID"`
	Title     string  `json:"title" dc:"标题"`
}

func uniqueTrimmedStrings(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(items))
	list := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		list = append(list, value)
	}
	return list
}

func uniquePositiveInt64s(items []int64) []int64 {
	if len(items) == 0 {
		return []int64{}
	}
	seen := make(map[int64]struct{}, len(items))
	list := make([]int64, 0, len(items))
	for _, item := range items {
		if item <= 0 {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		list = append(list, item)
	}
	return list
}
