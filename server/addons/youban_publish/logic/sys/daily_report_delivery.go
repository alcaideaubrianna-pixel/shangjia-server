package sys

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"

	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	botService "hotgo/addons/youban_bot/service"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/library/dailyreport"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const dailyReportDeliveryMaxAttempts = 5

type dailyReportDeliveryRow struct {
	ID        int64  `orm:"id"`
	TenantID  int64  `orm:"tenant_id"`
	AccountID int64  `orm:"account_id"`
	Payload   string `orm:"source_breakdown_json"`
	Cursor    int    `orm:"notify_cursor"`
	Attempts  int    `orm:"notify_retry_count"`
}

func (s *sSysPublish) ProcessDailyReports(ctx context.Context, now time.Time) error {
	// Keep the generation/delivery coordinator bounded so a backlog cannot hold
	// the cron worker indefinitely. Lease duration exceeds this deadline.
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return err
	}
	today := now.In(loc)
	budget := 100
	var firstError error
	for day := 1; day <= 7 && budget > 0; day++ {
		generated, generationErr := s.generateDailyReportBatch(ctx, today.AddDate(0, 0, -day), budget)
		if generationErr != nil {
			firstError = generationErr
			break
		}
		budget -= generated
	}
	// A generation failure must not prevent already-persisted reports from
	// being delivered, nor should one failed delivery prevent retention cleanup.
	if deliveryErr := s.deliverDailyReports(ctx, now, 50); deliveryErr != nil && firstError == nil {
		firstError = deliveryErr
	}
	_, err = pdao.YoubanPublishDailyReport.Ctx(ctx).WhereLT("report_date", dailyReportOldestDate(today, 7)).Delete()
	if err != nil {
		return gerror.Wrap(err, "清理过期日报失败")
	}
	return firstError
}

func dailyReportDeliveryDue() string {
	return `((notify_status IN ('pending','failed') AND (notify_next_retry_at IS NULL OR notify_next_retry_at<=?)) OR (notify_status='sending' AND notify_lease_until<=?))`
}

func (s *sSysPublish) deliverDailyReports(ctx context.Context, now time.Time, limit int) error {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return err
	}
	var rows []*dailyReportDeliveryRow
	err = pdao.YoubanPublishDailyReport.Ctx(ctx).Where(dailyReportDeliveryDue(), now, now).WhereLT("notify_retry_count", dailyReportDeliveryMaxAttempts).WhereGTE("report_date", dailyReportOldestDate(now.In(loc), 7)).WhereLT("report_date", now.In(loc).Format("2006-01-02")).Order("report_date,id").Limit(limit).Scan(&rows)
	if err != nil {
		return gerror.Wrap(err, "读取待投递日报失败")
	}
	var firstError error
	for _, row := range rows {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = s.deliverDailyReport(ctx, row, now); err != nil {
			g.Log().Warningf(ctx, "每日推送报告投递失败 reportId:%d tenantId:%d accountId:%d err:%+v", row.ID, row.TenantID, row.AccountID, err)
			if firstError == nil {
				firstError = err
			}
		}
	}
	return firstError
}

func (s *sSysPublish) deliverDailyReport(ctx context.Context, row *dailyReportDeliveryRow, now time.Time) error {
	token := guid.S()
	claim, err := pdao.YoubanPublishDailyReport.Ctx(ctx).Where("id", row.ID).Where(dailyReportDeliveryDue(), now, now).WhereLT("notify_retry_count", dailyReportDeliveryMaxAttempts).Data(do.YoubanPublishDailyReport{NotifyStatus: "sending", NotifyToken: token, NotifyLeaseUntil: gtime.New(now.Add(2 * time.Minute))}).Update()
	if err != nil {
		return err
	}
	n, err := claim.RowsAffected()
	if err != nil || n == 0 {
		return err
	}
	// Reload progress after the claim, since another worker may have progressed
	// the cursor between selection and acquisition of an expired lease.
	if err = pdao.YoubanPublishDailyReport.Ctx(ctx).Where("id", row.ID).Where("notify_token", token).Scan(row); err != nil {
		return err
	}
	var report sysin.DailyReportModel
	if err = json.Unmarshal([]byte(row.Payload), &report); err != nil {
		return s.failDailyReportDelivery(ctx, row, token, now, err)
	}
	if report.TenantID != row.TenantID || report.AccountID != row.AccountID {
		return s.failDailyReportDelivery(ctx, row, token, now, gerror.New("日报归属与快照不一致"))
	}
	account, err := g.DB().Model(publishAccountTable).Ctx(ctx).Where("id", row.AccountID).Where("tenant_id", row.TenantID).Where("account_type", sysin.PublishAccountTypeAdmin).Where("status", 1).WhereNull("deleted_at").One()
	if err != nil {
		return s.failDailyReportDelivery(ctx, row, token, now, err)
	}
	if account.IsEmpty() {
		return s.finishDailyReportDelivery(ctx, row.ID, token, "skipped", "账户已删除或不可用")
	}
	enabled, err := s.accountNotificationEnabled(ctx, row.TenantID, row.AccountID, "daily")
	if err != nil {
		return s.failDailyReportDelivery(ctx, row, token, now, err)
	}
	if !enabled {
		return s.finishDailyReportDelivery(ctx, row.ID, token, "skipped", "用户已关闭每日报告")
	}
	parts := dailyreport.Messages(&report)
	if row.Cursor < 0 || row.Cursor > len(parts) {
		return s.failDailyReportDelivery(ctx, row, token, now, gerror.New("日报发送进度无效"))
	}
	for i := row.Cursor; i < len(parts); i++ {
		if err = ctx.Err(); err != nil {
			return s.failDailyReportDelivery(ctx, row, token, now, err)
		}
		err = botService.SysBot().NotifyAccount(ctx, &botsysin.NotifyAccountInp{App: "api", AccountId: row.AccountID, BotStrategy: "bound", IgnoreFeatureSwitch: true, RequireDelivery: true, ParseMode: "HTML", Text: parts[i], Buttons: [][]botsysin.NotifyButton{{{Label: "查看近七天报告", Data: "acct:report"}}}})
		if err != nil {
			return s.failDailyReportDelivery(ctx, row, token, now, err)
		}
		result, err := pdao.YoubanPublishDailyReport.Ctx(ctx).Where("id", row.ID).Where("notify_token", token).Where("notify_status", "sending").Data(do.YoubanPublishDailyReport{NotifyCursor: i + 1}).Update()
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return gerror.New("日报投递租约已被替换")
		}
	}
	return s.finishDailyReportDelivery(ctx, row.ID, token, "sent", "")
}

func (s *sSysPublish) finishDailyReportDelivery(ctx context.Context, id int64, token, status, message string) error {
	_, err := pdao.YoubanPublishDailyReport.Ctx(ctx).Where("id", id).Where("notify_token", token).Where("notify_status", "sending").Data(g.Map{"notify_status": status, "notify_lease_until": nil, "notify_next_retry_at": nil, "notify_error": message}).Update()
	return err
}
func (s *sSysPublish) failDailyReportDelivery(ctx context.Context, row *dailyReportDeliveryRow, token string, now time.Time, cause error) error {
	// Use a short independent cleanup context if sending exhausted its deadline.
	cleanup := ctx
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		cleanup, cancel = context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
	}
	_, err := pdao.YoubanPublishDailyReport.Ctx(cleanup).Where("id", row.ID).Where("notify_token", token).Where("notify_status", "sending").Data(g.Map{"notify_status": "failed", "notify_retry_count": row.Attempts + 1, "notify_lease_until": nil, "notify_next_retry_at": gtime.New(now.Add(time.Duration(row.Attempts+1) * 5 * time.Minute)), "notify_error": boundDailyReportError(cause.Error())}).Update()
	if err != nil {
		return gerror.Wrap(err, "保存日报通知重试状态失败")
	}
	return cause
}
func boundDailyReportError(message string) string {
	runes := []rune(message)
	if len(runes) > 512 {
		return string(runes[:512])
	}
	return message
}
