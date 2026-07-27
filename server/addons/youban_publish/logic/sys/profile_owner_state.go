package sys

import (
	"context"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"

	"hotgo/internal/library/contexts"
)

func (s *sSysPublish) profileState(ctx context.Context, profileId int64, tenantId int64, accountId int64) (gdb.Record, error) {
	mod := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	row, err := mod.One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取资料归属配置失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("资料不存在或无权操作")
	}
	return row, nil
}

func (s *sSysPublish) upsertProfileStateTx(ctx context.Context, tx gdb.TX, profileId int64, tenantId int64, accountId int64, channelJSON string, customerRemark string, antiScanEnabled int, publishAt *gtime.Time) error {
	now := gtime.Now()
	data := g.Map{
		"tenant_id": tenantId, "account_id": accountId, "profile_id": profileId,
		"channel_id_json": channelJSON, "customer_remark": customerRemark,
		"anti_scan_enabled": antiScanEnabled, "publish_at": publishAt,
		"created_by": contexts.GetUserId(ctx), "updated_by": contexts.GetUserId(ctx),
		"created_at": now, "updated_at": now, "deleted_at": nil,
	}
	_, err := tx.Model(publishProfileStateTable).Ctx(ctx).Data(data).
		OnConflict("profile_id").OnDuplicateEx("id", "created_by", "created_at").Save()
	if err != nil {
		return gerror.Wrap(err, "保存资料发布配置失败")
	}
	return nil
}

func profileMediaOwner(state gdb.Record) gdb.Record {
	return gdb.Record{
		"id":            gvar.New(nil),
		"tenant_id":     gvar.New(state["tenant_id"].Int64()),
		"account_id":    gvar.New(state["account_id"].Int64()),
		"profile_id":    gvar.New(state["profile_id"].Int64()),
		"profile_media": gvar.New(true),
	}
}

func profileStatePublishAt(state gdb.Record) *gtime.Time {
	if state.IsEmpty() || state["publish_at"].IsNil() {
		return nil
	}
	return gtime.New(gconv.Time(state["publish_at"].Interface()))
}
