package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	botService "hotgo/addons/youban_bot/service"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/cache"
)

const (
	imageQuotaDefaultMonthlyLimit int64 = 5000
	imageQuotaPerUSDT             int64 = 100
	imageQuotaProductType               = "image_quota"
	imageQuotaTable                     = "hg_youban_publish_image_quota"
	imageQuotaLedgerTable               = "hg_youban_publish_image_quota_ledger"
)

type imageQuotaRow struct {
	TenantId           int64  `orm:"tenant_id"`
	MonthlyLimit       int64  `orm:"monthly_limit"`
	MonthlyUsed        int64  `orm:"monthly_used"`
	PurchasedRemaining int64  `orm:"purchased_remaining"`
	Period             string `orm:"period"`
}

func (s *sSysPublish) ensureImageQuotaAvailable(ctx context.Context, tenantId int64) error {
	quota, err := s.imageProcessingQuota(ctx, tenantId)
	if err != nil {
		return err
	}
	if quota.TotalRemaining <= 0 {
		return gerror.New("图片处理额度已用完，请购买额外额度或等待下月重置")
	}
	return nil
}

func (s *sSysPublish) consumeImageQuota(ctx context.Context, tenantId, accountId int64, referenceKey, scene string) error {
	if err := s.ensureImageQuotaPeriod(ctx, tenantId); err != nil {
		return err
	}
	vip, err := s.tenantVipStatus(ctx, tenantId)
	if err != nil {
		return err
	}
	vipActive := vip != nil && vip.IsVip
	remainingAfterConsume := int64(-1)
	period := imageQuotaPeriod(time.Now())
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		result, err := tx.Model(imageQuotaLedgerTable).Ctx(ctx).Data(g.Map{
			"tenant_id": tenantId, "account_id": accountId, "event_type": "consume",
			"reference_key": referenceKey, "change_count": -1, "remark": scene,
			"created_at": gtime.Now(),
		}).InsertIgnore()
		if err != nil {
			return gerror.Wrap(err, "写入图片额度使用流水失败")
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			return nil
		}
		var row imageQuotaRow
		if err = tx.Model(imageQuotaTable).Ctx(ctx).Where("tenant_id", tenantId).LockUpdate().Scan(&row); err != nil {
			return gerror.Wrap(err, "锁定图片处理额度失败")
		}
		if vipActive && row.MonthlyUsed < row.MonthlyLimit {
			remainingAfterConsume = row.MonthlyLimit - row.MonthlyUsed - 1 + row.PurchasedRemaining
			_, err = tx.Model(imageQuotaTable).Ctx(ctx).Where("tenant_id", tenantId).
				Data(g.Map{"monthly_used": row.MonthlyUsed + 1, "updated_at": gtime.Now()}).Update()
		} else if row.PurchasedRemaining > 0 {
			remainingAfterConsume = row.PurchasedRemaining - 1
			_, err = tx.Model(imageQuotaTable).Ctx(ctx).Where("tenant_id", tenantId).
				Data(g.Map{"purchased_remaining": row.PurchasedRemaining - 1, "updated_at": gtime.Now()}).Update()
		} else {
			return gerror.New("图片处理额度已用完，请购买额外额度或等待下月重置")
		}
		return gerror.Wrap(err, "扣减图片处理额度失败")
	})
	if err == nil {
		_, _ = cache.Instance().Remove(ctx, imageQuotaCacheKey(tenantId))
		_, _ = cache.Instance().Remove(ctx, tenantVipFullCacheKey(tenantId))
		if remainingAfterConsume >= 0 && remainingAfterConsume <= 500 {
			s.notifyImageQuotaThreshold(ctx, tenantId, accountId, remainingAfterConsume, period)
		}
	}
	return err
}

func (s *sSysPublish) notifyImageQuotaThreshold(ctx context.Context, tenantId, accountId, remaining int64, period string) {
	threshold := int64(-1)
	for _, candidate := range []int64{0, 100, 500} {
		if remaining <= candidate {
			threshold = candidate
			break
		}
	}
	if threshold < 0 {
		return
	}
	reference := fmt.Sprintf("quota_notice:%d:%s:%d", tenantId, period, threshold)
	result, err := g.DB().Model(imageQuotaLedgerTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id": tenantId, "account_id": accountId, "event_type": "notice",
		"reference_key": reference, "change_count": 0,
		"remark": fmt.Sprintf("图片处理额度阈值提醒:%d", threshold), "created_at": gtime.Now(),
	}).InsertIgnore()
	if err != nil {
		g.Log().Warningf(ctx, "写入图片额度提醒记录失败 tenantId:%d threshold:%d err:%+v", tenantId, threshold, err)
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return
	}
	notifyAccountId, err := s.tenantVipNotifyAccountId(ctx, tenantId, accountId)
	if err != nil || notifyAccountId <= 0 {
		return
	}
	message := fmt.Sprintf("图片背景替换额度即将耗尽。\n\n当前剩余：%d 次\n额度用完后，频道会自动切换为轻量随机扰动，不会中断发布。", remaining)
	if remaining <= 0 {
		message = "图片背景替换额度已用完。\n\n频道已自动降级为轻量随机扰动，发布任务不会中断。购买额外额度后会自动恢复背景替换。"
	}
	if err = botService.SysBot().NotifyAccount(ctx, &botsysin.NotifyAccountInp{
		BotStrategy: "official", FallbackBoundBot: true, IgnoreFeatureSwitch: true,
		App: consts.AppApi, AccountId: notifyAccountId, Text: message,
	}); err != nil {
		g.Log().Warningf(ctx, "发送图片额度提醒失败 tenantId:%d accountId:%d err:%+v", tenantId, notifyAccountId, err)
	}
}

func (s *sSysPublish) ImageProcessingQuota(ctx context.Context) (*sysin.ImageProcessingQuotaModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.imageProcessingQuota(ctx, account.TenantId)
}

func (s *sSysPublish) imageProcessingQuota(ctx context.Context, tenantId int64) (*sysin.ImageProcessingQuotaModel, error) {
	if tenantId <= 0 {
		return nil, gerror.New("租户ID不能为空")
	}
	cacheKey := imageQuotaCacheKey(tenantId)
	if value, err := cache.Instance().Get(ctx, cacheKey); err == nil && !value.IsNil() {
		var cached sysin.ImageProcessingQuotaModel
		if scanErr := value.Scan(&cached); scanErr == nil && cached.Period != "" {
			return &cached, nil
		}
	}
	if err := s.ensureImageQuotaPeriod(ctx, tenantId); err != nil {
		return nil, err
	}
	var row imageQuotaRow
	if err := g.DB().Model(imageQuotaTable).Safe().Ctx(ctx).Where("tenant_id", tenantId).Scan(&row); err != nil {
		return nil, gerror.Wrap(err, "读取图片处理额度失败")
	}
	vip, err := s.tenantVipStatus(ctx, tenantId)
	if err != nil {
		return nil, err
	}
	monthlyLimit := row.MonthlyLimit
	if monthlyLimit <= 0 {
		monthlyLimit = imageQuotaDefaultMonthlyLimit
	}
	monthlyRemaining := int64(0)
	if vip != nil && vip.IsVip {
		monthlyRemaining = monthlyLimit - row.MonthlyUsed
		if monthlyRemaining < 0 {
			monthlyRemaining = 0
		}
	}
	res := &sysin.ImageProcessingQuotaModel{
		MonthlyLimit:       monthlyLimit,
		MonthlyUsed:        row.MonthlyUsed,
		MonthlyRemaining:   monthlyRemaining,
		PurchasedRemaining: row.PurchasedRemaining,
		TotalRemaining:     monthlyRemaining + row.PurchasedRemaining,
		Period:             imageQuotaPeriod(time.Now()),
		ResetAt:            imageQuotaNextReset(time.Now()).Format("2006-01-02 15:04:05"),
	}
	_ = cache.Instance().Set(ctx, cacheKey, res, 2*time.Minute)
	return res, nil
}

func (s *sSysPublish) ensureImageQuotaPeriod(ctx context.Context, tenantId int64) error {
	now := time.Now()
	period := imageQuotaPeriod(now)
	_, err := g.DB().Model(imageQuotaTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id": tenantId, "monthly_limit": imageQuotaDefaultMonthlyLimit,
		"monthly_used": 0, "purchased_remaining": 0, "period": period,
		"created_at": gtime.Now(), "updated_at": gtime.Now(),
	}).InsertIgnore()
	if err != nil {
		return gerror.Wrap(err, "初始化图片处理额度失败")
	}
	_, err = g.DB().Model(imageQuotaTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).WhereNot("period", period).
		Data(g.Map{"monthly_used": 0, "period": period, "updated_at": gtime.Now()}).Update()
	return gerror.Wrap(err, "重置图片处理月度额度失败")
}

func (s *sSysPublish) addPurchasedImageQuota(ctx context.Context, tenantId, accountId, count int64, referenceKey, remark string) error {
	if tenantId <= 0 || count <= 0 || strings.TrimSpace(referenceKey) == "" {
		return gerror.New("图片处理额度充值参数不合法")
	}
	if err := s.ensureImageQuotaPeriod(ctx, tenantId); err != nil {
		return err
	}
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		result, err := tx.Model(imageQuotaLedgerTable).Ctx(ctx).Data(g.Map{
			"tenant_id": tenantId, "account_id": accountId, "event_type": "purchase",
			"reference_key": referenceKey, "change_count": count, "remark": remark,
			"created_at": gtime.Now(),
		}).InsertIgnore()
		if err != nil {
			return gerror.Wrap(err, "写入图片额度充值流水失败")
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			return nil
		}
		_, err = tx.Model(imageQuotaTable).Ctx(ctx).Where("tenant_id", tenantId).
			Data(g.Map{"updated_at": gtime.Now()}).Increment("purchased_remaining", count)
		return gerror.Wrap(err, "增加图片处理额度失败")
	})
	if err == nil {
		_, _ = cache.Instance().Remove(ctx, imageQuotaCacheKey(tenantId))
	}
	return err
}

func imageQuotaPeriod(now time.Time) string { return now.Format("2006-01") }

func imageQuotaNextReset(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
}

func imageQuotaCacheKey(tenantId int64) string {
	return fmt.Sprintf("youban_publish:image_quota:%d", tenantId)
}
