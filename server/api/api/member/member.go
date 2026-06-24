// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package member

import (
	"context"

	"hotgo/api/api/member/v1"
)

type IMemberV1 interface {
	GetIdByCode(ctx context.Context, req *v1.GetIdByCodeReq) (res *v1.GetIdByCodeRes, err error)
	Register(ctx context.Context, req *v1.RegisterReq) (res *v1.RegisterRes, err error)
	AccountLogin(ctx context.Context, req *v1.AccountLoginReq) (res *v1.AccountLoginRes, err error)
	Info(ctx context.Context, req *v1.InfoReq) (res *v1.InfoRes, err error)
	MemberVipPay(ctx context.Context, req *v1.MemberVipPayReq) (res *v1.MemberVipPayRes, err error)
	MemberVipConfig(ctx context.Context, req *v1.MemberVipConfigReq) (res *v1.MemberVipConfigRes, err error)
	UpdateProfile(ctx context.Context, req *v1.UpdateProfileReq) (res *v1.UpdateProfileRes, err error)
	UpdatePassword(ctx context.Context, req *v1.UpdatePasswordReq) (res *v1.UpdatePasswordRes, err error)
	Settings(ctx context.Context, req *v1.SettingsReq) (res *v1.SettingsRes, err error)
	UpdateSettings(ctx context.Context, req *v1.UpdateSettingsReq) (res *v1.UpdateSettingsRes, err error)
	FavoriteList(ctx context.Context, req *v1.FavoriteListReq) (res *v1.FavoriteListRes, err error)
	BlockedProfileList(ctx context.Context, req *v1.BlockedProfileListReq) (res *v1.BlockedProfileListRes, err error)
	FavoriteToggle(ctx context.Context, req *v1.FavoriteToggleReq) (res *v1.FavoriteToggleRes, err error)
	FavoriteIds(ctx context.Context, req *v1.FavoriteIdsReq) (res *v1.FavoriteIdsRes, err error)
	ProfileRelation(ctx context.Context, req *v1.ProfileRelationReq) (res *v1.ProfileRelationRes, err error)
	BlockProfile(ctx context.Context, req *v1.BlockProfileReq) (res *v1.BlockProfileRes, err error)
	UnblockProfile(ctx context.Context, req *v1.UnblockProfileReq) (res *v1.UnblockProfileRes, err error)
	RejectProfile(ctx context.Context, req *v1.RejectProfileReq) (res *v1.RejectProfileRes, err error)
	ImmersiveProfileList(ctx context.Context, req *v1.ImmersiveProfileListReq) (res *v1.ImmersiveProfileListRes, err error)
	TraceList(ctx context.Context, req *v1.TraceListReq) (res *v1.TraceListRes, err error)
	TraceRecord(ctx context.Context, req *v1.TraceRecordReq) (res *v1.TraceRecordRes, err error)
	Stats(ctx context.Context, req *v1.StatsReq) (res *v1.StatsRes, err error)
	Agreement(ctx context.Context, req *v1.AgreementReq) (res *v1.AgreementRes, err error)
	ShareCreate(ctx context.Context, req *v1.ShareCreateReq) (res *v1.ShareCreateRes, err error)
	ShareOpen(ctx context.Context, req *v1.ShareOpenReq) (res *v1.ShareOpenRes, err error)
}
