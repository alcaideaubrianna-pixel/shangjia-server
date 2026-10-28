package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/do"

	"hotgo/addons/youban_publish/model/input/sysin"
)

var dailyReportTable = pdao.YoubanPublishDailyReport.Table()

// BuildDailyReport reads the durable collection and publish projections for a
// China calendar day. Source counts use DISTINCT profile_id, so retries and
// multiple dispatch rows cannot inflate the report.
func (s *sSysPublish) BuildDailyReport(ctx context.Context, tenantID, accountID int64, reportDate time.Time) (*sysin.DailyReportModel, error) {
	if tenantID <= 0 || accountID <= 0 {
		return nil, gerror.New("日报账号参数不合法")
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return nil, gerror.Wrap(err, "加载中国时区失败")
	}
	day := reportDate.In(loc)
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)
	report := &sysin.DailyReportModel{TenantID: tenantID, AccountID: accountID, ReportDate: start.Format("2006-01-02"), Sources: []*sysin.DailyReportSourceRow{}, GeneratedAt: time.Now()}

	var sources []struct {
		SourceID int64  `orm:"source_id"`
		ChatID   string `orm:"source_chat_id"`
		Title    string `orm:"source_title"`
		Count    int    `orm:"count"`
	}
	// Group by the actual chat, not rule/source or mutable title. A Bot source
	// may collect many chats, and one chat may match several rules.
	sourceSQL := `WITH material AS (
 SELECT d.source_id,d.profile_id,d.id,
 COALESCE(NULLIF(d.source_chat_id_snapshot,''),CONCAT('source:',d.source_id)) AS chat_key,
 COALESCE(NULLIF(d.source_name_snapshot,''),NULLIF(s.title,''),NULLIF(d.source_chat_id_snapshot,''),'未知来源频道') AS title
 FROM hg_youban_publish_collect_dispatch d
 LEFT JOIN hg_youban_publish_collect_source s ON s.id=d.source_id AND s.tenant_id=d.tenant_id AND s.deleted_at IS NULL
 WHERE d.tenant_id=? AND d.account_id=? AND d.status=? AND d.created_at>=? AND d.created_at<? AND d.profile_id>0
 ), named AS (
 SELECT *,ROW_NUMBER() OVER(PARTITION BY chat_key ORDER BY id DESC) AS title_rank FROM material
 )
 SELECT MIN(source_id) AS source_id,chat_key AS source_chat_id,
 MAX(CASE WHEN title_rank=1 THEN title END) AS source_title,
 COUNT(DISTINCT profile_id) AS count
 FROM named GROUP BY chat_key ORDER BY count DESC,chat_key`
	if err = g.DB().GetScan(ctx, &sources, sourceSQL, tenantID, accountID, sysin.CollectDispatchStatusSent, start, end); err != nil {
		return nil, gerror.Wrap(err, "统计来源频道采集量失败")
	}
	for _, row := range sources {
		report.Sources = append(report.Sources, &sysin.DailyReportSourceRow{SourceID: row.SourceID, SourceChatID: row.ChatID, SourceTitle: row.Title, Count: row.Count})
	}
	var total struct {
		Count int `orm:"count"`
	}
	if err = g.DB().Model("hg_youban_publish_collect_dispatch").Safe().Ctx(ctx).
		Fields("COUNT(DISTINCT profile_id) AS count").Where("tenant_id", tenantID).Where("account_id", accountID).
		Where("status", sysin.CollectDispatchStatusSent).WhereGTE("created_at", start).WhereLT("created_at", end).
		WhereGT("profile_id", 0).Scan(&total); err != nil {
		return nil, gerror.Wrap(err, "统计采集资料总量失败")
	}
	report.CollectedTotal = total.Count

	var counts []struct {
		Action                      string
		Success, Failed, Processing int
	}
	if err = g.DB().Model(publishSuccessRecordTable+" r").Safe().Ctx(ctx).
		LeftJoin(publishTgJobTable+" j", dailyReportJobJoin()).
		Fields("r.action AS action,SUM(CASE WHEN r.status='success' THEN 1 ELSE 0 END) AS success, SUM(CASE WHEN r.status='failed' THEN 1 ELSE 0 END) AS failed, SUM(CASE WHEN r.status IN ('pending','sending','unknown','failed_retry') THEN 1 ELSE 0 END) AS processing").
		Where("r.tenant_id", tenantID).Where("r.account_id", accountID).
		WhereIn("r.action", []string{"collect_publish", "cycle_publish"}).
		Where(dailyReportResultTime()+" >= ? AND "+dailyReportResultTime()+" < ?", start, end).Group("r.action").Scan(&counts); err != nil {
		return nil, gerror.Wrap(err, "统计日报推送结果失败")
	}
	for _, count := range counts {
		if count.Action == "cycle_publish" {
			report.CycleSuccess, report.CycleFailed, report.CycleProcessing = count.Success, count.Failed, count.Processing
		} else {
			report.PublishSuccess, report.PublishFailed, report.PublishProcessing = count.Success, count.Failed, count.Processing
		}
	}
	if err = g.DB().Model(publishSuccessRecordTable+" r").Safe().Ctx(ctx).
		LeftJoin(publishTgJobTable+" j", dailyReportJobJoin()).
		LeftJoin(publishChannelTable+" c", "c.id=r.channel_id AND c.tenant_id=r.tenant_id AND c.account_id=r.account_id").
		Fields("r.channel_id AS channel_id,COALESCE(MAX(NULLIF(c.title,'')),CONCAT('',r.channel_id)) AS title,r.action AS action,SUM(CASE WHEN r.status='success' THEN 1 ELSE 0 END) AS success,SUM(CASE WHEN r.status='failed' THEN 1 ELSE 0 END) AS failed,SUM(CASE WHEN r.status IN ('pending','sending','unknown','failed_retry') THEN 1 ELSE 0 END) AS processing").
		Where("r.tenant_id", tenantID).Where("r.account_id", accountID).
		WhereIn("r.action", []string{"collect_publish", "cycle_publish"}).
		Where(dailyReportResultTime()+" >= ? AND "+dailyReportResultTime()+" < ?", start, end).
		Group("r.channel_id,r.action").Order("r.channel_id,r.action").Scan(&report.Channels); err != nil {
		return nil, gerror.Wrap(err, "统计日报目标频道失败")
	}

	return report, nil
}

func (s *sSysPublish) SaveDailyReport(ctx context.Context, report *sysin.DailyReportModel) error {
	if report == nil || report.TenantID <= 0 || report.AccountID <= 0 || report.ReportDate == "" {
		return gerror.New("日报数据不完整")
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return gerror.Wrap(err, "序列化日报明细失败")
	}
	_, err = pdao.YoubanPublishDailyReport.Ctx(ctx).Data(do.YoubanPublishDailyReport{
		TenantId: report.TenantID, AccountId: report.AccountID, ReportDate: gtime.New(report.ReportDate),
		CollectedTotal: report.CollectedTotal, PublishSuccess: report.PublishSuccess, PublishFailed: report.PublishFailed,
		PublishProcessing: report.PublishProcessing, SourceBreakdownJson: string(payload), GeneratedAt: gtime.New(report.GeneratedAt),
	}).OnConflict("tenant_id,account_id,report_date").OnDuplicate("tenant_id").Save()
	if err != nil {
		return gerror.Wrap(err, "保存日报失败")
	}
	return nil
}

func (s *sSysPublish) DailyReports(ctx context.Context, in *sysin.DailyReportListInp) ([]*sysin.DailyReportModel, error) {
	if in == nil || in.TenantID <= 0 || in.AccountID <= 0 {
		return nil, gerror.New("日报查询参数不合法")
	}
	days := in.Days
	if days <= 0 || days > 7 {
		days = 7
	}
	var rows []struct {
		ReportDate string `orm:"report_date"`
		Payload    string `orm:"source_breakdown_json"`
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return nil, gerror.Wrap(err, "加载中国时区失败")
	}
	today := time.Now().In(loc)
	oldest := dailyReportOldestDate(today, days)
	if err := pdao.YoubanPublishDailyReport.Ctx(ctx).Fields("report_date,source_breakdown_json").Where("tenant_id", in.TenantID).Where("account_id", in.AccountID).WhereGTE("report_date", oldest).WhereLT("report_date", today.Format("2006-01-02")).OrderDesc("report_date").Limit(days).Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取日报失败")
	}
	list := make([]*sysin.DailyReportModel, 0, len(rows))
	for _, row := range rows {
		var item sysin.DailyReportModel
		if err := json.Unmarshal([]byte(row.Payload), &item); err != nil {
			return nil, gerror.Wrap(err, "解析日报失败")
		}
		list = append(list, &item)
	}
	return list, nil
}

func (s *sSysPublish) GenerateDailyReports(ctx context.Context, reportDate time.Time, limit int) error {
	_, err := s.generateDailyReportBatch(ctx, reportDate, limit)
	return err
}

func (s *sSysPublish) generateDailyReportBatch(ctx context.Context, reportDate time.Time, limit int) (int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return 0, err
	}
	day := reportDate.In(loc)
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)
	// Include enabled configurations even when they collected/sent zero groups.
	// Existing snapshots are excluded, making repeated minute polls inexpensive.
	query := `SELECT a.tenant_id,a.id AS account_id FROM hg_youban_publish_account a
 WHERE a.status=1 AND a.deleted_at IS NULL AND a.account_type='admin' AND a.created_at<?
 AND NOT EXISTS(SELECT 1 FROM hg_youban_publish_daily_report r WHERE r.tenant_id=a.tenant_id AND r.account_id=a.id AND r.report_date=?)
 AND (EXISTS(SELECT 1 FROM hg_youban_publish_collect_source s WHERE s.tenant_id=a.tenant_id AND s.account_id=a.id AND s.status=1 AND s.deleted_at IS NULL AND s.created_at<?)
 OR EXISTS(SELECT 1 FROM hg_youban_publish_channel c WHERE c.tenant_id=a.tenant_id AND c.tg_account_id=a.id AND c.cycle_publish_enabled=1 AND c.deleted_at IS NULL AND c.created_at<?)
 OR EXISTS(SELECT 1 FROM hg_youban_publish_collect_dispatch d WHERE d.tenant_id=a.tenant_id AND d.account_id=a.id AND d.created_at>=? AND d.created_at<?)
 OR EXISTS(SELECT 1 FROM hg_youban_publish_success_record r LEFT JOIN hg_youban_publish_tg_job j ON ` + dailyReportJobJoin() + ` WHERE r.tenant_id=a.tenant_id AND r.account_id=a.id AND ` + dailyReportResultTime() + `>=? AND ` + dailyReportResultTime() + `<? AND r.action IN ('collect_publish','cycle_publish')))
 ORDER BY a.tenant_id,a.id LIMIT ?`
	rows, err := g.DB().GetAll(ctx, query, end, start.Format("2006-01-02"), end, end, start, end, start, end, limit)
	if err != nil {
		return 0, gerror.Wrap(err, "读取日报账号失败")
	}
	count := 0
	for _, owner := range rows {
		report, err := s.BuildDailyReport(ctx, owner["tenant_id"].Int64(), owner["account_id"].Int64(), start)
		if err != nil {
			return count, err
		}
		if err = s.SaveDailyReport(ctx, report); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func dailyReportSourceText(report *sysin.DailyReportModel) string {
	if report == nil {
		return ""
	}
	return fmt.Sprintf("%s 来源频道:%d", report.ReportDate, len(report.Sources))
}

// Daily reports describe completed calendar days; today is never included.
func dailyReportOldestDate(today time.Time, days int) string {
	if days < 1 || days > 7 {
		days = 7
	}
	return time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location()).AddDate(0, 0, -days).Format("2006-01-02")
}

// The success projection keeps its first-created timestamp across retries.
// Successful sends belong to sent_at's day, not the day they were enqueued.
func dailyReportResultTime() string {
	return "CASE WHEN r.status='success' THEN COALESCE(j.sent_at,r.created_at) WHEN r.status='failed' THEN COALESCE(j.updated_at,r.created_at) ELSE r.created_at END"
}
func dailyReportJobJoin() string {
	return "j.id=r.job_id AND j.tenant_id=r.tenant_id AND j.account_id=r.account_id"
}
