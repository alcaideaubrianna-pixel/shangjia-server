package sys

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model"
	"hotgo/internal/library/cache"
)

const telegramPublishPolicyCacheTTL = 5 * time.Minute
const telegramLastSuccessCacheTTL = 24 * time.Hour

func telegramPublishPolicyCacheKey(tenantId, accountId int64) string {
	return fmt.Sprintf("youban_publish:tg:policy:%d:%d", tenantId, accountId)
}

func telegramLastSuccessCacheKey(job telegramJobRecord) string {
	return fmt.Sprintf("youban_publish:tg:last_success:%d:%d:%d", job.TenantId, job.AccountId, job.ChannelId)
}

func normalizeTelegramPublishConfig(conf *model.PublishConfig) *model.PublishConfig {
	if conf == nil {
		conf = defaultPublishConfig()
	}
	if conf.SendIntervalSeconds <= 0 {
		conf.SendIntervalSeconds = 5
	}
	if conf.MaxRetryCount < 0 {
		conf.MaxRetryCount = 0
	}
	if conf.MaxRetryCount > 10 {
		conf.MaxRetryCount = 10
	}
	if conf.RetryIntervalMinutes <= 0 {
		conf.RetryIntervalMinutes = 5
	}
	return conf
}

func (s *sSysPublish) telegramPublishConfig(ctx context.Context, tenantId, accountId int64) (*model.PublishConfig, error) {
	key := telegramPublishPolicyCacheKey(tenantId, accountId)
	if value, err := cache.Instance().Get(ctx, key); err == nil && !value.IsNil() {
		conf := defaultPublishConfig()
		if scanErr := value.Scan(conf); scanErr == nil {
			return normalizeTelegramPublishConfig(conf), nil
		}
	}
	conf, err := NewSysConfig().publishConfigViewByAccount(ctx, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	conf = normalizeTelegramPublishConfig(conf)
	_ = cache.Instance().Set(ctx, key, conf, telegramPublishPolicyCacheTTL)
	return conf, nil
}

func clearTelegramPublishConfigCache(ctx context.Context, tenantId, accountId int64) {
	_, _ = cache.Instance().Remove(ctx, telegramPublishPolicyCacheKey(tenantId, accountId))
}

func (s *sSysPublish) telegramLastSuccessAt(ctx context.Context, job telegramJobRecord) (*gtime.Time, error) {
	key := telegramLastSuccessCacheKey(job)
	if value, err := cache.Instance().Get(ctx, key); err == nil && !value.IsNil() {
		if timestampMilli := value.Int64(); timestampMilli > 0 {
			return gtime.NewFromTime(time.UnixMilli(timestampMilli)), nil
		}
	}
	value, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).
		Where("account_id", job.AccountId).
		Where("channel_id", job.ChannelId).
		Where("status", "sent").
		WhereNotNull("sent_at").
		OrderDesc("id").
		Fields("sent_at").Value()
	if err != nil {
		return nil, gerror.Wrap(err, "读取频道最后成功推送时间失败")
	}
	last := value.GTime()
	if last != nil {
		_ = cache.Instance().Set(ctx, key, last.TimestampMilli(), telegramLastSuccessCacheTTL)
	}
	return last, nil
}

func (s *sSysPublish) markTelegramLastSuccess(ctx context.Context, job telegramJobRecord, sentAt *gtime.Time) {
	if sentAt == nil || job.TenantId <= 0 || job.AccountId <= 0 || job.ChannelId <= 0 {
		return
	}
	if err := cache.Instance().Set(ctx, telegramLastSuccessCacheKey(job), sentAt.TimestampMilli(), telegramLastSuccessCacheTTL); err != nil {
		g.Log().Warningf(ctx, "更新频道最后成功推送缓存失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, err)
	}
}

func (s *sSysPublish) telegramPublishIntervalDelay(ctx context.Context, job telegramJobRecord) (time.Duration, error) {
	conf, err := s.telegramPublishConfig(ctx, job.TenantId, job.AccountId)
	if err != nil {
		return 0, gerror.Wrap(err, "读取账号推送策略失败")
	}
	last, err := s.telegramLastSuccessAt(ctx, job)
	if err != nil || last == nil {
		return 0, err
	}
	next := last.Time.Add(time.Duration(conf.SendIntervalSeconds) * time.Second)
	if !next.After(time.Now()) {
		return 0, nil
	}
	return time.Until(next), nil
}

func (s *sSysPublish) postponeTelegramJobForPublishInterval(ctx context.Context, job telegramJobRecord, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	now := gtime.Now()
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).
		WhereIn("status", []string{"pending", "failed_retry"}).
		Data(g.Map{
			"dispatch_status":     tgDispatchStatusIdle,
			"next_retry_at":       now.Add(delay),
			"last_dispatch_error": "等待频道发送间隔",
			"updated_at":          now,
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "延后频道间隔任务失败")
	}
	return s.enqueueTelegramJobDirectWithUnique(ctx, job.Id, delay, false)
}
