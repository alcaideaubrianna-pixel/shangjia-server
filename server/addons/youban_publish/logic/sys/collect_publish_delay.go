package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) collectTelegramJobHasPendingVerifyContinuation(ctx context.Context, job telegramJobRecord) (bool, error) {
	if job.CollectSourceId <= 0 || job.CollectSourceMessageId <= 0 || strings.TrimSpace(job.CollectSourceChatId) == "" || job.ProfileId <= 0 {
		return false, nil
	}
	verifyCount, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", job.ProfileId).
		Where("purpose", "verify").
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查采集验证视频失败")
	}
	if verifyCount > 0 {
		return false, nil
	}
	mod := g.DB().Model(pdaoCollectEventTable()+" e").Safe().Ctx(ctx).
		Where("e.tenant_id", job.TenantId).
		Where("e.account_id", job.AccountId).
		Where("e.source_id", job.CollectSourceId).
		Where("e.source_chat_id", strings.TrimSpace(job.CollectSourceChatId)).
		Where("e.source_message_id > ?", job.CollectSourceMessageId).
		Where("e.media_count > 0").
		Where("COALESCE(e.raw_text, '') = ''").
		Where("NOT EXISTS (SELECT 1 FROM "+pdaoCollectEventTable()+" nx WHERE nx.tenant_id=e.tenant_id AND nx.account_id=e.account_id AND nx.source_id=e.source_id AND nx.source_chat_id=e.source_chat_id AND nx.source_message_id > ? AND nx.source_message_id < e.source_message_id AND nx.media_count > 0 AND COALESCE(nx.raw_text, '') <> '')", job.CollectSourceMessageId).
		WhereIn("e.status", []string{
			sysin.CollectEventStatusPending,
			sysin.CollectEventStatusGroupCollect,
			sysin.CollectEventStatusWaitingOrder,
			sysin.CollectEventStatusPrechecked,
			sysin.CollectEventStatusMediaPending,
			sysin.CollectEventStatusMediaReady,
			sysin.CollectEventStatusProcessed,
		})
	if since := collectEventOrderWindowSince(ctx); since != nil {
		mod = mod.WhereGTE("e.created_at", since)
	}
	mediaTable := pdao.YoubanPublishCollectEventMedia.Table()
	mod = mod.Where("EXISTS (SELECT 1 FROM " + mediaTable + " m WHERE m.event_id=e.id AND m.media_type='video')")
	mod = mod.Where("NOT EXISTS (SELECT 1 FROM " + mediaTable + " m WHERE m.event_id=e.id AND m.media_type<>'video')")
	count, err := mod.Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查采集后续验证视频失败")
	}
	return count > 0, nil
}
