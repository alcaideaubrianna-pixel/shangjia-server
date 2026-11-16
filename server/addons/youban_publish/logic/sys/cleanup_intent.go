package sys

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// telegramCleanupIntent stores immutable execution context, independently of current profile state.
type telegramCleanupIntent struct {
	Id          int64  `orm:"id"`
	JobId       int64  `orm:"job_id"`
	EventType   string `orm:"event_type"`
	OperationNo string `orm:"operation_no"`
	Payload     string `orm:"payload"`
}

func telegramCleanupIntentDedupeKey(jobID int64, eventType, operationNo string) string {
	if operationNo == "" {
		return fmt.Sprintf("telegram-cleanup:%d", jobID)
	}
	digest := sha256.Sum256([]byte(eventType + "\x00" + operationNo))
	return fmt.Sprintf("telegram-cleanup:%d:%x", jobID, digest)
}

func normalizeTelegramCleanupIntent(jobID int64, eventType, operationNo string, payload tgQueuePayload) (tgQueuePayload, error) {
	if jobID <= 0 {
		return payload, gerror.New("清理意图任务ID不合法")
	}
	switch eventType {
	case telegramOutboxCleanupPrefix + "manual", telegramOutboxCleanupPrefix + "keyword", telegramOutboxCleanupPrefix + "cycle":
	default:
		return payload, gerror.New("清理意图事件类型不合法")
	}
	if utf8.RuneCountInString(operationNo) > 128 {
		return payload, gerror.New("清理操作号超过长度限制")
	}
	if payload.JobId != 0 && payload.JobId != jobID {
		return payload, gerror.New("清理意图与消息任务不匹配")
	}
	if strings.TrimSpace(payload.CleanupOperationNo) != "" && strings.TrimSpace(payload.CleanupOperationNo) != operationNo {
		return payload, gerror.New("清理意图操作号不匹配")
	}
	payload.JobId = jobID
	payload.CleanupOperationNo = operationNo
	return payload, nil
}

// ensureTelegramCleanupIntent inserts once. Conflicts never reactivate delivered rows or rewrite payload.
// Empty operation retains the historical job-scoped dedupe key and never claims current profile state.
func ensureTelegramCleanupIntent(ctx context.Context, jobID int64, eventType, operationNo string, payload tgQueuePayload) (outboxID int64, created bool, err error) {
	eventType, operationNo = strings.TrimSpace(eventType), strings.TrimSpace(operationNo)
	payload, err = normalizeTelegramCleanupIntent(jobID, eventType, operationNo, payload)
	if err != nil {
		return
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return 0, false, gerror.Wrap(err, "序列化清理意图失败")
	}
	// Execution identity is assigned by the database; it is not part of persisted semantic payload.
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &fields); err != nil {
		return 0, false, err
	}
	delete(fields, "cleanupOutboxId")
	encoded, err = json.Marshal(fields)
	if err != nil {
		return 0, false, err
	}
	key := telegramCleanupIntentDedupeKey(jobID, eventType, operationNo)
	result, err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).InsertIgnore(g.Map{
		"job_id": jobID, "event_type": eventType, "operation_no": operationNo, "payload": string(encoded),
		"dedupe_key": key, "status": telegramOutboxPending, "available_at": gtime.Now(),
	})
	if err != nil {
		return 0, false, gerror.Wrap(err, "保存清理意图失败，请确认持久化迁移已执行")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, false, gerror.Wrap(err, "读取清理意图写入结果失败")
	}
	var row *telegramCleanupIntent
	err = g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).Master().Where("dedupe_key", key).
		Fields("id,job_id,event_type,operation_no,payload").Scan(&row)
	if err != nil {
		return 0, false, gerror.Wrap(err, "读取清理意图失败")
	}
	if row == nil {
		return 0, false, gerror.New("清理意图未保存成功")
	}
	if row.JobId != jobID || row.EventType != eventType || row.OperationNo != operationNo {
		return 0, false, gerror.New("清理意图幂等键归属冲突")
	}
	// First writer owns context. Existing legacy empty payload remains empty and is not upgraded in place.
	if row.Payload != "" {
		restored, restoreErr := decodeTelegramCleanupIntentPayload(*row)
		if restoreErr != nil {
			return 0, false, restoreErr
		}
		if restored.ForceCompletion != payload.ForceCompletion || !equalTelegramCleanupJobIDs(restored.CleanupJobIds, payload.CleanupJobIds) {
			return 0, false, gerror.New("清理意图已存在且执行上下文不同")
		}
	}
	return row.Id, affected > 0, nil
}

func equalTelegramCleanupJobIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// hydrateTelegramCleanupIntentPayload restores exactly one intent, without consulting mutable profile state.
func hydrateTelegramCleanupIntentPayload(ctx context.Context, outboxID int64) (tgQueuePayload, error) {
	if outboxID <= 0 {
		return tgQueuePayload{}, gerror.New("清理意图ID不合法")
	}
	var row *telegramCleanupIntent
	if err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).Master().Where("id", outboxID).
		Fields("id,job_id,event_type,operation_no,payload").Scan(&row); err != nil {
		return tgQueuePayload{}, gerror.Wrap(err, "恢复清理意图失败")
	}
	if row == nil {
		return tgQueuePayload{}, gerror.New("清理意图不存在")
	}
	return decodeTelegramCleanupIntentPayload(*row)
}

func decodeTelegramCleanupIntentPayload(row telegramCleanupIntent) (tgQueuePayload, error) {
	var payload tgQueuePayload
	if strings.TrimSpace(row.Payload) != "" {
		if err := json.Unmarshal([]byte(row.Payload), &payload); err != nil {
			return payload, gerror.Wrap(err, "清理意图上下文损坏")
		}
	}
	var err error
	payload, err = normalizeTelegramCleanupIntent(row.JobId, row.EventType, row.OperationNo, payload)
	if err != nil {
		return payload, err
	}
	// JSON assignment supports the agreed CleanupOutboxId field without editing tg_queue.go here.
	identity, err := json.Marshal(map[string]int64{"cleanupOutboxId": row.Id})
	if err != nil {
		return payload, err
	}
	if err = json.Unmarshal(identity, &payload); err != nil {
		return payload, err
	}
	return payload, nil
}
