package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const dailyReportTable = "hg_youban_publish_daily_report"

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
	if err = g.DB().Model("hg_youban_publish_collect_dispatch d").Safe().Ctx(ctx).
		LeftJoin("hg_youban_publish_collect_source s", "s.id=d.source_id AND s.tenant_id=d.tenant_id AND s.deleted_at IS NULL").
		Fields("d.source_id,COALESCE(NULLIF(d.source_chat_id_snapshot,''),'-') AS source_chat_id,COALESCE(NULLIF(d.source_name_snapshot,''),NULLIF(s.title,''),NULLIF(d.source_chat_id_snapshot,''),'未知来源频道') AS source_title,COUNT(DISTINCT d.profile_id) AS count").
		Where("d.tenant_id", tenantID).Where("d.account_id", accountID).
		Where("d.status", sysin.CollectDispatchStatusSent).
		WhereGTE("d.created_at", start).WhereLT("d.created_at", end).
		WhereGT("d.profile_id", 0).Group("d.source_id,source_chat_id,source_title").OrderDesc("count").Scan(&sources); err != nil {
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

	var counts struct{ Success, Failed, Processing int }
	if err = g.DB().Model(publishSuccessRecordTable).Safe().Ctx(ctx).
		Fields("SUM(CASE WHEN status='success' THEN 1 ELSE 0 END) AS success, SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END) AS failed, SUM(CASE WHEN status IN ('pending','sending','unknown','failed_retry') THEN 1 ELSE 0 END) AS processing").
		Where("tenant_id", tenantID).Where("account_id", accountID).
		WhereIn("action", []string{"collect_publish", "cycle_publish"}).
		WhereGTE("created_at", start).WhereLT("created_at", end).Scan(&counts); err != nil {
		return nil, gerror.Wrap(err, "统计日报推送结果失败")
	}
	report.PublishSuccess, report.PublishFailed, report.PublishProcessing = counts.Success, counts.Failed, counts.Processing
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
	_, err = g.DB().Model(dailyReportTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id": report.TenantID, "account_id": report.AccountID, "report_date": report.ReportDate,
		"collected_total": report.CollectedTotal, "publish_success": report.PublishSuccess, "publish_failed": report.PublishFailed,
		"publish_processing": report.PublishProcessing, "source_breakdown_json": string(payload), "generated_at": report.GeneratedAt,
	}).OnConflict("tenant_id,account_id,report_date").OnDuplicate("collected_total,publish_success,publish_failed,publish_processing,source_breakdown_json,generated_at").Save()
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
	oldest := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	if err := g.DB().Model(dailyReportTable).Safe().Ctx(ctx).Fields("report_date,source_breakdown_json").Where("tenant_id", in.TenantID).Where("account_id", in.AccountID).WhereGTE("report_date", oldest).OrderDesc("report_date").Limit(days).Scan(&rows); err != nil {
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
	if limit <= 0 {
		limit = 500
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return err
	}
	day := reportDate.In(loc)
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)
	var owners []struct {
		TenantID  int64 `orm:"tenant_id"`
		AccountID int64 `orm:"account_id"`
	}
	ownerSQL := `SELECT tenant_id,account_id FROM (SELECT tenant_id,account_id FROM hg_youban_publish_collect_dispatch WHERE created_at>=? AND created_at<? GROUP BY tenant_id,account_id UNION SELECT tenant_id,account_id FROM hg_youban_publish_success_record WHERE created_at>=? AND created_at<? AND action IN ('collect_publish','cycle_publish') GROUP BY tenant_id,account_id) owners ORDER BY tenant_id,account_id LIMIT ? OFFSET ?`
	for offset := 0; ; offset += limit {
		rows, queryErr := g.DB().GetAll(ctx, ownerSQL, start, end, start, end, limit, offset)
		if queryErr != nil {
			return gerror.Wrap(queryErr, "读取日报账号失败")
		}
		for _, row := range rows {
			owners = append(owners, struct {
				TenantID  int64 `orm:"tenant_id"`
				AccountID int64 `orm:"account_id"`
			}{TenantID: row["tenant_id"].Int64(), AccountID: row["account_id"].Int64()})
		}
		if len(rows) < limit {
			break
		}
	}
	for _, owner := range owners {
		report, buildErr := s.BuildDailyReport(ctx, owner.TenantID, owner.AccountID, day)
		if buildErr != nil {
			return buildErr
		}
		if saveErr := s.SaveDailyReport(ctx, report); saveErr != nil {
			return saveErr
		}
	}
	_, _ = g.DB().Model(dailyReportTable).Safe().Ctx(ctx).WhereLT("report_date", day.AddDate(0, 0, -7).Format("2006-01-02")).Delete()
	return nil
}

func dailyReportSourceText(report *sysin.DailyReportModel) string {
	if report == nil {
		return ""
	}
	return fmt.Sprintf("%s 来源频道:%d", report.ReportDate, len(report.Sources))
}
