package sysin

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/model/input/form"
)

const (
	AccountFollowStatusPending  = "pending"
	AccountFollowStatusApproved = "approved"
	AccountFollowStatusRejected = "rejected"
	AccountFollowStatusBlocked  = "blocked"
)

type AccountProfileViewInp struct {
	AccountId int64  `json:"accountId" dc:"账号ID"`
	Username  string `json:"username" dc:"用户名"`
}

type AccountProfileModel struct {
	Id                     int64       `json:"id" dc:"账号ID"`
	TenantId               int64       `json:"tenantId" dc:"租户ID"`
	Nickname               string      `json:"nickname" dc:"昵称"`
	Username               string      `json:"username" dc:"用户名"`
	AvatarUrl              string      `json:"avatarUrl" dc:"头像"`
	TelegramUsername       string      `json:"telegramUsername" dc:"TG用户名"`
	ContactTelegram        string      `json:"contactTelegram" dc:"联系TG"`
	ContactWechat          string      `json:"contactWechat" dc:"联系微信"`
	ContactPhone           string      `json:"contactPhone" dc:"联系电话"`
	ContactOther           string      `json:"contactOther" dc:"其他联系方式"`
	Remark                 string      `json:"remark" dc:"简介"`
	FollowApprovalRequired int         `json:"followApprovalRequired" dc:"关注需审批"`
	PublicFollowEnabled    int         `json:"publicFollowEnabled" dc:"公开关注"`
	NoteCount              int         `json:"noteCount" dc:"笔记数"`
	FollowingCount         int         `json:"followingCount" dc:"关注数"`
	FollowerCount          int         `json:"followerCount" dc:"粉丝数"`
	FollowStatus           string      `json:"followStatus" dc:"当前关注状态"`
	CreatedAt              *gtime.Time `json:"createdAt" dc:"创建时间"`
}

type AccountProfileSaveInp struct {
	Nickname               string `json:"nickname" dc:"昵称"`
	AvatarUrl              string `json:"avatarUrl" dc:"头像"`
	ContactTelegram        string `json:"contactTelegram" dc:"联系TG"`
	ContactWechat          string `json:"contactWechat" dc:"联系微信"`
	ContactPhone           string `json:"contactPhone" dc:"联系电话"`
	ContactOther           string `json:"contactOther" dc:"其他联系方式"`
	Remark                 string `json:"remark" dc:"简介"`
	FollowApprovalRequired int    `json:"followApprovalRequired" dc:"关注需审批"`
	PublicFollowEnabled    int    `json:"publicFollowEnabled" dc:"公开关注"`
}

func (in *AccountProfileSaveInp) Filter(ctx context.Context) error {
	in.Nickname = strings.TrimSpace(in.Nickname)
	in.AvatarUrl = strings.TrimSpace(in.AvatarUrl)
	in.ContactTelegram = strings.TrimSpace(in.ContactTelegram)
	in.ContactWechat = strings.TrimSpace(in.ContactWechat)
	in.ContactPhone = strings.TrimSpace(in.ContactPhone)
	in.ContactOther = strings.TrimSpace(in.ContactOther)
	in.Remark = strings.TrimSpace(in.Remark)
	if in.Nickname == "" {
		return gerror.New("账号昵称不能为空")
	}
	if in.FollowApprovalRequired != 1 {
		in.FollowApprovalRequired = 0
	}
	if in.PublicFollowEnabled != 0 {
		in.PublicFollowEnabled = 1
	}
	return nil
}

type AccountFollowListInp struct {
	form.PageReq
	ListType string `json:"listType" dc:"following/follower/request/public/blocked"`
	Keyword  string `json:"keyword" dc:"关键词"`
	Status   string `json:"status" dc:"状态"`
}

type AccountFollowModel struct {
	Id                       int64       `json:"id" dc:"ID"`
	AccountId                int64       `json:"accountId" dc:"对方账号ID"`
	Nickname                 string      `json:"nickname" dc:"昵称"`
	Username                 string      `json:"username" dc:"用户名"`
	AvatarUrl                string      `json:"avatarUrl" dc:"头像"`
	Remark                   string      `json:"remark" dc:"简介"`
	NoteCount                int         `json:"noteCount" dc:"笔记数"`
	FollowingCount           int         `json:"followingCount" dc:"关注数"`
	FollowerCount            int         `json:"followerCount" dc:"粉丝数"`
	LastNoteAt               *gtime.Time `json:"lastNoteAt" dc:"最近发布时间"`
	Status                   string      `json:"status" dc:"状态"`
	ApprovalRequiredSnapshot int         `json:"approvalRequiredSnapshot" dc:"申请时需审批"`
	CreatedAt                *gtime.Time `json:"createdAt" dc:"创建时间"`
	ApprovedAt               *gtime.Time `json:"approvedAt" dc:"通过时间"`
}

type AccountFollowApplyInp struct {
	Username string `json:"username" v:"required#用户名不能为空" dc:"用户名"`
	Remark   string `json:"remark" dc:"备注"`
}

func (in *AccountFollowApplyInp) Filter(ctx context.Context) error {
	in.Username = strings.TrimSpace(in.Username)
	in.Remark = strings.TrimSpace(in.Remark)
	if in.Username == "" {
		return gerror.New("用户名不能为空")
	}
	return nil
}

type AccountFollowActionInp struct {
	Id        int64  `json:"id" dc:"关注记录ID"`
	AccountId int64  `json:"accountId" dc:"账号ID"`
	Action    string `json:"action" v:"required#操作不能为空" dc:"approve/reject/block/unblock/remove"`
	Remark    string `json:"remark" dc:"备注"`
}

func (in *AccountFollowActionInp) Filter(ctx context.Context) error {
	in.Action = strings.TrimSpace(in.Action)
	in.Remark = strings.TrimSpace(in.Remark)
	switch in.Action {
	case "approve", "reject", "block", "unblock", "remove":
	default:
		return gerror.New("关注操作不合法")
	}
	if in.Id <= 0 && in.AccountId <= 0 {
		return gerror.New("关注记录或账号不能为空")
	}
	return nil
}

type FollowNoteListInp struct {
	ProfileListInp
	Scope string `json:"scope" dc:"mine/all/following"`
}
