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
	// Group by the actual chat, not rule/source or mutable title. A Bot source
	// may collect many chats, and one chat may match several rules.
	sourceSQL := `WITH material AS (
 SELECT d.source_id,d.profile_id,d.id,
 COALESCE(NULLIF(d.source_chat_id_snapshot,''),'source:' || CAST(d.source_id AS text)) AS chat_key,
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
	if err = g.DB().Model(publishSuccessRecordTable).Safe().Ctx(ctx).
		Fields("action,SUM(CASE WHEN status='success' THEN 1 ELSE 0 END) AS success, SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END) AS failed, SUM(CASE WHEN status IN ('pending','sending','unknown','failed_retry') THEN 1 ELSE 0 END) AS processing").
		Where("tenant_id", tenantID).Where("account_id", accountID).
		WhereIn("action", []string{"collect_publish", "cycle_publish"}).
		WhereGTE("created_at", start).WhereLT("created_at", end).Group("action").Scan(&counts); err != nil {
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
		LeftJoin(publishChannelTable+" c", "c.id=r.channel_id AND c.tenant_id=r.tenant_id AND c.account_id=r.account_id").
		Fields("r.channel_id AS channel_id,COALESCE(MAX(NULLIF(c.title,'')),CAST(r.channel_id AS text)) AS title,r.action AS action,SUM(CASE WHEN r.status='success' THEN 1 ELSE 0 END) AS success,SUM(CASE WHEN r.status='failed' THEN 1 ELSE 0 END) AS failed,SUM(CASE WHEN r.status IN ('pending','sending','unknown','failed_retry') THEN 1 ELSE 0 END) AS processing").
		Where("r.tenant_id", tenantID).Where("r.account_id", accountID).
		WhereIn("r.action", []string{"collect_publish", "cycle_publish"}).
		WhereGTE("r.created_at", start).WhereLT("r.created_at", end).
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
	oldest := dailyReportOldestDate(today, days)
	if err := g.DB().Model(dailyReportTable).Safe().Ctx(ctx).Fields("report_date,source_breakdown_json").Where("tenant_id", in.TenantID).Where("account_id", in.AccountID).WhereGTE("report_date", oldest).WhereLT("report_date", today.Format("2006-01-02")).OrderDesc("report_date").Limit(days).Scan(&rows); err != nil {
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
	_, err = g.DB().Model(dailyReportTable).Safe().Ctx(ctx).WhereLT("report_date", dailyReportOldestDate(time.Now().In(loc), 7)).Delete()
	if err != nil {
		return gerror.Wrap(err, "清理过期日报失败")
	}
	return nil
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
