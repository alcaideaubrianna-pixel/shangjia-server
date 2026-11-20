package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/internal/library/microbatch"
)

type deliveryCompletion struct {
	ID    int64
	Owner string
}

func (s *sCollector) completeRealtimeDelivery(ctx context.Context, completion deliveryCompletion) error {
	s.queueMu.Lock()
	if s.realtimeDeliveryReadyBatch == nil {
		parent := s.runtimeCtx
		if parent == nil {
			parent = context.Background()
		}
		s.realtimeDeliveryReadyBatch = microbatch.New(parent, 16, 5*time.Millisecond, func(ctx context.Context, completions []deliveryCompletion) []microbatch.Result[bool] {
			writeCtx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			return writeDeliveryCompletions(writeCtx, completions)
		})
	}
	writer := s.realtimeDeliveryReadyBatch
	s.queueMu.Unlock()

	confirmed, err := writer.Do(ctx, completion)
	if err != nil {
		return err
	}
	if !confirmed {
		return gerror.New("采集交付执行归属已变化，未确认本次完成")
	}
	return nil
}

func writeDeliveryCompletions(ctx context.Context, completions []deliveryCompletion) []microbatch.Result[bool] {
	c := dao.TgCollectorDelivery.Columns()
	values := make([]string, 0, len(completions))
	args := make([]any, 0, len(completions)*2+2)
	args = append(args, sysin.DeliveryStatusCompleted)
	for _, completion := range completions {
		values = append(values, "(CAST(? AS bigint),CAST(? AS text))")
		args = append(args, completion.ID, completion.Owner)
	}
	args = append(args, sysin.DeliveryStatusProcessing)
	query := fmt.Sprintf(`UPDATE %s d SET %s=?, %s=NULL, %s='', %s=NULL, %s='', %s=(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai')
 FROM (VALUES %s) AS input(delivery_id,owner)
 WHERE d.%s=input.delivery_id AND d.%s=input.owner AND d.%s=?
 RETURNING input.delivery_id,input.owner`, dao.TgCollectorDelivery.Table(), c.Status, c.NextRunAt, c.LeaseOwner, c.LeaseUntil, c.ErrorMessage, c.UpdatedAt,
		strings.Join(values, ","), c.Id, c.LeaseOwner, c.Status)
	rows, err := g.DB().GetAll(ctx, query, args...)
	matched := make(map[deliveryCompletion]bool, len(rows))
	for _, row := range rows {
		matched[deliveryCompletion{ID: row["delivery_id"].Int64(), Owner: row["owner"].String()}] = true
	}
	results := make([]microbatch.Result[bool], len(completions))
	for i, completion := range completions {
		results[i] = microbatch.Result[bool]{Value: matched[completion], Err: err}
	}
	return results
}
