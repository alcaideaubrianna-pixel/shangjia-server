package channelprogress

import (
	"context"
	"fmt"
	"github.com/gogf/gf/v2/frame/g"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/internal/library/microbatch"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
)

type Progress struct{ TenantID, ChannelID, MessageID int64 }

func Write(ctx context.Context, input []Progress) []microbatch.Result[bool] {
	out := make([]microbatch.Result[bool], len(input))
	if len(input) == 0 {
		return out
	}
	if len(input) > 50 {
		for i := range out {
			out[i].Err = gerror.New("频道进度批次不能超过50条")
		}
		return out
	}
	var values []string
	var args []any
	for _, p := range input {
		values = append(values, "(CAST(? AS bigint),CAST(? AS bigint),CAST(? AS bigint))")
		args = append(args, p.TenantID, p.ChannelID, p.MessageID)
	}
	c := pdao.YoubanPublishCollectChannel.Columns()
	now := "(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai')"
	query := fmt.Sprintf(`WITH input(tenant_id,channel_id,message_id) AS (VALUES %s), latest AS (
 SELECT tenant_id,channel_id,MAX(message_id) AS message_id FROM input GROUP BY tenant_id,channel_id)
 UPDATE %s c SET %s=GREATEST(COALESCE(c.%s,0),i.message_id),
 %s=CASE WHEN COALESCE(c.%s,0)<=i.message_id THEN %s ELSE c.%s END,
 %s=%s, %s=%s FROM latest i WHERE c.%s=i.channel_id AND c.%s=i.tenant_id`,
		strings.Join(values, ","), pdao.YoubanPublishCollectChannel.Table(), c.LastMessageId, c.LastMessageId,
		c.LastMessageAt, c.LastMessageId, now, c.LastMessageAt, c.LastSeenAt, now, c.UpdatedAt, now, c.Id, c.TenantId)
	_, err := g.DB().Exec(ctx, query, args...)
	for i := range out {
		out[i] = microbatch.Result[bool]{Value: err == nil, Err: err}
	}
	if err != nil {
		g.Log().Warning(ctx, "合并采集频道进度写入失败", g.Map{"event": "collect_channel_progress_batch", "requested": len(input), "error": err.Error()})
	}
	return out
}
