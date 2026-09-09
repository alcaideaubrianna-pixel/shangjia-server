package sys

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const botMediaRepairBatchLimit = 1000

func (s *sSysPublish) AIOpsQueueBotMediaRepair(ctx context.Context, profileIds []int64, limit int) ([]int64, error) {
	profileIds = uniqueIds(profileIds)
	if limit <= 0 || limit > botMediaRepairBatchLimit {
		limit = botMediaRepairBatchLimit
	}
	model := g.DB().Model(publishMediaTable+" m").Safe().Ctx(ctx).
		Fields("DISTINCT m.profile_id").
		InnerJoin("hg_content_profile p", "p.id=m.profile_id AND p.deleted_at IS NULL").
		InnerJoin(publishProfileStateTable+" ps", "ps.profile_id=m.profile_id AND ps.deleted_at IS NULL").
		Where("p.source_type", "youban_publish").
		Where("ps.status", 1).
		Where("m.status", 1).
		WhereNull("m.deleted_at").
		Where("COALESCE(m.storage_path,'')", "").
		WhereLike("m.file_url", "https://api.telegram.org/file/bot%")
	if len(profileIds) > 0 {
		model = model.WhereIn("m.profile_id", profileIds)
	}
	var rows []struct {
		ProfileId int64 `json:"profile_id"`
	}
	if err := model.OrderAsc("m.profile_id").Limit(limit).Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "扫描Bot历史媒体失败")
	}
	queued := make([]int64, 0, len(rows))
	for _, row := range rows {
		if err := s.enqueueBotMediaRepairTask(ctx, row.ProfileId); err != nil {
			return queued, gerror.Wrapf(err, "提交Bot媒体恢复任务失败 profileId:%d", row.ProfileId)
		}
		queued = append(queued, row.ProfileId)
	}
	g.Log().Info(ctx, "Bot历史媒体恢复已入队", g.Map{"profiles": len(queued), "profileIds": queued})
	return queued, nil
}

func (s *sSysPublish) enqueueBotMediaRepairTask(ctx context.Context, profileId int64) error {
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(botMediaRepairQueuePayload{ProfileId: profileId})
	if err != nil {
		return err
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeBotMediaRepair, body),
		asynq.Queue(tgQueueNameBackground), asynq.MaxRetry(5), asynq.Timeout(30*time.Minute), asynq.Unique(24*time.Hour))
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *sSysPublish) handleBotMediaRepairTask(ctx context.Context, task *asynq.Task) error {
	var payload botMediaRepairQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return gerror.Wrap(err, "解析Bot媒体恢复任务失败")
	}
	if payload.ProfileId <= 0 {
		return gerror.New("Bot媒体恢复任务缺少profileId")
	}
	result, err := s.rebuildBotProfileMedia(ctx, []int64{payload.ProfileId}, false)
	if err != nil {
		return err
	}
	complete, err := collectProfileMediaComplete(ctx, payload.ProfileId)
	if err != nil {
		return err
	}
	if !complete {
		return gerror.Newf("Bot资料媒体恢复后仍不完整 profileId:%d", payload.ProfileId)
	}
	if result.Requeued > 0 {
		if _, err = s.AIOpsRepublishProfiles(ctx, &sysin.ProfileStatusInp{Ids: []int64{payload.ProfileId}, Status: 1}); err != nil {
			return err
		}
	}
	g.Log().Info(ctx, "Bot历史媒体恢复完成", g.Map{"profileId": payload.ProfileId, "mediaRecovered": result.Requeued > 0})
	return nil
}
