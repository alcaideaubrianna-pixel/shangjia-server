package sys

import (
	"context"
	"math"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const (
	tgChannelMemberStatusPending  = "pending"
	tgChannelMemberStatusRunning  = "running"
	tgChannelMemberStatusSuccess  = "success"
	tgChannelMemberStatusFailed   = "failed"
	tgChannelMemberStatusCanceled = "canceled"

	tgChannelMemberStageCreated = "created"
	tgChannelMemberStageAdmins  = "admins"
	tgChannelMemberStageMembers = "members"
	tgChannelMemberStageFinish  = "finished"
)

func (s *sSysPublish) AdminChannelMemberSyncStart(ctx context.Context, in *sysin.TgChannelMemberSyncStartInp) (*sysin.TgChannelMemberSyncModel, error) {
	if err := ensureTgChannelMemberSchema(ctx); err != nil {
		return nil, err
	}
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || in.TgAccountId <= 0 {
		return nil, gerror.New("请选择TG账号")
	}
	in.ChannelId = strings.TrimSpace(in.ChannelId)
	if in.ChannelId == "" {
		return nil, gerror.New("请选择频道/群聊")
	}
	if err = s.ensureTgAccountBelongsAccount(ctx, in.TgAccountId, account.TenantId, account.Id); err != nil {
		return nil, err
	}
	cache, err := s.tgChannelCacheByChannelId(ctx, account.TenantId, in.TgAccountId, in.ChannelId)
	if err != nil {
		return nil, err
	}
	if running, err := s.channelMemberRunningTask(ctx, account.TenantId, in.TgAccountId, cache.ChannelId); err != nil {
		return nil, err
	} else if running != nil {
		return s.AdminChannelMemberSyncView(ctx, &sysin.TgChannelMemberSyncViewInp{Id: running.Id})
	}
	now := gtime.Now()
	id, err := g.DB().Model(publishTgChannelMemberTaskTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id":        account.TenantId,
		"tg_account_id":    in.TgAccountId,
		"channel_id":       cache.ChannelId,
		"channel_title":    strings.TrimSpace(cache.ChannelTitle),
		"channel_username": strings.TrimPrefix(strings.TrimSpace(cache.ChannelUsername), "@"),
		"status":           tgChannelMemberStatusPending,
		"stage":            tgChannelMemberStageCreated,
		"created_at":       now,
		"updated_at":       now,
	}).InsertAndGetId()
	if err != nil {
		return nil, gerror.Wrap(err, "创建频道成员同步任务失败")
	}
	if err = s.markChannelMemberTaskRunning(ctx, id, tgChannelMemberStageAdmins); err != nil {
		return nil, err
	}
	if err = s.enqueueChannelMemberSyncTask(ctx, id, 0); err != nil {
		_ = s.markChannelMemberTaskFailed(ctx, id, err)
		return nil, gerror.Wrap(err, "加入频道成员同步队列失败")
	}
	return s.AdminChannelMemberSyncView(ctx, &sysin.TgChannelMemberSyncViewInp{Id: id})
}

func (s *sSysPublish) AdminChannelMemberSyncView(ctx context.Context, in *sysin.TgChannelMemberSyncViewInp) (*sysin.TgChannelMemberSyncModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.channelMemberSyncTaskView(ctx, in.Id, account.TenantId)
}

func (s *sSysPublish) AdminChannelMemberSyncCancel(ctx context.Context, in *sysin.TgChannelMemberSyncCancelInp) error {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	task, err := s.channelMemberSyncTaskView(ctx, in.Id, account.TenantId)
	if err != nil {
		return err
	}
	if channelMemberTaskTerminal(task.Status) {
		return nil
	}
	_, err = g.DB().Model(publishTgChannelMemberTaskTable).Safe().Ctx(ctx).Where("id", task.Id).Data(g.Map{
		"status":      tgChannelMemberStatusCanceled,
		"stage":       tgChannelMemberStageFinish,
		"canceled_at": gtime.Now(),
		"updated_at":  gtime.Now(),
	}).Update()
	return gerror.Wrap(err, "取消频道成员同步任务失败")
}

func (s *sSysPublish) channelMemberRunningTask(ctx context.Context, tenantId int64, tgAccountId int64, channelId string) (*sysin.TgChannelMemberSyncModel, error) {
	var item *sysin.TgChannelMemberSyncModel
	err := g.DB().Model(publishTgChannelMemberTaskTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("tg_account_id", tgAccountId).
		Where("channel_id", channelId).
		WhereIn("status", []string{tgChannelMemberStatusPending, tgChannelMemberStatusRunning}).
		OrderDesc("id").
		Limit(1).
		Scan(&item)
	if err != nil {
		return nil, gerror.Wrap(err, "读取频道成员同步任务失败")
	}
	return item, nil
}

func (s *sSysPublish) channelMemberSyncTaskView(ctx context.Context, id int64, tenantId int64) (*sysin.TgChannelMemberSyncModel, error) {
	if id <= 0 {
		return nil, gerror.New("任务ID不能为空")
	}
	var item *sysin.TgChannelMemberSyncModel
	err := g.DB().Model(publishTgChannelMemberTaskTable).Safe().Ctx(ctx).
		Where("id", id).
		Where("tenant_id", tenantId).
		Scan(&item)
	if err != nil {
		return nil, gerror.Wrap(err, "读取频道成员同步任务失败")
	}
	if item == nil || item.Id <= 0 {
		return nil, gerror.New("频道成员同步任务不存在")
	}
	fillChannelMemberTaskDisplay(item)
	return item, nil
}

func (s *sSysPublish) markChannelMemberTaskRunning(ctx context.Context, id int64, stage string) error {
	data := g.Map{"status": tgChannelMemberStatusRunning, "stage": stage, "updated_at": gtime.Now()}
	if stage == tgChannelMemberStageAdmins {
		data["started_at"] = gtime.Now()
	}
	_, err := g.DB().Model(publishTgChannelMemberTaskTable).Safe().Ctx(ctx).Where("id", id).Data(data).Update()
	return gerror.Wrap(err, "更新频道成员同步任务失败")
}

func (s *sSysPublish) markChannelMemberTaskFailed(ctx context.Context, id int64, err error) error {
	_, updateErr := g.DB().Model(publishTgChannelMemberTaskTable).Safe().Ctx(ctx).Where("id", id).Data(g.Map{
		"status":        tgChannelMemberStatusFailed,
		"error_message": err.Error(),
		"finished_at":   gtime.Now(),
		"updated_at":    gtime.Now(),
	}).Update()
	return updateErr
}

func channelMemberTaskTerminal(status string) bool {
	switch strings.TrimSpace(status) {
	case tgChannelMemberStatusSuccess, tgChannelMemberStatusFailed, tgChannelMemberStatusCanceled:
		return true
	default:
		return false
	}
}

func fillChannelMemberTaskDisplay(item *sysin.TgChannelMemberSyncModel) {
	if item == nil {
		return
	}
	if item.ProgressTotal > 0 {
		item.Progress = int(math.Round(float64(item.ProgressDone) * 100 / float64(item.ProgressTotal)))
	}
	if item.Progress > 100 {
		item.Progress = 100
	}
	item.StatusText = channelMemberTaskStatusText(item.Status)
	item.StageText = channelMemberTaskStageText(item.Stage)
}

func channelMemberTaskStatusText(status string) string {
	switch status {
	case tgChannelMemberStatusPending:
		return "等待中"
	case tgChannelMemberStatusRunning:
		return "同步中"
	case tgChannelMemberStatusSuccess:
		return "已完成"
	case tgChannelMemberStatusFailed:
		return "失败"
	case tgChannelMemberStatusCanceled:
		return "已取消"
	default:
		return status
	}
}

func channelMemberTaskStageText(stage string) string {
	switch stage {
	case tgChannelMemberStageAdmins:
		return "同步管理员"
	case tgChannelMemberStageMembers:
		return "同步成员"
	case tgChannelMemberStageFinish:
		return "完成"
	default:
		return "准备中"
	}
}

func channelMemberTaskFromRecord(row gdb.Record) *sysin.TgChannelMemberSyncModel {
	if row.IsEmpty() {
		return nil
	}
	var item sysin.TgChannelMemberSyncModel
	_ = row.Struct(&item)
	fillChannelMemberTaskDisplay(&item)
	return &item
}
