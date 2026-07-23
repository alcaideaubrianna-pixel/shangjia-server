package sysin

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/model/input/form"
)

type TgChannelMemberListInp struct {
	form.PageReq
	TgAccountId int64    `json:"tgAccountId" dc:"TG账号ID"`
	ChannelId   string   `json:"channelId" dc:"频道ID"`
	Role        string   `json:"role" dc:"角色: creator/admin/member"`
	Roles       []string `json:"roles" dc:"角色列表"`
	Keyword     string   `json:"keyword" dc:"昵称/用户名/用户ID"`
	Status      int      `json:"status" dc:"状态: 1有效 2失效"`
}

func (in *TgChannelMemberListInp) Filter(ctx context.Context) error {
	_ = ctx
	in.ChannelId = strings.TrimSpace(in.ChannelId)
	in.Keyword = strings.TrimSpace(in.Keyword)
	in.Role = normalizeTgChannelMemberRole(in.Role)
	in.Roles = normalizeTgChannelMemberRoles(append(in.Roles, in.Role))
	if in.TgAccountId <= 0 {
		return gerror.New("请选择TG账号")
	}
	return nil
}

type TgChannelMemberModel struct {
	Id                  int64       `json:"id" dc:"ID"`
	TenantId            int64       `json:"tenantId" dc:"租户ID"`
	TgAccountId         int64       `json:"tgAccountId" dc:"TG账号ID"`
	ChannelId           string      `json:"channelId" dc:"频道ID"`
	ChannelTitle        string      `json:"channelTitle" dc:"频道名称"`
	UserId              int64       `json:"userId" dc:"用户ID"`
	AccessHash          string      `json:"accessHash" dc:"用户AccessHash"`
	DisplayName         string      `json:"displayName" dc:"显示名称"`
	FirstName           string      `json:"firstName" dc:"名"`
	LastName            string      `json:"lastName" dc:"姓"`
	Username            string      `json:"username" dc:"用户名"`
	Phone               string      `json:"phone" dc:"手机号"`
	ParticipantRole     string      `json:"participantRole" dc:"角色: creator/admin/member"`
	ParticipantRoleText string      `json:"participantRoleText" dc:"角色文本"`
	IsBot               int         `json:"isBot" dc:"是否机器人"`
	IsPremium           int         `json:"isPremium" dc:"是否Premium"`
	Status              int         `json:"status" dc:"状态: 1有效 2失效"`
	LastSyncTaskId      int64       `json:"lastSyncTaskId" dc:"最后同步任务ID"`
	LastSyncedAt        *gtime.Time `json:"lastSyncedAt" dc:"最后同步时间"`
	CreatedAt           *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt           *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type TgChannelMemberExportModel struct {
	TgChannelMemberModel
	ChannelTitle string `json:"channelTitle" dc:"频道名称"`
}

type TgChannelMemberSyncStartInp struct {
	TgAccountId int64  `json:"tgAccountId" v:"required|min:1#请选择TG账号|请选择TG账号" dc:"TG账号ID"`
	ChannelId   string `json:"channelId" v:"required#请选择频道/群聊" dc:"频道ID"`
}

type TgChannelMemberSyncViewInp struct {
	Id int64 `json:"id" v:"required|min:1#任务ID不能为空|任务ID不能为空" dc:"任务ID"`
}

type TgChannelMemberSyncCancelInp struct {
	Id int64 `json:"id" v:"required|min:1#任务ID不能为空|任务ID不能为空" dc:"任务ID"`
}

type TgChannelMemberSyncModel struct {
	Id              int64       `json:"id" dc:"任务ID"`
	TenantId        int64       `json:"tenantId" dc:"租户ID"`
	TgAccountId     int64       `json:"tgAccountId" dc:"TG账号ID"`
	ChannelId       string      `json:"channelId" dc:"频道ID"`
	ChannelTitle    string      `json:"channelTitle" dc:"频道名称"`
	ChannelUsername string      `json:"channelUsername" dc:"频道用户名"`
	Status          string      `json:"status" dc:"状态"`
	Stage           string      `json:"stage" dc:"阶段"`
	ProgressTotal   int         `json:"progressTotal" dc:"总进度"`
	ProgressDone    int         `json:"progressDone" dc:"已完成"`
	AdminTotal      int         `json:"adminTotal" dc:"管理员总数"`
	AdminDone       int         `json:"adminDone" dc:"管理员已完成"`
	MemberTotal     int         `json:"memberTotal" dc:"成员总数"`
	MemberDone      int         `json:"memberDone" dc:"成员已完成"`
	UpsertedCount   int         `json:"upsertedCount" dc:"写入数量"`
	RemovedCount    int         `json:"removedCount" dc:"失效数量"`
	ErrorMessage    string      `json:"errorMessage" dc:"错误信息"`
	Progress        int         `json:"progress" dc:"百分比"`
	StageText       string      `json:"stageText" dc:"阶段文本"`
	StatusText      string      `json:"statusText" dc:"状态文本"`
	CreatedAt       *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt       *gtime.Time `json:"updatedAt" dc:"更新时间"`
	StartedAt       *gtime.Time `json:"startedAt" dc:"开始时间"`
	FinishedAt      *gtime.Time `json:"finishedAt" dc:"完成时间"`
	CanceledAt      *gtime.Time `json:"canceledAt" dc:"取消时间"`
}

func normalizeTgChannelMemberRole(role string) string {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case "creator", "admin", "member":
		return strings.TrimSpace(strings.ToLower(role))
	default:
		return ""
	}
}

func normalizeTgChannelMemberRoles(values []string) []string {
	roles := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		role := normalizeTgChannelMemberRole(value)
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	return roles
}
