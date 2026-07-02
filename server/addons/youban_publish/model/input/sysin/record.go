package sysin

import (
	"context"
	"strings"

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
