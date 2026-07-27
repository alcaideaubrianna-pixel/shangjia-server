package sys

import (
	"context"
	"errors"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
)

var errPublishProfileUnavailable = errors.New("资料已不满足循环上架条件")

func (s *sSysPublish) MyProfilePublish(ctx context.Context, in *sysin.ProfileViewInp) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || !hasProfileSelector(in.Id, in.Uuid) {
		return gerror.New("资料UUID不能为空")
	}
	profileId, err := s.resolveProfileId(ctx, in.Id, in.Uuid, account.TenantId, account.Id)
	if err != nil {
		return err
	}
	taskId, alreadyPublished, err := s.createProfilePublishEvent(ctx, profileId, account.TenantId, account.Id)
	if err != nil || alreadyPublished {
		return err
	}
	return s.submitTask(ctx, taskId, account.Id)
}

func (s *sSysPublish) AdminProfilePublish(ctx context.Context, in *sysin.ProfileViewInp) error {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || !hasProfileSelector(in.Id, in.Uuid) {
		return gerror.New("资料UUID不能为空")
	}
	view, err := s.AdminProfileView(ctx, in)
	if err != nil {
		return err
	}
	if view == nil || view.Profile == nil {
		return gerror.New("资料不存在或无权操作")
	}
	taskId, alreadyPublished, err := s.createProfilePublishEvent(ctx, view.Profile.Id, view.Profile.TenantId, view.Profile.AccountId)
	if err != nil || alreadyPublished {
		return err
	}
	return s.submitTaskByTenant(ctx, taskId, account.TenantId, account.Id)
}

// createProfilePublishEvent is the only profile flow allowed to create a
// PublishTask. Profile and task_id=0 media are the editable source; the task is
// an immutable snapshot for one publish operation.
func (s *sSysPublish) createProfilePublishEvent(ctx context.Context, profileId int64, tenantId int64, accountId int64) (taskId int64, alreadyPublished bool, err error) {
	if err = s.cleanupProfileDownMessagesBeforePublish(ctx, profileId, tenantId); err != nil {
		return 0, false, err
	}
	return s.createProfilePublishSnapshot(ctx, profileId, tenantId, accountId, profilePublishSnapshotOptions{ReuseActive: true, SkipIfOnline: true})
}

type profilePublishSnapshotOptions struct {
	ChannelIds      []int64
	ClientRequestId string
	ReuseActive     bool
	SkipIfOnline    bool
	RequireOnline   bool
}

func (s *sSysPublish) createProfilePublishSnapshot(ctx context.Context, profileId int64, tenantId int64, accountId int64, options profilePublishSnapshotOptions) (taskId int64, noSubmit bool, err error) {
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		profile, lockErr := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).
			Where("id", profileId).WhereNull("deleted_at").LockUpdate().One()
		if lockErr != nil {
			return gerror.Wrap(lockErr, "锁定待发布资料失败")
		}
		if profile.IsEmpty() {
			if options.RequireOnline {
				return errPublishProfileUnavailable
			}
			return gerror.New("资料不存在或无权操作")
		}
		if options.RequireOnline && (profile["status"].Int() != 1 || profile["visibility"].String() != consts.ContentVisibilityPublic) {
			return errPublishProfileUnavailable
		}
		stateMod := tx.Model(publishProfileStateTable).Ctx(ctx).
			Where("profile_id", profileId).Where("tenant_id", tenantId).WhereNull("deleted_at")
		if accountId > 0 {
			stateMod = stateMod.Where("account_id", accountId)
		}
		state, stateErr := stateMod.One()
		if stateErr != nil {
			return gerror.Wrap(stateErr, "读取资料发布配置失败")
		}
		if state.IsEmpty() {
			if options.RequireOnline {
				return errPublishProfileUnavailable
			}
			return gerror.New("资料不存在或无权操作")
		}
		channelIds := uniqueIds(options.ChannelIds)
		channelJSON := state["channel_id_json"].String()
		if len(channelIds) > 0 {
			channelJSON, err = encodeBotIds(channelIds)
			if err != nil {
				return gerror.Wrap(err, "编码发布频道失败")
			}
		} else {
			channelIds = decodeInt64JSON(channelJSON)
		}
		if len(channelIds) == 0 {
			return gerror.New("请选择至少一个有效的上架频道")
		}
		if options.ClientRequestId != "" {
			existing, existingErr := tx.Model(publishTaskTable).Ctx(ctx).
				Where("tenant_id", tenantId).Where("client_request_id", options.ClientRequestId).
				WhereNull("deleted_at").One()
			if existingErr != nil {
				return gerror.Wrap(existingErr, "读取幂等发布事件失败")
			}
			if !existing.IsEmpty() {
				taskId = existing["id"].Int64()
				// The event may have been committed before its TG jobs were
				// created. Let the common submit workflow recover that gap.
				return nil
			}
		}
		if options.ReuseActive {
			active, activeErr := tx.Model(publishTaskTable).Ctx(ctx).
				Where("profile_id", profileId).
				Where("tenant_id", tenantId).
				Where("account_id", state["account_id"].Int64()).
				WhereIn("status", []string{sysin.PublishTaskStatusPending, sysin.PublishTaskStatusPublishing}).
				WhereNull("deleted_at").OrderDesc("id").One()
			if activeErr != nil {
				return gerror.Wrap(activeErr, "读取正在发布的事件失败")
			}
			if !active.IsEmpty() {
				taskId = active["id"].Int64()
				return nil
			}
		}
		if options.SkipIfOnline && profile["status"].Int() == 1 {
			noSubmit = true
			return nil
		}
		now := gtime.Now()
		mediaCount, countErr := tx.Model(publishMediaTable).Ctx(ctx).
			Where("profile_id", profileId).Where("task_id", 0).WhereNull("deleted_at").Count()
		if countErr != nil {
			return gerror.Wrap(countErr, "统计资料媒体失败")
		}
		taskId, err = tx.Model(publishTaskTable).Ctx(ctx).Data(g.Map{
			"tenant_id": tenantId, "merchant_id": tenantId,
			"account_id": state["account_id"].Int64(), "profile_id": profileId,
			"client_request_id": options.ClientRequestId,
			"title":             profile["title"].String(), "province": profile["province"].String(),
			"city": profile["city"].String(), "plain_text": profile["plain_text"].String(),
			"media_count": mediaCount, "channel_id_json": channelJSON,
			"customer_remark":   state["customer_remark"].String(),
			"anti_scan_enabled": state["anti_scan_enabled"].Int(),
			"tg_push_enabled":   1, "tg_status": "pending",
			"status": sysin.PublishTaskStatusPending, "published_at": profileStatePublishAt(state),
			"created_by": contexts.GetUserId(ctx), "updated_by": contexts.GetUserId(ctx),
			"created_at": now, "updated_at": now,
		}).InsertAndGetId()
		if err != nil {
			return gerror.Wrap(err, "创建资料发布事件失败")
		}
		return s.cloneProfileTaskMedia(ctx, tx, 0, taskId, profileId, tenantId, state["account_id"].Int64(), contexts.GetUserId(ctx))
	})
	return taskId, noSubmit, err
}
