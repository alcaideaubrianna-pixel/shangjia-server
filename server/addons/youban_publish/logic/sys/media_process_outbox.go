package sys

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/hibiken/asynq"
)

const mediaProcessOutboxEvery = time.Second

type mediaProcessDelivery struct {
	MediaID      int64  `orm:"media_id"`
	Generation   int64  `orm:"generation"`
	AttemptCount int    `orm:"attempt_count"`
	Bootstrap    bool   `orm:"bootstrap"`
	TaskID       string `orm:"task_id"`
	LeaseToken   string `orm:"lease_token"`
}

func claimMediaProcessDeliveries(ctx context.Context, token string) ([]mediaProcessDelivery, error) {
	var rows []mediaProcessDelivery
	err := g.DB().GetScan(ctx, &rows, `WITH candidates AS (
 SELECT media_id FROM hg_youban_publish_media_process_outbox
 WHERE status IN ('pending','leased') AND available_at<=statement_timestamp()
 ORDER BY available_at,media_id LIMIT 50 FOR UPDATE SKIP LOCKED
 ) UPDATE hg_youban_publish_media_process_outbox o
 SET status='leased',lease_token=?,available_at=clock_timestamp()+interval '30 seconds',
 attempt_count=o.attempt_count+1,updated_at=clock_timestamp()
 FROM candidates c WHERE o.media_id=c.media_id
 RETURNING o.media_id,o.generation,o.attempt_count,o.bootstrap,o.task_id,o.lease_token`, token)
	return rows, err
}

// Only this small, indexed task table is polled. Media execution retries and
// crashed-worker recovery belong to Asynq, not a second business-table scanner.
func dispatchMediaProcessDeliveries(ctx context.Context, deliver func(context.Context, mediaProcessDelivery) (string, string, error)) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	token := guid.S()
	rows, err := claimMediaProcessDeliveries(ctx, token)
	if err != nil {
		return gerror.Wrap(err, "领取媒体处理投递任务失败，请确认已执行 Outbox 迁移")
	}
	var firstError error
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		} // Unacknowledged leases expire normally.
		status, reason, deliveryErr := deliver(ctx, row)
		if deliveryErr != nil {
			if firstError == nil {
				firstError = deliveryErr
			}
			backoff := 1 << min(row.AttemptCount, 6)
			_, err = g.DB().Exec(ctx, `UPDATE hg_youban_publish_media_process_outbox
 SET status='pending',lease_token='',available_at=clock_timestamp()+?*interval '1 second',last_error=?,updated_at=clock_timestamp()
 WHERE media_id=? AND generation=? AND status='leased' AND lease_token=?`, backoff, boundMediaProcessDeliveryError(deliveryErr.Error()), row.MediaID, row.Generation, token)
		} else {
			if status != "sent" && status != "blocked" {
				return fmt.Errorf("invalid media delivery outcome %q", status)
			}
			_, err = g.DB().Exec(ctx, `UPDATE hg_youban_publish_media_process_outbox
 SET status=?,lease_token='',last_error=?,updated_at=clock_timestamp()
 WHERE media_id=? AND generation=? AND status='leased' AND lease_token=?`, status, reason, row.MediaID, row.Generation, token)
		}
		if err != nil {
			return gerror.Wrap(err, "保存媒体处理投递结果失败")
		}
	}
	return firstError
}

func boundMediaProcessDeliveryError(text string) string {
	chars := []rune(text)
	if len(chars) > 512 {
		return string(chars[:512])
	}
	return text
}

func (s *sSysPublish) dispatchMediaProcessOutbox(ctx context.Context) error {
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	inspectOpt := telegramQueueRedisOpt(ctx)
	inspectOpt.DialTimeout = time.Second
	inspectOpt.ReadTimeout = time.Second
	inspectOpt.WriteTimeout = time.Second
	inspector := asynq.NewInspector(inspectOpt)
	defer inspector.Close()
	return dispatchMediaProcessDeliveries(ctx, func(ctx context.Context, row mediaProcessDelivery) (string, string, error) {
		return deliverMediaProcessIntent(ctx, row, client, inspector)
	})
}

func legacyMediaProcessOwner(ctx context.Context, inspector *asynq.Inspector, mediaID int64) (string, bool, error) {
	ended := false
	for generation := 0; generation < 100; generation++ {
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		id := fmt.Sprintf("media-process:%d", mediaID)
		if generation > 0 {
			id = fmt.Sprintf("%s:repair:%d", id, generation)
		}
		info, err := inspector.GetTaskInfo(tgQueueNameMediaProcess, id)
		if errors.Is(err, asynq.ErrTaskNotFound) || errors.Is(err, asynq.ErrQueueNotFound) {
			return id, ended, nil
		}
		if err != nil {
			return "", false, err
		}
		if info.State != asynq.TaskStateArchived && info.State != asynq.TaskStateCompleted {
			return id, false, nil
		}
		ended = true
	}
	return "", true, nil
}

func (s *sSysPublish) runMediaProcessOutbox(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		delay := mediaProcessOutboxEvery
		if err := s.dispatchMediaProcessOutbox(ctx); err != nil {
			g.Log().Warningf(ctx, "投递媒体处理 Outbox 失败 err:%+v", err)
			delay = 5 * time.Second
		}
		timer.Reset(delay)
	}
}
