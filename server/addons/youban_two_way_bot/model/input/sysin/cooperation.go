package sysin

import (
	"context"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/internal/model/input/form"
	"strings"
)

const (
	CooperationReviewPending     = "pending"
	CooperationReviewApproved    = "approved"
	CooperationReviewRejected    = "rejected"
	CooperationReviewNotRequired = "not_required"
	CooperationReviewCanceled    = "canceled"
	CooperationJoinNotStarted    = "not_started"
	CooperationJoinProcessing    = "processing"
	CooperationJoinSuccess       = "success"
	CooperationJoinPartialFailed = "partial_failed"
	CooperationJoinFailed        = "failed"
	CooperationJoinRemoved       = "removed"
	CooperationJoinPartialRemove = "partial_remove_failed"
	CooperationJoinRemoveFailed  = "remove_failed"
)

type CooperationConfigSaveInp struct {
	BotId            int64   `json:"botId"`
	TwoWayBotId      int64   `json:"twoWayBotId"`
	NotificationType string  `json:"notificationType"`
	ReviewRequired   int     `json:"reviewRequired"`
	Status           int     `json:"status"`
	ChannelIds       []int64 `json:"channelIds"`
}

func (in *CooperationConfigSaveInp) Filter(context.Context) error {
	if in.TwoWayBotId <= 0 {
		return gerror.New("请选择双向机器人")
	}
	if len(in.ChannelIds) == 0 {
		return gerror.New("请选择上架频道")
	}
	in.NotificationType = strings.TrimSpace(in.NotificationType)
	if in.NotificationType == "" {
		in.NotificationType = "two_way"
	}
	if in.NotificationType != "two_way" && in.NotificationType != "official" {
		return gerror.New("通知方式不合法")
	}
	if in.ReviewRequired != 0 && in.ReviewRequired != 1 {
		return gerror.New("审核配置不合法")
	}
	if in.Status == 0 {
		in.Status = 1
	}
	if in.Status != 1 && in.Status != 2 {
		return gerror.New("状态不合法")
	}
	return nil
}

type CooperationConfigModel struct {
	Id               int64   `json:"id"`
	BotId            int64   `json:"botId"`
	BotName          string  `json:"botName"`
	BotUsername      string  `json:"botUsername"`
	TwoWayBotId      int64   `json:"twoWayBotId"`
	NotificationType string  `json:"notificationType"`
	ReviewRequired   int     `json:"reviewRequired"`
	Status           int     `json:"status"`
	ChannelIds       []int64 `json:"channelIds"`
}
type CooperationApplicationListInp struct {
	form.PageReq
	Keyword      string `json:"keyword"`
	ReviewStatus string `json:"reviewStatus"`
	JoinStatus   string `json:"joinStatus"`
}
type CooperationApplicationModel struct {
	Id                   int64                                 `json:"id"`
	ApplicantTgUserId    string                                `json:"applicantTgUserId"`
	ApplicantUsername    string                                `json:"applicantUsername"`
	ApplicantFirstName   string                                `json:"applicantFirstName"`
	ApplicantLastName    string                                `json:"applicantLastName"`
	SubmittedBotUserId   string                                `json:"submittedBotUserId"`
	SubmittedBotUsername string                                `json:"submittedBotUsername"`
	SubmittedBotName     string                                `json:"submittedBotName"`
	ReviewStatus         string                                `json:"reviewStatus"`
	JoinStatus           string                                `json:"joinStatus"`
	ErrorMessage         string                                `json:"errorMessage"`
	SubmittedAt          *gtime.Time                           `json:"submittedAt"`
	ReviewedAt           *gtime.Time                           `json:"reviewedAt"`
	Blacklisted          int                                   `json:"blacklisted"`
	Channels             []*CooperationApplicationChannelModel `json:"channels"`
}
type CooperationApplicationChannelModel struct {
	ChannelId    int64       `json:"channelId"`
	ChannelTitle string      `json:"channelTitle"`
	Status       string      `json:"status"`
	ErrorMessage string      `json:"errorMessage"`
	JoinedAt     *gtime.Time `json:"joinedAt"`
}
type CooperationApplicationActionInp struct {
	Id     int64  `json:"id"`
	Remark string `json:"remark"`
}
