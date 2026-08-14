package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

type sAccountTasks struct{}

func init() { collectorservice.RegisterAccountTasks(&sAccountTasks{}) }

func (s *sAccountTasks) Submit(ctx context.Context, in *sysin.AccountTaskSubmit) (int64, error) {
	if in == nil || in.TenantID <= 0 || in.AccountID <= 0 {
		return 0, gerror.New("Telegram账号任务归属不完整")
	}
	in.TaskType = strings.TrimSpace(in.TaskType)
	in.TaskKey = strings.TrimSpace(in.TaskKey)
	if in.TaskType == "" || in.TaskKey == "" {
		return 0, gerror.New("Telegram账号任务参数不完整")
	}
	if err := validateAccountTaskSubmit(in); err != nil {
		return 0, err
	}
	if in.MaxAttempts <= 0 {
		in.MaxAttempts = 5
	}
	columns := dao.TgCollectorAccountTask.Columns()
	now := gtime.Now()
	var nextRunAt *gtime.Time
	if in.NextRunAt != nil {
		nextRunAt = gtime.New(*in.NextRunAt)
	}
	data := g.Map{
		columns.TenantId: in.TenantID, columns.AccountId: in.AccountID, columns.TaskType: in.TaskType,
		columns.TaskKey: in.TaskKey, columns.Priority: in.Priority, columns.Status: sysin.AccountTaskStatusPending,
		columns.HistoryTaskId: in.HistoryTaskID, columns.MediaOwnerAccountId: in.MediaOwnerAccountID,
		columns.MaxAttempts: in.MaxAttempts, columns.NextRunAt: nextRunAt, columns.CreatedAt: now, columns.UpdatedAt: now,
	}
	if in.Media != nil {
		fillAccountTaskMediaData(data, in.Media)
	}
	_, err := dao.TgCollectorAccountTask.Ctx(ctx).Data(data).OnConflict(columns.TenantId + "," + columns.TaskKey).
		OnDuplicate(g.Map{columns.TaskKey: conflictIncomingColumn(ctx, columns.TaskKey)}).
		Save()
	if err != nil {
		return 0, gerror.Wrap(err, "保存Telegram账号任务失败")
	}
	var row entity.TgCollectorAccountTask
	if err = dao.TgCollectorAccountTask.Ctx(ctx).
		Fields(columns.Id).
		Where(columns.TenantId, in.TenantID).
		Where(columns.TaskKey, in.TaskKey).
		Scan(&row); err != nil {
		return 0, gerror.Wrap(err, "读取Telegram账号任务失败")
	}
	return row.Id, nil
}

func validateAccountTaskSubmit(in *sysin.AccountTaskSubmit) error {
	switch in.TaskType {
	case sysin.AccountTaskTypeHistoryPage:
		if in.HistoryTaskID <= 0 {
			return gerror.New("Telegram历史采集任务ID无效")
		}
	case sysin.AccountTaskTypeMediaDownload:
		if in.MediaOwnerAccountID <= 0 || in.Media == nil || strings.TrimSpace(in.Media.FileID) == "" {
			return gerror.New("Telegram媒体下载任务参数不完整")
		}
	case sysin.AccountTaskTypeUsernameResolveDiagnostic:
	case sysin.AccountTaskTypeDialogCacheRefresh:
	default:
		return gerror.Newf("不支持的Telegram账号任务类型：%s", in.TaskType)
	}
	return nil
}

func (s *sAccountTasks) Get(ctx context.Context, taskID int64) (*sysin.AccountTask, error) {
	if taskID <= 0 {
		return nil, gerror.New("Telegram账号任务ID无效")
	}
	var row entity.TgCollectorAccountTask
	if err := dao.TgCollectorAccountTask.Ctx(ctx).WherePri(taskID).Scan(&row); err != nil {
		return nil, gerror.Wrap(err, "读取Telegram账号任务失败")
	}
	if row.Id <= 0 {
		return nil, gerror.New("Telegram账号任务不存在")
	}
	return accountTaskModel(&row), nil
}

func (s *sAccountTasks) Claim(ctx context.Context, lease *sysin.AccountLease, limit int, ttl time.Duration) ([]*sysin.AccountTask, error) {
	if lease == nil || lease.AccountID <= 0 || lease.Epoch <= 0 || strings.TrimSpace(lease.InstanceID) == "" {
		return nil, gerror.New("Telegram账号任务领取租约无效")
	}
	if limit <= 0 {
		limit = 1
	}
	if limit > 20 {
		limit = 20
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	columns := dao.TgCollectorAccountTask.Columns()
	now := gtime.Now()
	var candidates []*entity.TgCollectorAccountTask
	err := dao.TgCollectorAccountTask.Ctx(ctx).
		Fields(columns.Id).
		Where(columns.AccountId, lease.AccountID).
		WhereIn(columns.Status, []string{sysin.AccountTaskStatusPending, sysin.AccountTaskStatusFailedRetry}).
		Where("("+columns.NextRunAt+" IS NULL OR "+columns.NextRunAt+"<=?)", now).
		Where("("+columns.LeaseUntil+" IS NULL OR "+columns.LeaseUntil+"<?)", now).
		OrderDesc(columns.Priority).
		Order(gdb.Raw("CASE WHEN " + columns.TaskType + " = 'media_download' THEN 1 ELSE 0 END DESC")).
		OrderAsc(columns.NextRunAt + "," + columns.Id).
		Limit(limit).
		Scan(&candidates)
	if err != nil {
		return nil, gerror.Wrap(err, "读取待执行Telegram账号任务失败")
	}
	claimed := make([]*sysin.AccountTask, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == nil || candidate.Id <= 0 {
			continue
		}
		result, updateErr := dao.TgCollectorAccountTask.Ctx(ctx).
			WherePri(candidate.Id).
			WhereIn(columns.Status, []string{sysin.AccountTaskStatusPending, sysin.AccountTaskStatusFailedRetry}).
			Where("("+columns.LeaseUntil+" IS NULL OR "+columns.LeaseUntil+"<?)", now).
			Data(g.Map{
				columns.Status: sysin.AccountTaskStatusProcessing, columns.LeaseOwner: lease.InstanceID,
				columns.LeaseEpoch: lease.Epoch, columns.LeaseUntil: now.Add(ttl),
				columns.AttemptCount: gdb.Raw(columns.AttemptCount + "+1"), columns.UpdatedAt: now,
			}).Update()
		if updateErr != nil {
			return nil, gerror.Wrap(updateErr, "领取Telegram账号任务失败")
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			continue
		}
		var row entity.TgCollectorAccountTask
		if updateErr = dao.TgCollectorAccountTask.Ctx(ctx).WherePri(candidate.Id).Scan(&row); updateErr != nil {
			return nil, gerror.Wrap(updateErr, "读取已领取Telegram账号任务失败")
		}
		claimed = append(claimed, accountTaskModel(&row))
	}
	return claimed, nil
}

func (s *sAccountTasks) Complete(ctx context.Context, taskID int64, lease *sysin.AccountLease, result *sysin.AccountMediaDownloadResult) error {
	if taskID <= 0 || lease == nil {
		return gerror.New("Telegram账号任务完成参数无效")
	}
	columns := dao.TgCollectorAccountTask.Columns()
	data := g.Map{
		columns.Status: sysin.AccountTaskStatusCompleted, columns.LeaseOwner: "", columns.LeaseEpoch: 0,
		columns.LeaseUntil: nil, columns.ErrorMessage: "", columns.CompletedAt: gtime.Now(), columns.UpdatedAt: gtime.Now(),
	}
	if result != nil {
		fillAccountTaskMediaData(data, &result.Media)
		data[columns.AttachmentId] = result.AttachmentID
		data[columns.FileUrl] = result.FileURL
		data[columns.StoragePath] = result.StoragePath
		data[columns.ResultErrorCode] = result.ErrorCode
		if result.ErrorMessage != "" {
			data[columns.ErrorMessage] = result.ErrorMessage
		}
	}
	updated, err := dao.TgCollectorAccountTask.Ctx(ctx).
		WherePri(taskID).
		Where(columns.Status, sysin.AccountTaskStatusProcessing).
		Where(columns.LeaseOwner, lease.InstanceID).
		Where(columns.LeaseEpoch, lease.Epoch).
		Data(data).Update()
	if err != nil {
		return gerror.Wrap(err, "完成Telegram账号任务失败")
	}
	affected, _ := updated.RowsAffected()
	if affected == 0 {
		return gerror.New("Telegram账号任务租约已失效，拒绝提交完成结果")
	}
	return nil
}

func (s *sAccountTasks) Fail(ctx context.Context, in *sysin.AccountTaskFailure) error {
	if in == nil || in.TaskID <= 0 || in.Lease == nil {
		return gerror.New("Telegram账号任务失败参数无效")
	}
	columns := dao.TgCollectorAccountTask.Columns()
	var row entity.TgCollectorAccountTask
	if err := dao.TgCollectorAccountTask.Ctx(ctx).WherePri(in.TaskID).Scan(&row); err != nil {
		return gerror.Wrap(err, "读取Telegram账号任务重试状态失败")
	}
	status := sysin.AccountTaskStatusFailedRetry
	var nextRunAt any = gtime.Now().Add(in.RetryDelay)
	if in.RetryDelay <= 0 {
		nextRunAt = gtime.Now().Add(5 * time.Second)
	}
	if row.AttemptCount >= row.MaxAttempts {
		status = sysin.AccountTaskStatusDead
		nextRunAt = nil
	}
	errorMessage := "Telegram账号任务执行失败"
	if in.Cause != nil {
		errorMessage = in.Cause.Error()
	}
	updated, err := dao.TgCollectorAccountTask.Ctx(ctx).
		WherePri(in.TaskID).
		Where(columns.Status, sysin.AccountTaskStatusProcessing).
		Where(columns.LeaseOwner, in.Lease.InstanceID).
		Where(columns.LeaseEpoch, in.Lease.Epoch).
		Data(g.Map{
			columns.Status: status, columns.NextRunAt: nextRunAt, columns.LeaseOwner: "",
			columns.LeaseEpoch: 0, columns.LeaseUntil: nil, columns.ErrorMessage: errorMessage, columns.UpdatedAt: gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新Telegram账号任务失败状态失败")
	}
	affected, _ := updated.RowsAffected()
	if affected == 0 {
		return gerror.New("Telegram账号任务租约已失效，拒绝提交失败结果")
	}
	return nil
}

func (s *sAccountTasks) RecoverExpired(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	columns := dao.TgCollectorAccountTask.Columns()
	now := gtime.Now()
	var rows []*entity.TgCollectorAccountTask
	if err := dao.TgCollectorAccountTask.Ctx(ctx).
		Fields(columns.Id, columns.AttemptCount, columns.MaxAttempts).
		Where(columns.Status, sysin.AccountTaskStatusProcessing).
		WhereLT(columns.LeaseUntil, now).
		OrderAsc(columns.LeaseUntil + "," + columns.Id).
		Limit(limit).
		Scan(&rows); err != nil {
		return 0, gerror.Wrap(err, "读取超时Telegram账号任务失败")
	}
	recovered := 0
	for _, row := range rows {
		status := sysin.AccountTaskStatusFailedRetry
		var nextRunAt any = now
		if row.AttemptCount >= row.MaxAttempts {
			status = sysin.AccountTaskStatusDead
			nextRunAt = nil
		}
		result, err := dao.TgCollectorAccountTask.Ctx(ctx).
			WherePri(row.Id).
			Where(columns.Status, sysin.AccountTaskStatusProcessing).
			WhereLT(columns.LeaseUntil, now).
			Data(g.Map{
				columns.Status: status, columns.NextRunAt: nextRunAt, columns.LeaseOwner: "",
				columns.LeaseEpoch: 0, columns.LeaseUntil: nil,
				columns.ErrorMessage: "账号任务执行租约超时，已自动恢复", columns.UpdatedAt: now,
			}).Update()
		if err != nil {
			return recovered, gerror.Wrap(err, "恢复超时Telegram账号任务失败")
		}
		affected, _ := result.RowsAffected()
		recovered += int(affected)
	}
	return recovered, nil
}

func (s *sAccountTasks) ActiveStatusStats(ctx context.Context) ([]sysin.AccountTaskStatusStat, error) {
	columns := dao.TgCollectorAccountTask.Columns()
	type statusRow struct {
		Status          string      `json:"status"`
		Total           int64       `json:"total"`
		OldestCreatedAt *gtime.Time `json:"oldestCreatedAt"`
		OldestUpdatedAt *gtime.Time `json:"oldestUpdatedAt"`
	}
	var rows []*statusRow
	if err := dao.TgCollectorAccountTask.Ctx(ctx).
		Fields(
			columns.Status,
			"COUNT(1) AS total",
			"MIN("+columns.CreatedAt+") AS oldest_created_at",
			"MIN("+columns.UpdatedAt+") AS oldest_updated_at",
		).
		WhereIn(columns.Status, []string{
			sysin.AccountTaskStatusPending,
			sysin.AccountTaskStatusProcessing,
			sysin.AccountTaskStatusFailedRetry,
		}).
		Group(columns.Status).
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "统计Telegram账号任务状态失败")
	}
	stats := make([]sysin.AccountTaskStatusStat, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		stat := sysin.AccountTaskStatusStat{Status: row.Status, Total: row.Total}
		if row.OldestCreatedAt != nil {
			value := row.OldestCreatedAt.Time
			stat.OldestCreatedAt = &value
		}
		if row.OldestUpdatedAt != nil {
			value := row.OldestUpdatedAt.Time
			stat.OldestUpdatedAt = &value
		}
		stats = append(stats, stat)
	}
	return stats, nil
}

func accountTaskModel(row *entity.TgCollectorAccountTask) *sysin.AccountTask {
	if row == nil {
		return nil
	}
	model := &sysin.AccountTask{
		ID: row.Id, TenantID: row.TenantId, AccountID: row.AccountId, TaskType: row.TaskType,
		TaskKey: row.TaskKey, Priority: row.Priority, Status: row.Status,
		AttemptCount: row.AttemptCount, MaxAttempts: row.MaxAttempts,
		LeaseOwner: row.LeaseOwner, LeaseEpoch: row.LeaseEpoch,
		ErrorMessage: row.ErrorMessage,
	}
	model.HistoryTaskID = row.HistoryTaskId
	model.MediaOwnerAccountID = row.MediaOwnerAccountId
	model.Media = accountTaskMediaModel(row)
	model.MediaResult = sysin.AccountMediaDownloadResult{AttachmentID: row.AttachmentId, FileURL: row.FileUrl, StoragePath: row.StoragePath, Media: model.Media, ErrorCode: row.ResultErrorCode, ErrorMessage: row.ErrorMessage}
	if row.LeaseUntil != nil {
		value := row.LeaseUntil.Time
		model.LeaseUntil = &value
	}
	if row.NextRunAt != nil {
		value := row.NextRunAt.Time
		model.NextRunAt = &value
	}
	if row.CreatedAt != nil {
		value := row.CreatedAt.Time
		model.CreatedAt = &value
	}
	if row.CompletedAt != nil {
		value := row.CompletedAt.Time
		model.CompletedAt = &value
	}
	return model
}

func fillAccountTaskMediaData(data g.Map, media *sysin.CollectorMediaItem) {
	if media == nil {
		return
	}
	columns := dao.TgCollectorAccountTask.Columns()
	data[columns.MediaType], data[columns.MediaPurpose], data[columns.SourceFileId] = media.Type, media.Purpose, media.FileID
	data[columns.FileUrl], data[columns.StoragePath], data[columns.PosterUrl] = media.FileURL, media.StoragePath, media.PosterURL
	data[columns.FileMd5], data[columns.FilePhash], data[columns.SourceKind] = media.FileMD5, media.FilePHash, media.SourceKind
	data[columns.SourceMediaId], data[columns.SourceAccessHash], data[columns.SourceFileReference] = media.SourceMediaID, media.SourceAccessHash, media.SourceFileReference
	data[columns.SourceThumbSize], data[columns.SourceMimeType], data[columns.SourceDcId], data[columns.SourceSize] = media.SourceThumbSize, media.SourceMimeType, media.SourceDCID, media.SourceSize
	data[columns.DebugMetaText] = media.DebugMetaJSON
}

func accountTaskMediaModel(row *entity.TgCollectorAccountTask) sysin.CollectorMediaItem {
	return sysin.CollectorMediaItem{Type: row.MediaType, Purpose: row.MediaPurpose, FileID: row.SourceFileId, FileURL: row.FileUrl, StoragePath: row.StoragePath, PosterURL: row.PosterUrl, FileMD5: row.FileMd5, FilePHash: row.FilePhash, SourceKind: row.SourceKind, SourceMediaID: row.SourceMediaId, SourceAccessHash: row.SourceAccessHash, SourceFileReference: []byte(row.SourceFileReference), SourceThumbSize: row.SourceThumbSize, SourceMimeType: row.SourceMimeType, SourceDCID: row.SourceDcId, SourceSize: row.SourceSize, DebugMetaJSON: row.DebugMetaText}
}
