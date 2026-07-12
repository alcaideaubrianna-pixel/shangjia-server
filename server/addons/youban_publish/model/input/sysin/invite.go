package sysin

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/model/input/form"
)

type InviteInfoModel struct {
	Code           string      `json:"code" dc:"邀请码"`
	Source         string      `json:"source" dc:"来源"`
	ExpiresAt      *gtime.Time `json:"expiresAt" dc:"过期时间"`
	InviteCount    int         `json:"inviteCount" dc:"已邀请数量"`
	UsedCount      int         `json:"usedCount" dc:"已使用数量"`
	ExpireDays     int         `json:"expireDays" dc:"有效期天数"`
	InviteUrl      string      `json:"inviteUrl" dc:"注册链接"`
	WebInviteHint  string      `json:"webInviteHint" dc:"网页提示"`
	CanGenerateBot bool        `json:"canGenerateBot" dc:"是否可生成"`
}

type InviteListInp struct {
	form.PageReq
	Keyword string `json:"keyword" dc:"邀请码/账号/租户关键词"`
	Source  string `json:"source" dc:"来源:web/bot"`
	Status  string `json:"status" dc:"状态:active/used/expired"`
}

type InviteModel struct {
	Id                int64       `json:"id" dc:"ID"`
	Code              string      `json:"code" dc:"邀请码"`
	Source            string      `json:"source" dc:"来源"`
	InviterApp        string      `json:"inviterApp" dc:"邀请来源应用"`
	InviterTenantId   int64       `json:"inviterTenantId" dc:"邀请人租户ID"`
	InviterTenantName string      `json:"inviterTenantName" dc:"邀请人租户名称"`
	InviterAccountId  int64       `json:"inviterAccountId" dc:"邀请人账号ID"`
	InviterUsername   string      `json:"inviterUsername" dc:"邀请人账号"`
	InviterNickname   string      `json:"inviterNickname" dc:"邀请人昵称"`
	UsedTenantId      int64       `json:"usedTenantId" dc:"注册租户ID"`
	UsedTenantName    string      `json:"usedTenantName" dc:"注册租户名称"`
	UsedAccountId     int64       `json:"usedAccountId" dc:"注册账号ID"`
	UsedAccountName   string      `json:"usedAccountName" dc:"注册账号"`
	Status            string      `json:"status" dc:"状态"`
	ExpiresAt         *gtime.Time `json:"expiresAt" dc:"过期时间"`
	UsedAt            *gtime.Time `json:"usedAt" dc:"使用时间"`
	CreatedAt         *gtime.Time `json:"createdAt" dc:"创建时间"`
}

type InviteCreateInp struct {
	Source   string `json:"source" dc:"来源:web/bot"`
	ForceNew int    `json:"forceNew" dc:"是否强制生成新邀请码：1是 0否"`
}

type InviteCreateModel struct {
	Code      string      `json:"code" dc:"邀请码"`
	Source    string      `json:"source" dc:"来源"`
	ExpiresAt *gtime.Time `json:"expiresAt" dc:"过期时间"`
	InviteUrl string      `json:"inviteUrl" dc:"注册链接"`
}

func (in *InviteListInp) Filter(ctx context.Context) error {
	_ = ctx
	in.Source = strings.TrimSpace(strings.ToLower(in.Source))
	in.Status = strings.TrimSpace(strings.ToLower(in.Status))
	if in.Source != "" && in.Source != "web" && in.Source != "bot" {
		return gerror.New("来源不合法")
	}
	if in.Status != "" && in.Status != "active" && in.Status != "used" && in.Status != "expired" {
		return gerror.New("状态不合法")
	}
	return nil
}

func (in *InviteCreateInp) Filter(ctx context.Context) error {
	_ = ctx
	in.Source = strings.TrimSpace(strings.ToLower(in.Source))
	if in.Source == "" {
		in.Source = "web"
	}
	if in.Source != "web" && in.Source != "bot" {
		return gerror.New("来源不合法")
	}
	return nil
}
