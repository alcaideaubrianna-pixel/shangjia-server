package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"
	botservice "hotgo/addons/youban_bot/service"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
)

const profileMediaOperationTable = "hg_youban_publish_profile_media_operation"
const tgTaskTypeProfileMedia = "youban_publish:profile:media_process"

type profileMediaOperationPayload struct {
	OperationId int64 `json:"operationId"`
}

type profileMediaOperationInput struct {
	TenantId     int64                            `json:"tenantId"`
	AccountId    int64                            `json:"accountId"`
	ProfileNo    string                           `json:"profileNo"`
	DisplayMedia []*sysin.MessageTemplateMediaInp `json:"displayMedia"`
	VerifyMedia  []*sysin.MessageTemplateMediaInp `json:"verifyMedia"`
	TraceId      string                           `json:"traceId,omitempty"`
}

// BotProfileQueueMedia durably records the raw Telegram media payload before
// enqueueing work. The Bot request can safely return even if the worker role is
// temporarily unavailable; the recovery scan can enqueue pending rows later.
func (s *sSysPublish) BotProfileQueueMedia(ctx context.Context, tenantId, accountId int64, profileNo string, displayMedia, verifyMedia []*sysin.MessageTemplateMediaInp, traceId string) error {
	if tenantId <= 0 || accountId <= 0 || strings.TrimSpace(profileNo) == "" {
		return gerror.New("资料媒体任务参数不完整")
	}
	if len(displayMedia) == 0 && len(verifyMedia) == 0 {
		return nil
	}
	payload, err := json.Marshal(profileMediaOperationInput{TenantId: tenantId, AccountId: accountId, ProfileNo: profileNo, DisplayMedia: displayMedia, VerifyMedia: verifyMedia, TraceId: traceId})
	if err != nil {
		return gerror.Wrap(err, "编码资料媒体任务失败")
	}
	operationNo := fmt.Sprintf("media:%d:%s:%s", accountId, profileNo, strings.TrimSpace(traceId))
	if strings.HasSuffix(operationNo, ":") {
		operationNo = fmt.Sprintf("%s:%d", operationNo, time.Now().UnixNano())
	}
	result, err := g.DB().Model(profileMediaOperationTable).Ctx(ctx).Data(g.Map{
		"tenant_id": tenantId, "account_id": accountId, "profile_no": profileNo,
		"operation_no": operationNo, "status": "pending", "payload_json": string(payload),
		"trace_id": traceId, "retry_count": 0,
	}).OnConflict("tenant_id,account_id,profile_no,operation_no").OnDuplicate("payload_json,status,error_message,trace_id,retry_count,updated_at").Save()
	if err != nil {
		return gerror.Wrap(err, "保存资料媒体任务失败")
	}
	operationId, err := result.LastInsertId()
	if err != nil || operationId <= 0 {
		row, queryErr := g.DB().Model(profileMediaOperationTable).Ctx(ctx).Fields("id").Where("tenant_id", tenantId).Where("account_id", accountId).Where("profile_no", profileNo).Where("operation_no", operationNo).One()
		if queryErr != nil {
			return gerror.Wrap(err, "读取资料媒体任务失败")
		}
		operationId = row["id"].Int64()
	}
	return s.enqueueProfileMediaOperation(ctx, operationId)
}

func (s *sSysPublish) enqueueProfileMediaOperation(ctx context.Context, operationId int64) error {
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(profileMediaOperationPayload{OperationId: operationId})
	if err != nil {
		return err
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeProfileMedia, body), asynq.Queue(tgQueueNameBackground), asynq.MaxRetry(7), asynq.Timeout(10*time.Minute), asynq.TaskID(fmt.Sprintf("profile-media:%d", operationId)))
	return err
}

func (s *sSysPublish) handleProfileMediaTask(ctx context.Context, task *asynq.Task) error {
	var payload profileMediaOperationPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil || payload.OperationId <= 0 {
		return gerror.New("资料媒体任务参数无效")
	}
	var row struct {
		Id          int64  `json:"id"`
		Status      string `json:"status"`
		PayloadJson string `json:"payload_json"`
		RetryCount  int    `json:"retry_count"`
	}
	if err := g.DB().Model(profileMediaOperationTable).Ctx(ctx).Where("id", payload.OperationId).Scan(&row); err != nil {
		return err
	}
	if row.Id == 0 || row.Status == "success" {
		return nil
	}
	var input profileMediaOperationInput
	if err := json.Unmarshal([]byte(row.PayloadJson), &input); err != nil {
		return gerror.Wrap(err, "解析资料媒体任务失败")
	}
	_, err := g.DB().Model(profileMediaOperationTable).Ctx(ctx).Where("id", payload.OperationId).WhereIn("status", []string{"pending", "processing"}).Data(g.Map{"status": "processing", "started_at": time.Now(), "retry_count": row.RetryCount + 1}).Update()
	if err != nil {
		return err
	}
	if err = botservice.SysBot().BotProcessProfileMedia(ctx, input.TenantId, input.AccountId, input.ProfileNo, input.DisplayMedia, input.VerifyMedia); err != nil {
		_, _ = g.DB().Model(profileMediaOperationTable).Ctx(ctx).Where("id", payload.OperationId).Data(g.Map{"status": "pending", "error_message": err.Error(), "updated_at": time.Now()}).Update()
		return err
	}
	_, err = g.DB().Model(profileMediaOperationTable).Ctx(ctx).Where("id", payload.OperationId).Data(g.Map{"status": "success", "finished_at": time.Now(), "error_message": "", "updated_at": time.Now()}).Update()
	if err != nil {
		return err
	}
	// Media persistence is the event that releases a publish request waiting
	// on media. Wake it in the same worker completion path; relying on a
	// periodic scanner leaves users stuck after a successful media task.
	var profile struct {
		Id int64 `orm:"id"`
	}
	if err = g.DB().Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).
		Fields("id").Where("profile_no", input.ProfileNo).WhereNull("deleted_at").Scan(&profile); err != nil {
		return err
	}
	if profile.Id > 0 {
		if err = s.wakeProfileTelegramJobs(ctx, profile.Id); err != nil {
			g.Log().Warningf(ctx, "媒体任务完成后唤醒资料上架失败 profileId:%d operationId:%d err:%+v", profile.Id, payload.OperationId, err)
		}
	}
	return nil
}

// recoverProfileMediaOperations makes the media pipeline restart-safe. It is
// intentionally bounded; subsequent scheduler passes pick up the remainder.
func (s *sSysPublish) recoverProfileMediaOperations(ctx context.Context) {
	var rows []struct {
		Id int64 `json:"id"`
	}
	if err := g.DB().Model(profileMediaOperationTable).Ctx(ctx).
		Fields("id").WhereIn("status", []string{"pending", "processing"}).
		Where("updated_at < ?", time.Now().Add(-2*time.Minute)).OrderAsc("id").Limit(200).Scan(&rows); err != nil {
		g.Log().Warningf(ctx, "恢复资料媒体任务失败 err:%s", err.Error())
		return
	}
	for _, row := range rows {
		if err := s.enqueueProfileMediaOperation(ctx, row.Id); err != nil {
			g.Log().Warningf(ctx, "恢复资料媒体任务入队失败 operation_id:%d err:%s", row.Id, err.Error())
		}
	}
	// Close the partial-completion window: media rows may have committed while
	// the operation status update or queue acknowledgement timed out.
	var readyRows []struct {
		OperationId int64 `json:"operationId"`
		ProfileId   int64 `json:"profileId"`
	}
	readyQuery := `SELECT o.id AS operation_id,p.profile_id
		FROM ` + profileMediaOperationTable + ` o
		JOIN ` + publishProfileStateTable + ` p ON p.tenant_id=o.tenant_id AND p.account_id=o.account_id AND p.deleted_at IS NULL
		JOIN ` + dao.ContentProfile.Table() + ` cp ON cp.id=p.profile_id AND cp.profile_no=o.profile_no AND cp.deleted_at IS NULL
		WHERE o.status IN ('pending','processing') AND p.publish_task_status='waiting_media'
		AND (SELECT COUNT(*) FROM ` + publishMediaTable + ` m WHERE m.profile_id=p.profile_id AND m.deleted_at IS NULL)
			>= (p.expected_display_media_count+p.expected_verify_media_count)
		ORDER BY o.id LIMIT 200`
	if err := g.DB().GetScan(ctx, &readyRows, readyQuery); err != nil {
		g.Log().Warningf(ctx, "读取已齐全媒体任务失败 err:%s", err.Error())
	} else {
		for _, row := range readyRows {
			if _, err := g.DB().Model(profileMediaOperationTable).Ctx(ctx).Where("id", row.OperationId).
				WhereIn("status", []string{"pending", "processing"}).
				Data(g.Map{"status": "success", "error_message": "媒体记录已齐全，恢复收口", "finished_at": gtime.Now(), "updated_at": gtime.Now()}).Update(); err != nil {
				g.Log().Warningf(ctx, "收口已齐全媒体任务失败 operation_id:%d err:%s", row.OperationId, err.Error())
				continue
			}
			if err := s.wakeProfileTelegramJobs(ctx, row.ProfileId); err != nil {
				g.Log().Warningf(ctx, "收口后唤醒资料上架失败 profile_id:%d err:%s", row.ProfileId, err.Error())
			}
		}
	}
	// The media worker can finish successfully while the wake-up transaction
	// times out. Treat the durable success row plus waiting_media state as a
	// persisted media-ready event and retry the wake directly.
	var completed []struct {
		ProfileId int64 `json:"profileId"`
	}
	if err := g.DB().Model(profileMediaOperationTable+" o").Ctx(ctx).
		Fields("p.profile_id").
		InnerJoin(publishProfileStateTable+" p", "p.tenant_id=o.tenant_id AND p.account_id=o.account_id AND p.deleted_at IS NULL").
		InnerJoin(dao.ContentProfile.Table()+" cp", "cp.id=p.profile_id AND cp.profile_no=o.profile_no AND cp.deleted_at IS NULL").
		Where("o.status", "success").Where("p.publish_task_status", "waiting_media").
		Where("o.updated_at < ?", time.Now().Add(-10*time.Second)).
		OrderAsc("o.id").Limit(200).Scan(&completed); err != nil {
		g.Log().Warningf(ctx, "读取已完成媒体唤醒事件失败 err:%s", err.Error())
	} else {
		for _, row := range completed {
			if err := s.wakeProfileTelegramJobs(ctx, row.ProfileId); err != nil {
				g.Log().Warningf(ctx, "恢复已完成媒体上架唤醒失败 profile_id:%d err:%s", row.ProfileId, err.Error())
			}
		}
	}
	s.recoverWaitingMediaPublishes(ctx)
}

// recoverWaitingMediaPublishes is a DB-backed safety net for the small window
// where media completion commits successfully but Redis/Asynq is unavailable,
// or the wake event runs before waiting_media is recorded. It only revisits
// durable waiting intents; normal completion remains event driven.
func (s *sSysPublish) recoverWaitingMediaPublishes(ctx context.Context) {
	var rows []struct {
		ProfileId int64 `json:"profileId"`
	}
	if err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Fields("profile_id").Where("publish_task_status", "waiting_media").
		WhereNull("deleted_at").Where("publish_task_updated_at < ?", time.Now().Add(-10*time.Second)).
		OrderAsc("publish_task_updated_at").Limit(200).Scan(&rows); err != nil {
		g.Log().Warningf(ctx, "恢复等待媒体上架状态失败 err:%s", err.Error())
		return
	}
	for _, row := range rows {
		if err := s.wakeProfileTelegramJobs(ctx, row.ProfileId); err != nil {
			g.Log().Warningf(ctx, "恢复等待媒体上架失败 profileId:%d err:%+v", row.ProfileId, err)
		}
	}
}

func (s *sSysPublish) profileMediaRecoveryLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.recoverProfileMediaOperations(ctx)
		}
	}
}
