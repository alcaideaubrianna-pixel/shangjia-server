package sys

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"math/rand"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
	"hotgo/internal/library/storager"
	"hotgo/internal/model"
	"hotgo/internal/service"
	"hotgo/utility/file"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/gclient"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gtime"
)

const (
	importStageCreated  = "created"
	importStageLogin    = "login"
	importStageList     = "list"
	importStageDetail   = "detail"
	importStageMedia    = "media_storage"
	importStageTgMatch  = "tg_match"
	importStageFinished = "finished"

	legacyCMSDefaultPerPage = 12
	legacyCMSRequestMinGap  = 1200 * time.Millisecond
	legacyCMSRequestJitter  = 900 * time.Millisecond
	legacyCMSRequestRetries = 2

	importRunTable    = "hg_youban_publish_import_run"
	importRunLogTable = "hg_youban_publish_import_run_log"
)

func (s *sSysPublish) AdminImportTaskList(ctx context.Context, in *sysin.ImportTaskListInp) (list []*sysin.ImportTaskModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ImportTaskListInp{}
	}
	in.TenantId = account.TenantId
	return s.importTaskList(ctx, in)
}

func (s *sSysPublish) ServerImportTaskList(ctx context.Context, in *sysin.ImportTaskListInp) (list []*sysin.ImportTaskModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.ImportTaskListInp{}
	}
	return s.importTaskList(ctx, in)
}

func (s *sSysPublish) MyImportTaskList(ctx context.Context, in *sysin.ImportTaskListInp) (list []*sysin.ImportTaskModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ImportTaskListInp{}
	}
	in.TenantId = account.TenantId
	if in.AccountId <= 0 {
		in.AccountId = account.Id
	}
	return s.importTaskList(ctx, in)
}

func (s *sSysPublish) AdminImportTaskCreate(ctx context.Context, in *sysin.ImportTaskCreateInp) (id int64, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return 0, err
	}
	if in == nil {
		return 0, gerror.New("导入任务参数不能为空")
	}
	in.TenantId = account.TenantId
	return s.createImportTask(ctx, in, account.Id)
}

func (s *sSysPublish) ServerImportTaskCreate(ctx context.Context, in *sysin.ImportTaskCreateInp) (id int64, err error) {
	return s.createImportTask(ctx, in, contexts.GetUserId(ctx))
}

func (s *sSysPublish) createImportTask(ctx context.Context, in *sysin.ImportTaskCreateInp, operatorId int64) (id int64, err error) {
	if in == nil {
		return 0, gerror.New("导入任务参数不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return 0, err
	}
	if in.TenantId <= 0 {
		return 0, gerror.New("请选择账号归属")
	}
	if in.AccountId <= 0 {
		return 0, gerror.New("请选择导入到哪个上架账号")
	}
	if err = s.ensureTenant(ctx, in.TenantId); err != nil {
		return 0, err
	}
	if err = s.ensureAccountBelongsTenant(ctx, in.AccountId, in.TenantId); err != nil {
		return 0, err
	}
	if len(in.ChannelIds) > 0 {
		if err = s.ensureChannelsBelongTenant(ctx, in.ChannelIds, in.TenantId); err != nil {
			return 0, err
		}
	}
	channelJSON, err := json.Marshal(uniqueIds(in.ChannelIds))
	if err != nil {
		return 0, err
	}
	now := gtime.Now()
	data := g.Map{
		"tenant_id":         in.TenantId,
		"account_id":        in.AccountId,
		"source_name":       in.SourceName,
		"base_url":          in.BaseUrl,
		"username":          in.Username,
		"limit_count":       in.LimitCount,
		"per_page":          in.PerPage,
		"proxy_enabled":     in.ProxyEnabled,
		"proxy_pool":        in.ProxyPool,
		"media_concurrency": in.MediaConcurrency,
		"channel_id_json":   string(channelJSON),
		"remark":            in.Remark,
		"updated_by":        operatorId,
		"updated_at":        now,
	}
	if in.Password != "" {
		data["password_cipher"] = encodeImportPassword(in.Password)
	}
	control, _ := json.Marshal(g.Map{"importMode": in.ImportMode})
	data["result_json"] = string(control)
	if len(in.TgRange) == 2 {
		data["tg_start_at"] = in.TgRange[0]
		data["tg_end_at"] = in.TgRange[1]
	}
	if in.Id > 0 {
		_, err = pdao.YoubanPublishImportTask.Ctx(ctx).Where("id", in.Id).WhereNull("deleted_at").Data(data).Update()
		if err != nil {
			return 0, gerror.Wrap(err, "更新旧站导入任务失败")
		}
		return in.Id, nil
	}
	if in.Password == "" {
		return 0, gerror.New("旧站密码不能为空")
	}
	data["created_by"] = operatorId
	data["created_at"] = now
	data["status"] = sysin.ImportTaskStatusPending
	data["stage"] = importStageCreated
	id, err = pdao.YoubanPublishImportTask.Ctx(ctx).Data(data).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "创建旧站导入任务失败")
	}
	return id, nil
}

func (s *sSysPublish) ServerImportTaskView(ctx context.Context, in *sysin.ImportTaskViewInp) (res *sysin.ImportTaskModel, err error) {
	return s.importTaskView(ctx, in.Id, 0, 0)
}

func (s *sSysPublish) AdminImportTaskView(ctx context.Context, in *sysin.ImportTaskViewInp) (res *sysin.ImportTaskModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.importTaskView(ctx, in.Id, account.TenantId, 0)
}

func (s *sSysPublish) MyImportTaskView(ctx context.Context, in *sysin.ImportTaskViewInp) (res *sysin.ImportTaskModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.importTaskView(ctx, in.Id, account.TenantId, account.Id)
}

func (s *sSysPublish) MyImportRunList(ctx context.Context, in *sysin.ImportRunListInp) ([]*sysin.ImportRunModel, int, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ImportRunListInp{}
	}
	in.TenantId = account.TenantId
	in.AccountId = account.Id
	return s.ServerImportRunList(ctx, in)
}

func (s *sSysPublish) MyImportRunLogList(ctx context.Context, in *sysin.ImportRunLogListInp) ([]*sysin.ImportRunLogModel, int, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		return nil, 0, gerror.New("导入记录参数不能为空")
	}
	if err = s.ensureImportRunBelongAccount(ctx, in.RunId, account.TenantId, account.Id); err != nil {
		return nil, 0, err
	}
	return s.ServerImportRunLogList(ctx, in)
}

func (s *sSysPublish) ServerImportTaskStart(ctx context.Context, in *sysin.ImportTaskActionInp) error {
	_, err := s.ServerImportRunCreate(ctx, &sysin.ImportRunCreateInp{
		TaskId:  in.Id,
		RunType: sysin.ImportRunTypeImport,
	})
	return err
}

func (s *sSysPublish) ServerImportRunCreate(ctx context.Context, in *sysin.ImportRunCreateInp) (int64, error) {
	if err := ensureImportRunTables(ctx); err != nil {
		return 0, err
	}
	if err := in.Filter(ctx); err != nil {
		return 0, err
	}
	row, err := s.importTaskRow(ctx, in.TaskId)
	if err != nil {
		return 0, err
	}
	params, _ := json.Marshal(g.Map{
		"importMode":  in.ImportMode,
		"scanMode":    in.ScanMode,
		"recentCount": in.RecentCount,
	})
	now := gtime.Now()
	id, err := g.DB().Model(importRunTable).Safe().Ctx(ctx).Data(g.Map{
		"task_id":               row["id"].Int64(),
		"tenant_id":             row["tenant_id"].Int64(),
		"account_id":            row["account_id"].Int64(),
		"source_name":           row["source_name"].String(),
		"base_url":              row["base_url"].String(),
		"username":              row["username"].String(),
		"run_type":              in.RunType,
		"import_mode":           in.ImportMode,
		"scan_mode":             in.ScanMode,
		"recent_count":          in.RecentCount,
		"status":                sysin.ImportTaskStatusPending,
		"stage":                 importStageCreated,
		"media_missing_storage": 0,
		"params_json":           string(params),
		"created_by":            contexts.GetUserId(ctx),
		"updated_by":            contexts.GetUserId(ctx),
		"created_at":            now,
		"updated_at":            now,
	}).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "创建导入执行记录失败")
	}
	_ = s.appendImportRunLog(ctx, id, "info", importStageCreated, "执行记录已创建", nil)
	return id, s.enqueueImportRun(ctx, id, 0)
}

func (s *sSysPublish) AdminImportTaskStart(ctx context.Context, in *sysin.ImportTaskActionInp) error {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if err = s.ensureImportTaskBelongTenant(ctx, in.Id, account.TenantId); err != nil {
		return err
	}
	if err = s.resetImportTask(ctx, in.Id, account.Id); err != nil {
		return err
	}
	return s.enqueueImportTask(ctx, in.Id, 0)
}

func (s *sSysPublish) AdminImportTaskRetry(ctx context.Context, in *sysin.ImportTaskActionInp) error {
	return s.AdminImportTaskStart(ctx, in)
}

func (s *sSysPublish) ServerImportTaskRetry(ctx context.Context, in *sysin.ImportTaskActionInp) error {
	return s.ServerImportTaskStart(ctx, in)
}

func (s *sSysPublish) ServerImportTaskScan(ctx context.Context, in *sysin.ImportTaskScanInp) (*sysin.ImportTaskScanModel, error) {
	if in == nil {
		return nil, gerror.New("扫描参数不能为空")
	}
	if err := in.Filter(ctx); err != nil {
		return nil, err
	}
	row, err := s.importTaskRow(ctx, in.Id)
	if err != nil {
		return nil, err
	}
	importer := newLegacyCMSImporter(row)
	if err = importer.login(ctx); err != nil {
		return nil, err
	}
	sourceIds, err := importer.collectSourceIds(ctx, in.ScanMode, in.RecentCount, 0)
	if err != nil {
		return nil, err
	}
	res := &sysin.ImportTaskScanModel{
		TaskId:      in.Id,
		ScanMode:    in.ScanMode,
		RecentCount: in.RecentCount,
		SourceTotal: len(sourceIds),
		Items:       make([]*sysin.ImportTaskScanItem, 0, len(sourceIds)),
		ScannedAt:   gtime.Now(),
	}
	for _, sourceId := range sourceIds {
		item, itemErr := s.scanImportTaskSourceItem(ctx, row, sourceId)
		if itemErr != nil {
			return nil, itemErr
		}
		res.Items = append(res.Items, item)
		if item.Status == "existing" {
			res.ExistingTotal++
		} else {
			res.MissingTotal++
		}
		res.MediaTotal += item.MediaTotal
		res.MediaMissingStorage += item.MediaMissingStorage
		if item.Status == "missing" || item.MediaMissingStorage > 0 {
			res.CanRepairTotal++
		}
	}
	return res, nil
}

func (s *sSysPublish) ServerImportTaskRepair(ctx context.Context, in *sysin.ImportTaskRepairInp) error {
	_, err := s.ServerImportRunCreate(ctx, &sysin.ImportRunCreateInp{
		TaskId:      in.Id,
		RunType:     sysin.ImportRunTypeRepair,
		ImportMode:  in.ImportMode,
		ScanMode:    in.ScanMode,
		RecentCount: in.RecentCount,
	})
	return err
}

func (s *sSysPublish) ServerImportRunList(ctx context.Context, in *sysin.ImportRunListInp) ([]*sysin.ImportRunModel, int, error) {
	if err := ensureImportRunTables(ctx); err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ImportRunListInp{}
	}
	mod := g.DB().Model(importRunTable).Safe().Ctx(ctx).As("r").
		LeftJoin(publishTenantTable+" m", "m.id=r.tenant_id").
		LeftJoin(publishAccountTable+" a", "a.id=r.account_id").
		WhereNull("r.deleted_at")
	if in.TaskId > 0 {
		mod = mod.Where("r.task_id", in.TaskId)
	}
	if in.TenantId > 0 {
		mod = mod.Where("r.tenant_id", in.TenantId)
	}
	if in.AccountId > 0 {
		mod = mod.Where("r.account_id", in.AccountId)
	}
	if in.RunType != "" {
		mod = mod.Where("r.run_type", in.RunType)
	}
	if in.Status != "" {
		mod = mod.Where("r.status", in.Status)
	}
	if strings.TrimSpace(in.Keyword) != "" {
		kw := "%" + strings.TrimSpace(in.Keyword) + "%"
		mod = mod.Where("(r.base_url LIKE ? OR r.username LIKE ?)", kw, kw)
	}
	total, err := mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "统计导入记录失败")
	}
	if total == 0 {
		return []*sysin.ImportRunModel{}, 0, nil
	}
	var list []*sysin.ImportRunModel
	if err = mod.Fields("r.*,m.name AS tenant_name,a.nickname AS account_name").
		Page(in.Page, in.PerPage).
		OrderDesc("r.id").
		Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取导入记录失败")
	}
	for _, item := range list {
		if item != nil && item.ProgressTotal > 0 {
			item.Percent = float64(item.ProgressDone) * 100 / float64(item.ProgressTotal)
		}
	}
	if err = s.applyImportRunOwnerNames(ctx, list); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (s *sSysPublish) ServerImportRunDelete(ctx context.Context, in *sysin.ImportRunActionInp) error {
	if err := ensureImportRunTables(ctx); err != nil {
		return err
	}
	if in == nil {
		return gerror.New("导入记录参数不能为空")
	}
	_, err := g.DB().Model(importRunTable).Safe().Ctx(ctx).Where("id", in.Id).Data(g.Map{
		"deleted_by": contexts.GetUserId(ctx),
		"deleted_at": gtime.Now(),
		"updated_at": gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "删除导入记录失败")
	}
	_, _ = g.DB().Model(importRunLogTable).Safe().Ctx(ctx).Where("run_id", in.Id).Delete()
	return nil
}

func (s *sSysPublish) ServerImportRunCancel(ctx context.Context, in *sysin.ImportRunActionInp) error {
	if err := ensureImportRunTables(ctx); err != nil {
		return err
	}
	if in == nil {
		return gerror.New("导入记录参数不能为空")
	}
	now := gtime.Now()
	_, err := g.DB().Model(importRunTable).Safe().Ctx(ctx).
		Where("id", in.Id).
		WhereNull("deleted_at").
		Data(g.Map{
			"status":      sysin.ImportTaskStatusCanceled,
			"stage":       "canceled",
			"finished_at": now,
			"updated_by":  contexts.GetUserId(ctx),
			"updated_at":  now,
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "取消导入记录失败")
	}
	_ = s.appendImportRunLog(ctx, in.Id, "warning", "canceled", "导入记录已取消", nil)
	return nil
}

func (s *sSysPublish) ServerImportRunLogList(ctx context.Context, in *sysin.ImportRunLogListInp) ([]*sysin.ImportRunLogModel, int, error) {
	if err := ensureImportRunTables(ctx); err != nil {
		return nil, 0, err
	}
	if in == nil {
		return nil, 0, gerror.New("导入记录参数不能为空")
	}
	mod := g.DB().Model(importRunLogTable).Safe().Ctx(ctx).Where("run_id", in.RunId)
	total, err := mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "统计导入日志失败")
	}
	var list []*sysin.ImportRunLogModel
	if total > 0 {
		if err = mod.Page(in.Page, in.PerPage).OrderDesc("id").Scan(&list); err != nil {
			return nil, 0, gerror.Wrap(err, "获取导入日志失败")
		}
	}
	return list, total, nil
}

func (s *sSysPublish) ServerImportRunLogClear(ctx context.Context, in *sysin.ImportRunActionInp) error {
	if err := ensureImportRunTables(ctx); err != nil {
		return err
	}
	if in == nil {
		return gerror.New("导入记录参数不能为空")
	}
	_, err := g.DB().Model(importRunLogTable).Safe().Ctx(ctx).Where("run_id", in.Id).Delete()
	if err != nil {
		return gerror.Wrap(err, "清理导入日志失败")
	}
	return nil
}

func (s *sSysPublish) ServerImportTaskCancel(ctx context.Context, in *sysin.ImportTaskActionInp) error {
	if err := s.ensureImportTaskExists(ctx, in.Id); err != nil {
		return err
	}
	_, err := pdao.YoubanPublishImportTask.Ctx(ctx).
		Where("id", in.Id).
		Data(g.Map{
			"status":      sysin.ImportTaskStatusCanceled,
			"stage":       "canceled",
			"finished_at": gtime.Now(),
			"updated_by":  contexts.GetUserId(ctx),
			"updated_at":  gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "取消旧站导入任务失败")
	}
	return nil
}

func (s *sSysPublish) AdminImportTaskCancel(ctx context.Context, in *sysin.ImportTaskActionInp) error {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if err = s.ensureImportTaskBelongTenant(ctx, in.Id, account.TenantId); err != nil {
		return err
	}
	_, err = pdao.YoubanPublishImportTask.Ctx(ctx).
		Where("id", in.Id).
		Where("tenant_id", account.TenantId).
		Data(g.Map{
			"status":      sysin.ImportTaskStatusCanceled,
			"stage":       "canceled",
			"finished_at": gtime.Now(),
			"updated_by":  account.Id,
			"updated_at":  gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "取消旧站导入任务失败")
	}
	return nil
}

func (s *sSysPublish) ExecuteImportTask(ctx context.Context, id int64) (err error) {
	row, err := pdao.YoubanPublishImportTask.Ctx(ctx).Where("id", id).WhereNull("deleted_at").One()
	if err != nil {
		return gerror.Wrap(err, "读取旧站导入任务失败")
	}
	if row.IsEmpty() || row["status"].String() == sysin.ImportTaskStatusRunning || row["status"].String() == sysin.ImportTaskStatusCanceled {
		return nil
	}
	ctx = importRuntimeContext(ctx, row["created_by"].Int64())
	startedAt := gtime.Now()
	if err = s.updateImportTaskProgress(ctx, id, g.Map{
		"status":        sysin.ImportTaskStatusRunning,
		"stage":         importStageLogin,
		"error_message": "",
		"started_at":    startedAt,
		"updated_at":    startedAt,
	}); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = s.updateImportTaskProgress(ctx, id, g.Map{
				"status":        sysin.ImportTaskStatusFailed,
				"error_message": err.Error(),
				"finished_at":   gtime.Now(),
				"updated_at":    gtime.Now(),
			})
		}
	}()

	importer := newLegacyCMSImporter(row)
	if err = importer.login(ctx); err != nil {
		return err
	}
	if err = s.updateImportTaskProgress(ctx, id, g.Map{"stage": importStageList, "updated_at": gtime.Now()}); err != nil {
		return err
	}
	page, err := importer.fetchListPage(ctx, 1)
	if err != nil {
		return err
	}
	total := page.ItemTotal
	if row["limit_count"].Int() > 0 && (total == 0 || total > row["limit_count"].Int()) {
		total = row["limit_count"].Int()
	}
	if total <= 0 {
		total = len(page.Items)
	}
	_ = s.updateImportTaskProgress(ctx, id, g.Map{
		"page_total":     page.PageTotal,
		"item_total":     total,
		"progress_total": total * 2,
		"updated_at":     gtime.Now(),
	})

	// 第一版先完成任务框架、登录和列表采集进度；资料落库、媒体转存和TG匹配继续在该执行器内扩展。
	result, _ := json.Marshal(g.Map{"listCount": len(page.Items), "message": "旧站登录和列表采集完成"})
	return s.updateImportTaskProgress(ctx, id, g.Map{
		"status":        sysin.ImportTaskStatusSuccess,
		"stage":         importStageFinished,
		"page_done":     1,
		"item_done":     len(page.Items),
		"progress_done": len(page.Items),
		"result_json":   string(result),
		"finished_at":   gtime.Now(),
		"updated_at":    gtime.Now(),
	})
}

func (s *sSysPublish) ExecuteImportRun(ctx context.Context, runId int64) (err error) {
	if err = ensureImportRunTables(ctx); err != nil {
		return err
	}
	run, err := g.DB().Model(importRunTable).Safe().Ctx(ctx).Where("id", runId).WhereNull("deleted_at").One()
	if err != nil {
		return gerror.Wrap(err, "读取导入执行记录失败")
	}
	if run.IsEmpty() || run["status"].String() == sysin.ImportTaskStatusRunning || run["status"].String() == sysin.ImportTaskStatusCanceled {
		return nil
	}
	ctx = importRuntimeContext(ctx, run["created_by"].Int64())
	task, err := s.importTaskRow(ctx, run["task_id"].Int64())
	if err != nil {
		return err
	}
	startedAt := gtime.Now()
	if err = s.updateImportRunProgress(ctx, runId, g.Map{
		"status":        sysin.ImportTaskStatusRunning,
		"stage":         importStageLogin,
		"error_message": "",
		"started_at":    startedAt,
		"updated_at":    startedAt,
	}); err != nil {
		return err
	}
	_ = s.appendImportRunLog(ctx, runId, "info", importStageLogin, "开始登录旧站", nil)
	defer func() {
		if err != nil {
			_ = s.updateImportRunProgress(ctx, runId, g.Map{
				"status":        sysin.ImportTaskStatusFailed,
				"error_message": err.Error(),
				"finished_at":   gtime.Now(),
				"updated_at":    gtime.Now(),
			})
			_ = s.appendImportRunLog(ctx, runId, "error", "", err.Error(), nil)
		}
	}()

	importer := newLegacyCMSImporter(task)
	if err = importer.login(ctx); err != nil {
		return err
	}
	_ = s.appendImportRunLog(ctx, runId, "info", importStageList, "旧站登录成功，开始采集列表", nil)
	if err = s.updateImportRunProgress(ctx, runId, g.Map{"stage": importStageList, "updated_at": gtime.Now()}); err != nil {
		return err
	}
	scanMode := run["scan_mode"].String()
	if scanMode == "" {
		scanMode = sysin.ImportTaskScanModeRecent
	}
	runType := run["run_type"].String()
	importMode := run["import_mode"].String()
	if importMode == "" {
		importMode = sysin.ImportTaskModeIncremental
	}
	sourceIds, err := importer.collectSourceIds(ctx, scanMode, run["recent_count"].Int(), task["limit_count"].Int())
	if err != nil {
		return err
	}
	total := len(sourceIds)
	_ = s.updateImportRunProgress(ctx, runId, g.Map{
		"item_total":     total,
		"progress_total": total,
		"updated_at":     gtime.Now(),
	})
	scan := &sysin.ImportTaskScanModel{TaskId: task["id"].Int64(), ScanMode: scanMode, RecentCount: run["recent_count"].Int(), SourceTotal: total, Items: make([]*sysin.ImportTaskScanItem, 0, total), ScannedAt: gtime.Now()}
	imported := 0
	mediaImported := 0
	for idx, sourceId := range sourceIds {
		canceled, cancelErr := s.isImportRunCanceled(ctx, runId)
		if cancelErr != nil {
			return cancelErr
		}
		if canceled {
			_ = s.appendImportRunLog(ctx, runId, "warning", "canceled", "导入记录已取消，停止执行", nil)
			return nil
		}
		item, itemErr := s.scanImportTaskSourceItem(ctx, task, sourceId)
		if itemErr != nil {
			return itemErr
		}
		scan.Items = append(scan.Items, item)
		if item.Status == "existing" {
			scan.ExistingTotal++
		} else {
			scan.MissingTotal++
		}
		scan.MediaTotal += item.MediaTotal
		scan.MediaMissingStorage += item.MediaMissingStorage
		if item.Status == "missing" || item.MediaMissingStorage > 0 {
			scan.CanRepairTotal++
		}
		if runType != sysin.ImportRunTypeScan {
			oldMediaTotal := item.MediaTotal
			oldMediaMissing := item.MediaMissingStorage
			_ = s.appendImportRunLog(ctx, runId, "info", importStageDetail, "开始采集笔记文本", g.Map{"sourceNoteId": sourceId, "index": idx + 1, "total": total})
			var importRes *legacyCMSImportResult
			importRes, itemErr = s.importLegacyCMSDetail(ctx, runId, importer, task, sourceId, importMode, item)
			if itemErr != nil {
				return itemErr
			}
			if importRes.Imported {
				imported++
			}
			mediaImported += importRes.MediaImported
			item.Status = "existing"
			item.TaskId = importRes.TaskId
			item.ProfileId = importRes.ProfileId
			item.MediaTotal = importRes.MediaTotal
			item.MediaMissingStorage = 0
			scan.MediaTotal += item.MediaTotal - oldMediaTotal
			scan.MediaMissingStorage -= oldMediaMissing
			if scan.MediaMissingStorage < 0 {
				scan.MediaMissingStorage = 0
			}
			_ = s.appendImportRunLog(ctx, runId, "info", importStageDetail, importRes.Message, g.Map{"sourceNoteId": sourceId, "profileId": importRes.ProfileId, "taskId": importRes.TaskId})
		}
		_ = s.updateImportRunProgress(ctx, runId, g.Map{
			"item_done":             idx + 1,
			"progress_done":         idx + 1,
			"duplicate":             scan.ExistingTotal,
			"imported":              imported,
			"media_total":           scan.MediaTotal,
			"media_missing_storage": scan.MediaMissingStorage,
			"media_done":            mediaImported,
			"media_imported":        mediaImported,
			"updated_at":            gtime.Now(),
		})
	}
	result, _ := json.Marshal(scan)
	canceled, cancelErr := s.isImportRunCanceled(ctx, runId)
	if cancelErr != nil {
		return cancelErr
	}
	if canceled {
		_ = s.appendImportRunLog(ctx, runId, "warning", "canceled", "导入记录已取消，跳过完成状态写入", nil)
		return nil
	}
	finishMessage := "扫描完成"
	if runType != sysin.ImportRunTypeScan {
		finishMessage = "导入完成"
	} else {
		finishMessage = "仅扫描完成，未导入资料"
	}
	_ = s.appendImportRunLog(ctx, runId, "info", importStageFinished, finishMessage, g.Map{"sourceTotal": total, "missing": scan.MissingTotal, "imported": imported, "mediaImported": mediaImported})
	return s.updateImportRunProgress(ctx, runId, g.Map{
		"status":      sysin.ImportTaskStatusSuccess,
		"stage":       importStageFinished,
		"result_json": string(result),
		"finished_at": gtime.Now(),
		"updated_at":  gtime.Now(),
	})
}

func (s *sSysPublish) importTaskList(ctx context.Context, in *sysin.ImportTaskListInp) (list []*sysin.ImportTaskModel, totalCount int, err error) {
	mod := pdao.YoubanPublishImportTask.Ctx(ctx).As("t").
		LeftJoin(publishTenantTable+" m", "m.id=t.tenant_id").
		LeftJoin(publishAccountTable+" a", "a.id=t.account_id").
		WhereNull("t.deleted_at")
	mod = applyImportTaskFilters(mod, in)
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "统计旧站导入任务失败")
	}
	if totalCount == 0 {
		return []*sysin.ImportTaskModel{}, 0, nil
	}
	if err = mod.Fields("t.*,m.name AS tenant_name,a.nickname AS account_name").
		Page(in.Page, in.PerPage).
		OrderDesc("t.id").
		Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取旧站导入任务失败")
	}
	if err = s.applyImportTaskOwnerNames(ctx, list); err != nil {
		return nil, 0, err
	}
	fillImportTaskPercent(list)
	return list, totalCount, nil
}

func (s *sSysPublish) importTaskView(ctx context.Context, id int64, tenantId int64, accountId int64) (*sysin.ImportTaskModel, error) {
	var item *sysin.ImportTaskModel
	mod := pdao.YoubanPublishImportTask.Ctx(ctx).As("t").
		LeftJoin(publishTenantTable+" m", "m.id=t.tenant_id").
		LeftJoin(publishAccountTable+" a", "a.id=t.account_id").
		Where("t.id", id).
		WhereNull("t.deleted_at")
	if tenantId > 0 {
		mod = mod.Where("t.tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("t.account_id", accountId)
	}
	if err := mod.Fields("t.*,m.name AS tenant_name,a.nickname AS account_name").Scan(&item); err != nil {
		return nil, gerror.Wrap(err, "获取旧站导入任务详情失败")
	}
	if item == nil {
		return nil, gerror.New("旧站导入任务不存在")
	}
	fillImportTaskPercent([]*sysin.ImportTaskModel{item})
	return item, nil
}

func (s *sSysPublish) importTaskRow(ctx context.Context, id int64) (gdb.Record, error) {
	row, err := pdao.YoubanPublishImportTask.Ctx(ctx).Where("id", id).WhereNull("deleted_at").One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取旧站导入任务失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("旧站导入任务不存在")
	}
	return row, nil
}

func (s *sSysPublish) scanImportTaskSourceItem(ctx context.Context, row gdb.Record, sourceNoteId int64) (*sysin.ImportTaskScanItem, error) {
	clientRequestId := legacyCMSClientRequestID(row, sourceNoteId)
	item := &sysin.ImportTaskScanItem{
		SourceNoteId:    sourceNoteId,
		ClientRequestId: clientRequestId,
		Status:          "missing",
	}
	task, err := pdao.YoubanPublishTask.Ctx(ctx).
		Where("tenant_id", row["tenant_id"].Int64()).
		Where("account_id", row["account_id"].Int64()).
		Where("client_request_id", clientRequestId).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "扫描本地资料任务失败")
	}
	if task.IsEmpty() {
		profile, profileErr := dao.ContentProfile.Ctx(ctx).
			Fields(dao.ContentProfile.Columns().Id).
			Where(dao.ContentProfile.Columns().SourceKey, legacyCMSProfileSourceKey(row, sourceNoteId)).
			WhereNull(dao.ContentProfile.Columns().DeletedAt).
			One()
		if profileErr != nil {
			return nil, gerror.Wrap(profileErr, "扫描本地资料失败")
		}
		if profile.IsEmpty() {
			return item, nil
		}
		item.Status = "existing"
		item.ProfileId = profile["id"].Int64()
	} else {
		item.Status = "existing"
		item.TaskId = task["id"].Int64()
		item.ProfileId = task["profile_id"].Int64()
	}
	mediaMod := pdao.YoubanPublishMedia.Ctx(ctx).
		Where("tenant_id", row["tenant_id"].Int64()).
		Where("account_id", row["account_id"].Int64()).
		WhereNull("deleted_at")
	if item.TaskId > 0 {
		mediaMod = mediaMod.Where("task_id", item.TaskId)
	} else if item.ProfileId > 0 {
		mediaMod = mediaMod.Where("profile_id", item.ProfileId)
	} else {
		return item, nil
	}
	total, err := mediaMod.Clone().Count()
	if err != nil {
		return nil, gerror.Wrap(err, "统计本地媒体失败")
	}
	missing, err := mediaMod.Clone().Where("storage_path", "").Count()
	if err != nil {
		return nil, gerror.Wrap(err, "统计未迁移到当前存储媒体失败")
	}
	item.MediaTotal = total
	item.MediaMissingStorage = missing
	return item, nil
}

type legacyCMSImportResult struct {
	TaskId        int64
	ProfileId     int64
	Imported      bool
	MediaTotal    int
	MediaImported int
	Message       string
}

type legacyCMSDetail struct {
	SourceNoteId int64
	Title        string
	PlainText    string
	Province     string
	City         string
	CreatedAt    *gtime.Time
	UpdatedAt    *gtime.Time
	Media        []*legacyCMSMedia
}

type legacyCMSMedia struct {
	URL       string
	Name      string
	MediaType string
	Purpose   string
	SortIndex int
}

func (s *sSysPublish) importLegacyCMSDetail(ctx context.Context, runId int64, importer *legacyCMSImporter, taskRow gdb.Record, sourceNoteId int64, importMode string, scanItem *sysin.ImportTaskScanItem) (*legacyCMSImportResult, error) {
	if scanItem != nil && scanItem.Status == "existing" && importMode == sysin.ImportTaskModeIncremental && scanItem.MediaMissingStorage == 0 {
		return &legacyCMSImportResult{
			TaskId:     scanItem.TaskId,
			ProfileId:  scanItem.ProfileId,
			MediaTotal: scanItem.MediaTotal,
			Message:    "本地已存在，增量模式跳过",
		}, nil
	}
	detail, err := importer.fetchDetail(ctx, sourceNoteId)
	if err != nil {
		return nil, err
	}
	detail.Title = normalizeLegacyImportedTitle(detail.Title, sourceNoteId)
	detail.Province = normalizeLegacyLocationValue(detail.Province)
	detail.City = normalizeLegacyLocationValue(detail.City)
	_ = s.appendImportRunLog(ctx, runId, "info", importStageDetail, "笔记文本采集完成", g.Map{"sourceNoteId": sourceNoteId, "title": detail.Title, "mediaTotal": len(detail.Media)})
	channelIds := decodeImportTaskChannelIds(taskRow["channel_id_json"].String())
	input := &sysin.ProfileSaveInp{
		TaskId:         scanItem.TaskId,
		Title:          detail.Title,
		Province:       detail.Province,
		City:           detail.City,
		PlainText:      detail.PlainText,
		ChannelIds:     channelIds,
		CustomerRemark: fmt.Sprintf("旧站导入：%s #%d", taskRow["base_url"].String(), sourceNoteId),
		Visibility:     consts.ContentVisibilityPrivate,
		Status:         1,
	}
	if scanItem.ProfileId > 0 {
		input.Id = scanItem.ProfileId
	}
	saved, err := s.saveProfile(ctx, input, taskRow["tenant_id"].Int64(), taskRow["account_id"].Int64())
	if err != nil {
		return nil, err
	}
	now := gtime.Now()
	sourceKey := legacyCMSProfileSourceKey(taskRow, sourceNoteId)
	clientRequestId := legacyCMSClientRequestID(taskRow, sourceNoteId)
	if _, err = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", saved.TaskId).Data(g.Map{
		"client_request_id": clientRequestId,
		"updated_at":        now,
	}).Update(); err != nil {
		return nil, gerror.Wrap(err, "更新旧站导入任务幂等键失败")
	}
	profileColumns := dao.ContentProfile.Columns()
	profileData := g.Map{
		profileColumns.Title:           detail.Title,
		profileColumns.Province:        detail.Province,
		profileColumns.City:            detail.City,
		profileColumns.SourceNoteId:    sourceNoteId,
		profileColumns.SourceKey:       sourceKey,
		profileColumns.ImportStatus:    "imported",
		profileColumns.SourceCreateBy:  taskRow["username"].String(),
		profileColumns.SourceUpdateBy:  taskRow["username"].String(),
		profileColumns.SourceCreatedAt: detail.CreatedAt,
		profileColumns.SourceUpdatedAt: detail.UpdatedAt,
		profileColumns.UpdatedAt:       now,
	}
	if _, err = dao.ContentProfile.Ctx(ctx).Where(profileColumns.Id, saved.Id).Data(profileData).Update(); err != nil {
		return nil, gerror.Wrap(err, "更新旧站资料来源信息失败")
	}
	if _, err = pdao.YoubanPublishTask.Ctx(ctx).Where("id", saved.TaskId).Data(g.Map{
		"title":      detail.Title,
		"province":   detail.Province,
		"city":       detail.City,
		"updated_at": now,
	}).Update(); err != nil {
		return nil, gerror.Wrap(err, "更新旧站导入任务标题失败")
	}
	task, err := s.getTaskByTenant(ctx, saved.TaskId, taskRow["tenant_id"].Int64())
	if err != nil {
		return nil, err
	}
	if importMode == sysin.ImportTaskModeOverwrite {
		if err = s.clearImportTaskMedia(ctx, saved.TaskId, saved.Id); err != nil {
			return nil, err
		}
	}
	mediaImported := 0
	if importMode == sysin.ImportTaskModeOverwrite || scanItem == nil || scanItem.MediaTotal == 0 || scanItem.MediaMissingStorage > 0 {
		_ = s.appendImportRunLog(ctx, runId, "info", importStageMedia, "开始采集笔记资源", g.Map{"sourceNoteId": sourceNoteId, "profileId": saved.Id, "mediaTotal": len(detail.Media)})
		mediaImported, err = s.importLegacyCMSMedia(ctx, runId, sourceNoteId, importer, task, detail.Media)
		if err != nil {
			return nil, err
		}
		_ = s.appendImportRunLog(ctx, runId, "info", importStageMedia, "笔记资源采集完成", g.Map{"sourceNoteId": sourceNoteId, "profileId": saved.Id, "mediaImported": mediaImported, "mediaTotal": len(detail.Media)})
	}
	message := "资料已导入"
	if scanItem != nil && scanItem.Status == "existing" {
		message = "资料已更新"
	}
	return &legacyCMSImportResult{
		TaskId:        saved.TaskId,
		ProfileId:     saved.Id,
		Imported:      true,
		MediaTotal:    len(detail.Media),
		MediaImported: mediaImported,
		Message:       message,
	}, nil
}

func decodeImportTaskChannelIds(value string) []int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return []int64{}
	}
	var ids []int64
	if err := json.Unmarshal([]byte(value), &ids); err != nil {
		return []int64{}
	}
	return uniqueIds(ids)
}

func (s *sSysPublish) clearImportTaskMedia(ctx context.Context, taskId int64, profileId int64) error {
	_, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("task_id", taskId).
		Where("profile_id", profileId).
		WhereNull("deleted_at").
		Data(g.Map{
			"deleted_by": contexts.GetUserId(ctx),
			"deleted_at": gtime.Now(),
			"updated_at": gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "清理旧站资料媒体失败")
	}
	return nil
}

func (s *sSysPublish) importLegacyCMSMedia(ctx context.Context, runId int64, sourceNoteId int64, importer *legacyCMSImporter, task gdb.Record, media []*legacyCMSMedia) (int, error) {
	imported := 0
	for idx, item := range media {
		if item == nil || strings.TrimSpace(item.URL) == "" {
			continue
		}
		purpose := normalizeLegacyCMSMediaPurpose(item.Purpose)
		_ = s.appendImportRunLog(ctx, runId, "info", importStageMedia, "开始下载旧站资源", g.Map{"sourceNoteId": sourceNoteId, "url": item.URL, "purpose": purpose, "sortIndex": idx + 1})
		content, name, err := importer.downloadMedia(ctx, item)
		if err != nil {
			return imported, err
		}
		_ = s.appendImportRunLog(ctx, runId, "info", importStageMedia, "旧站资源下载完成", g.Map{"sourceNoteId": sourceNoteId, "name": name, "purpose": purpose, "size": len(content), "sortIndex": idx + 1})
		fileHeader, err := file.NewMultipartFileHeader(name, content)
		if err != nil {
			return imported, gerror.Wrap(err, "创建旧站媒体上传文件失败")
		}
		uploadFile := &ghttp.UploadFile{FileHeader: fileHeader}
		uploadType := storager.KindImg
		if item.MediaType == "video" {
			uploadType = storager.KindVideo
		}
		perceptualHash := ""
		if item.MediaType == "image" {
			perceptualHash, err = uploadImagePHash(uploadFile)
			if err != nil {
				return imported, err
			}
		}
		attachment, err := service.CommonUpload().UploadFile(ctx, uploadType, uploadFile)
		if err != nil {
			return imported, err
		}
		_ = s.appendImportRunLog(ctx, runId, "info", importStageMedia, "资源已写入当前存储", g.Map{"sourceNoteId": sourceNoteId, "attachmentId": attachment.Id, "path": attachment.Path, "purpose": purpose, "sortIndex": idx + 1})
		sortIndex := item.SortIndex
		if sortIndex <= 0 {
			sortIndex = idx + 1
		}
		if _, err = s.saveMediaAttachment(ctx, task, &sysin.MediaUploadInp{
			TaskId:    task["id"].Int64(),
			MediaType: item.MediaType,
			Purpose:   purpose,
			SortIndex: sortIndex,
		}, attachment, nil, perceptualHash); err != nil {
			return imported, err
		}
		imported++
	}
	return imported, nil
}

func legacyCMSClientRequestID(row gdb.Record, sourceNoteId int64) string {
	return fmt.Sprintf("legacy:%s:%s:%d", row["source_name"].String(), strings.TrimRight(row["base_url"].String(), "/"), sourceNoteId)
}

func legacyCMSProfileSourceKey(row gdb.Record, sourceNoteId int64) string {
	return "youban_publish:" + legacyCMSClientRequestID(row, sourceNoteId)
}

func applyImportTaskFilters(mod *gdb.Model, in *sysin.ImportTaskListInp) *gdb.Model {
	if in.TenantId > 0 {
		mod = mod.Where("t.tenant_id", in.TenantId)
	}
	if in.AccountId > 0 {
		mod = mod.Where("t.account_id", in.AccountId)
	}
	if in.Status != "" {
		mod = mod.Where("t.status", in.Status)
	}
	if strings.TrimSpace(in.Keyword) != "" {
		kw := "%" + strings.TrimSpace(in.Keyword) + "%"
		mod = mod.Where("(t.base_url LIKE ? OR t.username LIKE ? OR t.remark LIKE ?)", kw, kw, kw)
	}
	return mod
}

func fillImportTaskPercent(list []*sysin.ImportTaskModel) {
	for _, item := range list {
		if item == nil || item.ProgressTotal <= 0 {
			continue
		}
		item.Percent = float64(item.ProgressDone) * 100 / float64(item.ProgressTotal)
	}
}

func (s *sSysPublish) applyImportTaskOwnerNames(ctx context.Context, list []*sysin.ImportTaskModel) error {
	tenantIds := make([]int64, 0, len(list))
	for _, item := range list {
		if item != nil && item.TenantId > 0 {
			tenantIds = append(tenantIds, item.TenantId)
		}
	}
	names, err := s.tenantOwnerNames(ctx, tenantIds)
	if err != nil {
		return err
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		if name := strings.TrimSpace(item.TenantName); name == "" {
			item.TenantName = names[item.TenantId]
		}
		if item.TenantName == "" && item.TenantId > 0 {
			item.TenantName = fmt.Sprintf("账号归属#%d", item.TenantId)
		}
	}
	return nil
}

func (s *sSysPublish) applyImportRunOwnerNames(ctx context.Context, list []*sysin.ImportRunModel) error {
	tenantIds := make([]int64, 0, len(list))
	for _, item := range list {
		if item != nil && item.TenantId > 0 {
			tenantIds = append(tenantIds, item.TenantId)
		}
	}
	names, err := s.tenantOwnerNames(ctx, tenantIds)
	if err != nil {
		return err
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		if name := strings.TrimSpace(item.TenantName); name == "" {
			item.TenantName = names[item.TenantId]
		}
		if item.TenantName == "" && item.TenantId > 0 {
			item.TenantName = fmt.Sprintf("账号归属#%d", item.TenantId)
		}
	}
	return nil
}

func (s *sSysPublish) ensureImportTaskBelongTenant(ctx context.Context, id int64, tenantId int64) error {
	count, err := pdao.YoubanPublishImportTask.Ctx(ctx).Where("id", id).Where("tenant_id", tenantId).WhereNull("deleted_at").Count()
	if err != nil {
		return gerror.Wrap(err, "检查旧站导入任务归属失败")
	}
	if count == 0 {
		return gerror.New("旧站导入任务不存在")
	}
	return nil
}

func (s *sSysPublish) ensureImportTaskExists(ctx context.Context, id int64) error {
	count, err := pdao.YoubanPublishImportTask.Ctx(ctx).Where("id", id).WhereNull("deleted_at").Count()
	if err != nil {
		return gerror.Wrap(err, "检查旧站导入任务失败")
	}
	if count == 0 {
		return gerror.New("旧站导入任务不存在")
	}
	return nil
}

func (s *sSysPublish) ensureImportRunBelongAccount(ctx context.Context, id int64, tenantId int64, accountId int64) error {
	if err := ensureImportRunTables(ctx); err != nil {
		return err
	}
	count, err := g.DB().Model(importRunTable).Safe().Ctx(ctx).
		Where("id", id).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查导入记录归属失败")
	}
	if count == 0 {
		return gerror.New("导入记录不存在")
	}
	return nil
}

func (s *sSysPublish) isImportRunCanceled(ctx context.Context, id int64) (bool, error) {
	status, err := g.DB().Model(importRunTable).Safe().Ctx(ctx).Fields("status").Where("id", id).Value()
	if err != nil {
		return false, gerror.Wrap(err, "检查导入记录状态失败")
	}
	return status.String() == sysin.ImportTaskStatusCanceled, nil
}

func (s *sSysPublish) resetImportTask(ctx context.Context, id int64, operatorId int64, extra ...g.Map) error {
	data := g.Map{
		"status":        sysin.ImportTaskStatusPending,
		"stage":         importStageCreated,
		"error_message": "",
		"updated_by":    operatorId,
		"updated_at":    gtime.Now(),
	}
	for _, item := range extra {
		for k, v := range item {
			data[k] = v
		}
	}
	_, err := pdao.YoubanPublishImportTask.Ctx(ctx).Where("id", id).Data(data).Update()
	if err != nil {
		return gerror.Wrap(err, "启动旧站导入任务失败")
	}
	return nil
}

func (s *sSysPublish) updateImportTaskProgress(ctx context.Context, id int64, data g.Map) error {
	_, err := pdao.YoubanPublishImportTask.Ctx(ctx).Where("id", id).Data(data).Update()
	if err != nil {
		return gerror.Wrap(err, "更新旧站导入任务进度失败")
	}
	return nil
}

func (s *sSysPublish) updateImportRunProgress(ctx context.Context, id int64, data g.Map) error {
	_, err := g.DB().Model(importRunTable).Safe().Ctx(ctx).Where("id", id).Data(data).Update()
	if err != nil {
		return gerror.Wrap(err, "更新导入记录进度失败")
	}
	return nil
}

func (s *sSysPublish) appendImportRunLog(ctx context.Context, runId int64, level string, stage string, message string, data g.Map) error {
	contextJSON := ""
	if len(data) > 0 {
		raw, _ := json.Marshal(data)
		contextJSON = string(raw)
	}
	_, err := g.DB().Model(importRunLogTable).Safe().Ctx(ctx).Data(g.Map{
		"run_id":     runId,
		"level":      level,
		"stage":      stage,
		"message":    message,
		"context":    contextJSON,
		"created_at": gtime.Now(),
	}).Insert()
	return err
}

func importRuntimeContext(ctx context.Context, userId int64) context.Context {
	if userId <= 0 {
		userId = contexts.GetUserId(ctx)
	}
	if userId <= 0 {
		userId = 1
	}
	current := contexts.Get(ctx)
	if current == nil {
		return context.WithValue(ctx, consts.ContextHTTPKey, &model.Context{
			Module:    consts.AppAdmin,
			AddonName: "youban_publish",
			User: &model.Identity{
				Id:  userId,
				App: consts.AppAdmin,
			},
			Data: g.Map{},
		})
	}
	if current.Module == "" {
		current.Module = consts.AppAdmin
	}
	if current.AddonName == "" {
		current.AddonName = "youban_publish"
	}
	if current.User == nil || current.User.Id <= 0 {
		current.User = &model.Identity{Id: userId, App: consts.AppAdmin}
	}
	if current.User.App == "" {
		current.User.App = consts.AppAdmin
	}
	return ctx
}

func ensureImportRunTables(ctx context.Context) error {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		return ensureImportRunTablesPgsql(ctx)
	}
	return ensureImportRunTablesMysql(ctx)
}

func ensureImportRunTablesPgsql(ctx context.Context) error {
	if _, err := g.DB().Exec(ctx, `
CREATE TABLE IF NOT EXISTS "hg_youban_publish_import_run" (
  "id" BIGSERIAL PRIMARY KEY,
  "task_id" bigint NOT NULL DEFAULT 0,
  "tenant_id" bigint NOT NULL DEFAULT 0,
  "account_id" bigint NOT NULL DEFAULT 0,
  "source_name" varchar(64) NOT NULL DEFAULT 'lyy_cms',
  "base_url" varchar(255) NOT NULL DEFAULT '',
  "username" varchar(128) NOT NULL DEFAULT '',
  "run_type" varchar(32) NOT NULL DEFAULT 'import',
  "import_mode" varchar(32) NOT NULL DEFAULT 'incremental',
  "scan_mode" varchar(32) NOT NULL DEFAULT 'recent',
  "recent_count" integer NOT NULL DEFAULT 100,
  "status" varchar(32) NOT NULL DEFAULT 'pending',
  "stage" varchar(32) NOT NULL DEFAULT 'created',
  "progress_total" integer NOT NULL DEFAULT 0,
  "progress_done" integer NOT NULL DEFAULT 0,
  "page_total" integer NOT NULL DEFAULT 0,
  "page_done" integer NOT NULL DEFAULT 0,
  "item_total" integer NOT NULL DEFAULT 0,
  "item_done" integer NOT NULL DEFAULT 0,
  "imported" integer NOT NULL DEFAULT 0,
  "duplicate" integer NOT NULL DEFAULT 0,
  "media_total" integer NOT NULL DEFAULT 0,
  "media_done" integer NOT NULL DEFAULT 0,
  "media_imported" integer NOT NULL DEFAULT 0,
  "media_missing_storage" integer NOT NULL DEFAULT 0,
  "tg_total" integer NOT NULL DEFAULT 0,
  "tg_done" integer NOT NULL DEFAULT 0,
  "tg_matched" integer NOT NULL DEFAULT 0,
  "error_message" text,
  "params_json" text,
  "result_json" text,
  "created_by" bigint NOT NULL DEFAULT 0,
  "updated_by" bigint NOT NULL DEFAULT 0,
  "deleted_by" bigint NOT NULL DEFAULT 0,
  "started_at" timestamp DEFAULT NULL,
  "finished_at" timestamp DEFAULT NULL,
  "created_at" timestamp DEFAULT NULL,
  "updated_at" timestamp DEFAULT NULL,
  "deleted_at" timestamp DEFAULT NULL
);
`); err != nil {
		return err
	}
	if _, err := g.DB().Exec(ctx, `CREATE INDEX IF NOT EXISTS "idx_ybp_import_run_scope" ON "hg_youban_publish_import_run" ("tenant_id", "account_id", "status", "id")`); err != nil {
		return err
	}
	if _, err := g.DB().Exec(ctx, `CREATE INDEX IF NOT EXISTS "idx_ybp_import_run_task" ON "hg_youban_publish_import_run" ("task_id", "id")`); err != nil {
		return err
	}
	if _, err := g.DB().Exec(ctx, `
CREATE TABLE IF NOT EXISTS "hg_youban_publish_import_run_log" (
  "id" BIGSERIAL PRIMARY KEY,
  "run_id" bigint NOT NULL DEFAULT 0,
  "level" varchar(16) NOT NULL DEFAULT 'info',
  "stage" varchar(32) NOT NULL DEFAULT '',
  "message" text,
  "context" text,
  "created_at" timestamp DEFAULT NULL
);
`); err != nil {
		return err
	}
	_, err := g.DB().Exec(ctx, `CREATE INDEX IF NOT EXISTS "idx_ybp_import_run_log_run" ON "hg_youban_publish_import_run_log" ("run_id", "id")`)
	return err
}

func ensureImportRunTablesMysql(ctx context.Context) error {
	if _, err := g.DB().Exec(ctx, `
CREATE TABLE IF NOT EXISTS `+"`"+`hg_youban_publish_import_run`+"`"+` (
  `+"`"+`id`+"`"+` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `+"`"+`task_id`+"`"+` bigint(20) NOT NULL DEFAULT '0' COMMENT '导入任务ID',
  `+"`"+`tenant_id`+"`"+` bigint(20) NOT NULL DEFAULT '0' COMMENT '租户ID',
  `+"`"+`account_id`+"`"+` bigint(20) NOT NULL DEFAULT '0' COMMENT '上架账号ID',
  `+"`"+`source_name`+"`"+` varchar(64) NOT NULL DEFAULT 'lyy_cms' COMMENT '来源名称',
  `+"`"+`base_url`+"`"+` varchar(255) NOT NULL DEFAULT '' COMMENT '旧站域名',
  `+"`"+`username`+"`"+` varchar(128) NOT NULL DEFAULT '' COMMENT '旧站账号',
  `+"`"+`run_type`+"`"+` varchar(32) NOT NULL DEFAULT 'import' COMMENT '执行类型',
  `+"`"+`import_mode`+"`"+` varchar(32) NOT NULL DEFAULT 'incremental' COMMENT '导入方式',
  `+"`"+`scan_mode`+"`"+` varchar(32) NOT NULL DEFAULT 'recent' COMMENT '扫描范围',
  `+"`"+`recent_count`+"`"+` int(11) NOT NULL DEFAULT '100' COMMENT '最近数量',
  `+"`"+`status`+"`"+` varchar(32) NOT NULL DEFAULT 'pending' COMMENT '状态',
  `+"`"+`stage`+"`"+` varchar(32) NOT NULL DEFAULT 'created' COMMENT '阶段',
  `+"`"+`progress_total`+"`"+` int(11) NOT NULL DEFAULT '0',
  `+"`"+`progress_done`+"`"+` int(11) NOT NULL DEFAULT '0',
  `+"`"+`page_total`+"`"+` int(11) NOT NULL DEFAULT '0',
  `+"`"+`page_done`+"`"+` int(11) NOT NULL DEFAULT '0',
  `+"`"+`item_total`+"`"+` int(11) NOT NULL DEFAULT '0',
  `+"`"+`item_done`+"`"+` int(11) NOT NULL DEFAULT '0',
  `+"`"+`imported`+"`"+` int(11) NOT NULL DEFAULT '0',
  `+"`"+`duplicate`+"`"+` int(11) NOT NULL DEFAULT '0',
  `+"`"+`media_total`+"`"+` int(11) NOT NULL DEFAULT '0',
  `+"`"+`media_done`+"`"+` int(11) NOT NULL DEFAULT '0',
  `+"`"+`media_imported`+"`"+` int(11) NOT NULL DEFAULT '0',
  `+"`"+`media_missing_storage`+"`"+` int(11) NOT NULL DEFAULT '0',
  `+"`"+`tg_total`+"`"+` int(11) NOT NULL DEFAULT '0',
  `+"`"+`tg_done`+"`"+` int(11) NOT NULL DEFAULT '0',
  `+"`"+`tg_matched`+"`"+` int(11) NOT NULL DEFAULT '0',
  `+"`"+`error_message`+"`"+` text,
  `+"`"+`params_json`+"`"+` longtext,
  `+"`"+`result_json`+"`"+` longtext,
  `+"`"+`created_by`+"`"+` bigint(20) NOT NULL DEFAULT '0',
  `+"`"+`updated_by`+"`"+` bigint(20) NOT NULL DEFAULT '0',
  `+"`"+`deleted_by`+"`"+` bigint(20) NOT NULL DEFAULT '0',
  `+"`"+`started_at`+"`"+` datetime DEFAULT NULL,
  `+"`"+`finished_at`+"`"+` datetime DEFAULT NULL,
  `+"`"+`created_at`+"`"+` datetime DEFAULT NULL,
  `+"`"+`updated_at`+"`"+` datetime DEFAULT NULL,
  `+"`"+`deleted_at`+"`"+` datetime DEFAULT NULL,
  PRIMARY KEY (`+"`"+`id`+"`"+`),
  KEY `+"`"+`idx_ybp_import_run_scope`+"`"+` (`+"`"+`tenant_id`+"`"+`,`+"`"+`account_id`+"`"+`,`+"`"+`status`+"`"+`,`+"`"+`id`+"`"+`),
  KEY `+"`"+`idx_ybp_import_run_task`+"`"+` (`+"`"+`task_id`+"`"+`,`+"`"+`id`+"`"+`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架旧站导入执行记录';
`); err != nil {
		return err
	}
	_, err := g.DB().Exec(ctx, `
CREATE TABLE IF NOT EXISTS `+"`"+`hg_youban_publish_import_run_log`+"`"+` (
  `+"`"+`id`+"`"+` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `+"`"+`run_id`+"`"+` bigint(20) NOT NULL DEFAULT '0' COMMENT '执行记录ID',
  `+"`"+`level`+"`"+` varchar(16) NOT NULL DEFAULT 'info' COMMENT '日志级别',
  `+"`"+`stage`+"`"+` varchar(32) NOT NULL DEFAULT '' COMMENT '阶段',
  `+"`"+`message`+"`"+` text COMMENT '消息',
  `+"`"+`context`+"`"+` longtext COMMENT '上下文JSON',
  `+"`"+`created_at`+"`"+` datetime DEFAULT NULL COMMENT '创建时间',
  PRIMARY KEY (`+"`"+`id`+"`"+`),
  KEY `+"`"+`idx_ybp_import_run_log_run`+"`"+` (`+"`"+`run_id`+"`"+`,`+"`"+`id`+"`"+`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='悦伴上架旧站导入执行日志';
`)
	return err
}

func encodeImportPassword(password string) string {
	return base64.StdEncoding.EncodeToString([]byte(password))
}

func decodeImportPassword(cipher string) string {
	value, err := base64.StdEncoding.DecodeString(cipher)
	if err != nil {
		return cipher
	}
	return string(value)
}

type legacyCMSImporter struct {
	baseURL       string
	username      string
	password      string
	perPage       int
	proxyURL      string
	lastRequestAt time.Time
	requestMu     sync.Mutex
	client        *gclient.Client
}

type legacyCMSListPage struct {
	PageTotal int
	ItemTotal int
	Items     []int64
}

func newLegacyCMSImporter(row gdb.Record) *legacyCMSImporter {
	client := g.Client().
		SetTimeout(60 * time.Second).
		SetBrowserMode(true).
		SetAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	proxyURL := selectLegacyCMSProxy(row["proxy_enabled"].Int(), row["proxy_pool"].String())
	if proxyURL != "" {
		client.SetProxy(proxyURL)
	}
	return &legacyCMSImporter{
		baseURL:  strings.TrimRight(row["base_url"].String(), "/"),
		username: row["username"].String(),
		password: decodeImportPassword(row["password_cipher"].String()),
		perPage:  normalizeLegacyCMSPerPage(row["per_page"].Int()),
		proxyURL: proxyURL,
		client:   client,
	}
}

func selectLegacyCMSProxy(enabled int, pool string) string {
	if enabled != 1 {
		return ""
	}
	items := splitLegacyCMSProxyPool(pool)
	if len(items) == 0 {
		return ""
	}
	return items[int(time.Now().UnixNano()%int64(len(items)))]
}

func splitLegacyCMSProxyPool(pool string) []string {
	rawItems := strings.FieldsFunc(pool, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';' || r == ' '
	})
	items := make([]string, 0, len(rawItems))
	for _, item := range rawItems {
		item = normalizeLegacyCMSProxyURL(item)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func normalizeLegacyCMSProxyURL(proxyURL string) string {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return ""
	}
	lower := strings.ToLower(proxyURL)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "socks5://") {
		return proxyURL
	}
	return "http://" + proxyURL
}

func legacyCMSRequestErrorMessage(message string, proxyURL string) string {
	if proxyURL != "" {
		return message + "，已使用代理：" + proxyURL
	}
	return message + "，请检查旧站域名 DNS 是否可解析，或在导入任务中启用代理池"
}

func (i *legacyCMSImporter) beforeRequest(ctx context.Context) error {
	i.requestMu.Lock()
	defer i.requestMu.Unlock()
	if !i.lastRequestAt.IsZero() {
		gap := legacyCMSRequestMinGap + time.Duration(rand.Int63n(int64(legacyCMSRequestJitter)))
		wait := gap - time.Since(i.lastRequestAt)
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	i.lastRequestAt = time.Now()
	return nil
}

func (i *legacyCMSImporter) get(ctx context.Context, requestURL string, params ...interface{}) (*gclient.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= legacyCMSRequestRetries; attempt++ {
		if err := i.beforeRequest(ctx); err != nil {
			return nil, err
		}
		resp, err := i.client.Get(ctx, requestURL, params...)
		if err == nil && !legacyCMSShouldRetryStatus(resp.StatusCode) {
			return resp, nil
		}
		if err == nil {
			lastErr = gerror.Newf("旧站响应异常：%d", resp.StatusCode)
			resp.Close()
		} else {
			lastErr = err
		}
		if attempt < legacyCMSRequestRetries && legacyCMSShouldRetryError(lastErr) {
			if waitErr := legacyCMSRetryWait(ctx, attempt); waitErr != nil {
				return nil, waitErr
			}
			continue
		}
		return nil, lastErr
	}
	return nil, lastErr
}

func (i *legacyCMSImporter) post(ctx context.Context, requestURL string, data interface{}) (*gclient.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= legacyCMSRequestRetries; attempt++ {
		if err := i.beforeRequest(ctx); err != nil {
			return nil, err
		}
		resp, err := i.client.
			SetHeader("Origin", i.baseURL).
			SetHeader("Referer", requestURL).
			SetContentType("application/x-www-form-urlencoded").
			Post(ctx, requestURL, data)
		if err == nil && !legacyCMSShouldRetryStatus(resp.StatusCode) {
			return resp, nil
		}
		if err == nil {
			lastErr = gerror.Newf("旧站响应异常：%d", resp.StatusCode)
			resp.Close()
		} else {
			lastErr = err
		}
		if attempt < legacyCMSRequestRetries && legacyCMSShouldRetryError(lastErr) {
			if waitErr := legacyCMSRetryWait(ctx, attempt); waitErr != nil {
				return nil, waitErr
			}
			continue
		}
		return nil, lastErr
	}
	return nil, lastErr
}

func legacyCMSShouldRetryStatus(statusCode int) bool {
	return statusCode == 429 || statusCode == 502 || statusCode == 503 || statusCode == 504
}

func legacyCMSShouldRetryError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "eof") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporary") ||
		strings.Contains(msg, "旧站响应异常：429") ||
		strings.Contains(msg, "旧站响应异常：502") ||
		strings.Contains(msg, "旧站响应异常：503") ||
		strings.Contains(msg, "旧站响应异常：504")
}

func legacyCMSRetryWait(ctx context.Context, attempt int) error {
	wait := time.Duration(attempt+1)*3*time.Second + time.Duration(rand.Int63n(int64(2*time.Second)))
	timer := time.NewTimer(wait)
	select {
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (i *legacyCMSImporter) login(ctx context.Context) error {
	loginURL := i.baseURL + "/user/login"
	resp, err := i.get(ctx, loginURL)
	if err != nil {
		return gerror.Wrap(err, legacyCMSRequestErrorMessage("读取旧站登录页失败", i.proxyURL))
	}
	if resp.StatusCode != 200 {
		resp.Close()
		return gerror.Newf("旧站登录页响应异常：%d", resp.StatusCode)
	}
	loginHTML := resp.ReadAllString()
	resp.Close()

	csrfToken := parseLegacyCSRFToken(loginHTML)
	if csrfToken == "" {
		return gerror.New("旧站登录页未找到CSRF令牌")
	}

	resp, err = i.post(ctx, loginURL, g.Map{
		"_csrf_token": csrfToken,
		"username":    i.username,
		"password":    i.password,
	})
	if err != nil {
		return gerror.Wrap(err, "旧站登录请求失败")
	}
	defer resp.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 303 && resp.StatusCode != 302 {
		return gerror.Newf("旧站登录响应异常：%d", resp.StatusCode)
	}
	return nil
}

func parseLegacyCSRFToken(pageHTML string) string {
	for _, input := range regexpLegacyInput.FindAllString(pageHTML, -1) {
		if !regexpLegacyCSRFName.MatchString(input) {
			continue
		}
		match := regexpLegacyInputValue.FindStringSubmatch(input)
		if len(match) != 3 {
			continue
		}
		if match[1] != "" {
			return htmlpkg.UnescapeString(match[1])
		}
		return htmlpkg.UnescapeString(match[2])
	}
	return ""
}

func (i *legacyCMSImporter) fetchListPage(ctx context.Context, page int) (*legacyCMSListPage, error) {
	i.perPage = normalizeLegacyCMSPerPage(i.perPage)
	resp, err := i.get(ctx, i.baseURL+"/user/contents", g.Map{
		"per_page": i.perPage,
		"page":     page,
	})
	if err != nil {
		return nil, gerror.Wrap(err, "读取旧站列表失败")
	}
	defer resp.Close()
	if resp.StatusCode != 200 {
		body := strings.TrimSpace(resp.ReadAllString())
		if body != "" {
			return nil, gerror.Newf("旧站列表响应异常：%d，%s", resp.StatusCode, ellipsisString(body, 200))
		}
		return nil, gerror.Newf("旧站列表响应异常：%d，per_page=%d page=%d", resp.StatusCode, i.perPage, page)
	}
	html := resp.ReadAllString()
	return parseLegacyCMSList(html), nil
}

func (i *legacyCMSImporter) fetchDetail(ctx context.Context, sourceNoteId int64) (*legacyCMSDetail, error) {
	resp, err := i.get(ctx, fmt.Sprintf("%s/user/content/view/%d", i.baseURL, sourceNoteId))
	if err != nil {
		return nil, gerror.Wrap(err, "读取旧站详情失败")
	}
	defer resp.Close()
	if resp.StatusCode != 200 {
		body := strings.TrimSpace(resp.ReadAllString())
		if body != "" {
			return nil, gerror.Newf("旧站详情响应异常：%d，%s", resp.StatusCode, ellipsisString(body, 200))
		}
		return nil, gerror.Newf("旧站详情响应异常：%d", resp.StatusCode)
	}
	detail := parseLegacyCMSDetail(resp.ReadAllString(), sourceNoteId, i.baseURL)
	if detail.Title == "" {
		detail.Title = fmt.Sprintf("旧站资料%d", sourceNoteId)
	}
	if detail.PlainText == "" {
		return nil, gerror.Newf("旧站详情未解析到正文：%d", sourceNoteId)
	}
	return detail, nil
}

func (i *legacyCMSImporter) downloadMedia(ctx context.Context, item *legacyCMSMedia) ([]byte, string, error) {
	if item == nil || strings.TrimSpace(item.URL) == "" {
		return nil, "", gerror.New("旧站媒体地址为空")
	}
	resp, err := i.get(ctx, item.URL)
	if err != nil {
		return nil, "", gerror.Wrap(err, "下载旧站媒体失败")
	}
	defer resp.Close()
	if resp.StatusCode != 200 {
		return nil, "", gerror.Newf("旧站媒体响应异常：%d", resp.StatusCode)
	}
	content := resp.ReadAll()
	if len(content) == 0 {
		return nil, "", gerror.New("旧站媒体内容为空")
	}
	name := strings.TrimSpace(item.Name)
	if name == "" {
		if u, parseErr := url.Parse(item.URL); parseErr == nil {
			name = path.Base(u.Path)
		}
	}
	if name == "" || name == "." || name == "/" {
		name = fmt.Sprintf("legacy-%d", time.Now().UnixNano())
		if item.MediaType == "video" {
			name += ".mp4"
		} else {
			name += ".jpg"
		}
	}
	return content, name, nil
}

func normalizeLegacyCMSPerPage(perPage int) int {
	if perPage <= 0 || perPage > legacyCMSDefaultPerPage {
		return legacyCMSDefaultPerPage
	}
	return perPage
}

func ellipsisString(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "..."
}

func (i *legacyCMSImporter) collectSourceIds(ctx context.Context, scanMode string, recentCount int, limitCount int) ([]int64, error) {
	first, err := i.fetchListPage(ctx, 1)
	if err != nil {
		return nil, err
	}
	sourceIds := append([]int64{}, first.Items...)
	maxCount := limitCount
	if scanMode == sysin.ImportTaskScanModeRecent {
		maxCount = recentCount
	}
	if maxCount > 0 && len(sourceIds) >= maxCount {
		return sourceIds[:maxCount], nil
	}
	maxPages := first.PageTotal
	if scanMode == sysin.ImportTaskScanModeRecent && maxCount > 0 {
		needPages := maxCount / i.perPage
		if maxCount%i.perPage > 0 {
			needPages++
		}
		if maxPages <= 0 || needPages < maxPages {
			maxPages = needPages
		}
	}
	if maxPages <= 0 {
		maxPages = 10000
	}
	for page := 2; page <= maxPages; page++ {
		list, listErr := i.fetchListPage(ctx, page)
		if listErr != nil {
			return nil, listErr
		}
		if len(list.Items) == 0 {
			break
		}
		for _, sourceId := range list.Items {
			if sourceId > 0 && !int64In(sourceId, sourceIds) {
				sourceIds = append(sourceIds, sourceId)
			}
			if maxCount > 0 && len(sourceIds) >= maxCount {
				return sourceIds[:maxCount], nil
			}
		}
	}
	return sourceIds, nil
}

func parseLegacyCMSList(html string) *legacyCMSListPage {
	res := &legacyCMSListPage{}
	for _, match := range regexpLegacyViewID.FindAllStringSubmatch(html, -1) {
		if len(match) != 2 {
			continue
		}
		id := g.NewVar(match[1]).Int64()
		if id > 0 && !int64In(id, res.Items) {
			res.Items = append(res.Items, id)
		}
	}
	if match := regexpLegacyPage.FindStringSubmatch(html); len(match) == 3 {
		res.PageTotal = g.NewVar(match[1]).Int()
		res.ItemTotal = g.NewVar(match[2]).Int()
	}
	return res
}

func parseLegacyCMSDetail(html string, sourceNoteId int64, baseURL string) *legacyCMSDetail {
	res := &legacyCMSDetail{SourceNoteId: sourceNoteId, Media: []*legacyCMSMedia{}}
	if match := regexpLegacyH1.FindStringSubmatch(html); len(match) == 2 {
		res.Title = cleanLegacyHTMLText(match[1])
	}
	if match := regexpLegacyPreWrap.FindStringSubmatch(html); len(match) == 2 {
		res.PlainText = cleanLegacyHTMLText(match[1])
	}
	if match := regexpLegacyInfoText.FindStringSubmatch(html); len(match) == 2 {
		info := cleanLegacyHTMLText(strings.ReplaceAll(match[1], "<br>", "\n"))
		res.CreatedAt = parseLegacyDateAfter(info, "创建时间：")
		res.UpdatedAt = parseLegacyDateAfter(info, "更新时间：")
	}
	res.Province = parseLegacyTextField(res.PlainText, "省份")
	res.City = parseLegacyTextField(res.PlainText, "城市")
	res.Media = parseLegacyCMSDetailMedia(html, baseURL)
	return res
}

func parseLegacyCMSDetailMedia(html string, baseURL string) []*legacyCMSMedia {
	media := parseLegacyCMSMediaCards(html, baseURL)
	if len(media) > 0 {
		return media
	}
	return parseLegacyCMSMediaURLs(html, baseURL, "display")
}

func parseLegacyCMSMediaCards(html string, baseURL string) []*legacyCMSMedia {
	indexes := regexpLegacyFileCard.FindAllStringIndex(html, -1)
	if len(indexes) == 0 {
		return nil
	}
	list := make([]*legacyCMSMedia, 0, len(indexes))
	seen := map[string]bool{}
	sortIndexes := map[string]int{}
	for idx, item := range indexes {
		start := item[0]
		end := len(html)
		if idx+1 < len(indexes) {
			end = indexes[idx+1][0]
		}
		cardHTML := html[start:end]
		purpose := parseLegacyCMSMediaPurpose(cardHTML)
		cardMedia := parseLegacyCMSMediaURLs(cardHTML, baseURL, purpose)
		for _, mediaItem := range cardMedia {
			if mediaItem == nil || seen[mediaItem.URL] {
				continue
			}
			seen[mediaItem.URL] = true
			sortIndexes[mediaItem.Purpose]++
			mediaItem.SortIndex = sortIndexes[mediaItem.Purpose]
			list = append(list, mediaItem)
		}
	}
	return list
}

func parseLegacyCMSMediaURLs(html string, baseURL string, purpose string) []*legacyCMSMedia {
	list := []*legacyCMSMedia{}
	seen := map[string]bool{}
	for _, match := range regexpLegacyUploadURL.FindAllStringSubmatch(html, -1) {
		if len(match) != 2 {
			continue
		}
		mediaURL := normalizeLegacyURL(baseURL, htmlpkg.UnescapeString(match[1]))
		if mediaURL == "" || seen[mediaURL] {
			continue
		}
		seen[mediaURL] = true
		list = append(list, &legacyCMSMedia{
			URL:       mediaURL,
			Name:      legacyMediaName(mediaURL),
			MediaType: legacyMediaType(mediaURL),
			Purpose:   normalizeLegacyCMSMediaPurpose(purpose),
			SortIndex: len(list) + 1,
		})
	}
	return list
}

func parseLegacyCMSMediaPurpose(html string) string {
	text := cleanLegacyHTMLText(html)
	if strings.Contains(text, "第2次") || strings.Contains(text, "第二次发送") || strings.Contains(text, "验证资源") {
		return "verify"
	}
	return "display"
}

func normalizeLegacyCMSMediaPurpose(purpose string) string {
	switch strings.TrimSpace(purpose) {
	case "verify":
		return "verify"
	default:
		return "display"
	}
}

func cleanLegacyHTMLText(value string) string {
	value = regexpLegacyBR.ReplaceAllString(value, "\n")
	value = regexpLegacyTag.ReplaceAllString(value, "")
	value = htmlpkg.UnescapeString(value)
	value = strings.ReplaceAll(value, "\u00a0", " ")
	lines := strings.Split(value, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func parseLegacyTextField(text string, label string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		line = strings.ReplaceAll(line, "：", ":")
		prefix := label + ":"
		if strings.HasPrefix(line, prefix) {
			return normalizeLegacyTextValue(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func normalizeLegacyImportedTitle(title string, sourceNoteId int64) string {
	title = normalizeLegacyTextValue(title)
	if title != "" {
		return title
	}
	return fmt.Sprintf("旧站资料%d", sourceNoteId)
}

func normalizeLegacyLocationValue(value string) string {
	return normalizeLegacyTextValue(value)
}

func normalizeLegacyTextValue(value string) string {
	value = strings.TrimSpace(htmlpkg.UnescapeString(value))
	value = strings.ReplaceAll(value, "\u00a0", " ")
	value = strings.TrimSpace(value)
	normalized := strings.ToLower(strings.ReplaceAll(value, " ", ""))
	normalized = strings.ReplaceAll(normalized, "－", "-")
	normalized = strings.ReplaceAll(normalized, "—", "-")
	normalized = strings.ReplaceAll(normalized, "_", "-")
	switch normalized {
	case "", "-", "--", "nan", "na", "n/a", "null", "none", "undefined", "nan-nan", "nan/nan", "未知", "未填", "未填写", "暂无":
		return ""
	default:
		return value
	}
}

func parseLegacyDateAfter(text string, label string) *gtime.Time {
	idx := strings.Index(text, label)
	if idx < 0 {
		return nil
	}
	value := strings.TrimSpace(text[idx+len(label):])
	if len(value) > 19 {
		value = value[:19]
	}
	return gtime.NewFromStr(value)
}

func normalizeLegacyURL(baseURL string, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "data:") || strings.HasPrefix(raw, "blob:") {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	if strings.HasPrefix(raw, "//") {
		if strings.HasPrefix(baseURL, "https://") {
			return "https:" + raw
		}
		return "http:" + raw
	}
	if strings.HasPrefix(raw, "/") {
		return strings.TrimRight(baseURL, "/") + raw
	}
	return strings.TrimRight(baseURL, "/") + "/" + raw
}

func legacyMediaName(mediaURL string) string {
	u, err := url.Parse(mediaURL)
	if err != nil {
		return path.Base(mediaURL)
	}
	return path.Base(u.Path)
}

func legacyMediaType(mediaURL string) string {
	name := strings.ToLower(legacyMediaName(mediaURL))
	switch {
	case strings.HasSuffix(name, ".mp4"), strings.HasSuffix(name, ".mov"), strings.HasSuffix(name, ".m4v"), strings.HasSuffix(name, ".webm"):
		return "video"
	default:
		return "image"
	}
}

var (
	regexpLegacyInput      = regexp.MustCompile(`(?is)<input\b[^>]*>`)
	regexpLegacyCSRFName   = regexp.MustCompile(`(?is)\bname\s*=\s*["']_csrf_token["']`)
	regexpLegacyInputValue = regexp.MustCompile(`(?is)\bvalue\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	regexpLegacyViewID     = regexp.MustCompile(`/user/content/view/(\d+)`)
	regexpLegacyPage       = regexp.MustCompile(`第\s*\d+\s*/\s*(\d+)\s*页（共\s*(\d+)\s*条`)
	regexpLegacyH1         = regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>`)
	regexpLegacyPreWrap    = regexp.MustCompile(`(?is)<div\b[^>]*white-space\s*:\s*pre-wrap[^>]*>(.*?)</div>`)
	regexpLegacyInfoText   = regexp.MustCompile(`(?is)<div\b[^>]*class\s*=\s*["'][^"']*user-info-text[^"']*["'][^>]*>(.*?)</div>`)
	regexpLegacyFileCard   = regexp.MustCompile(`(?is)<div\b[^>]*class\s*=\s*["'][^"']*file-card[^"']*["'][^>]*>`)
	regexpLegacyUploadURL  = regexp.MustCompile(`(?is)(?:src|href)\s*=\s*["']([^"']*/uploads/[^"']+)["']`)
	regexpLegacyBR         = regexp.MustCompile(`(?is)<br\s*/?>`)
	regexpLegacyTag        = regexp.MustCompile(`(?is)<[^>]+>`)
)

func int64In(id int64, list []int64) bool {
	for _, item := range list {
		if item == id {
			return true
		}
	}
	return false
}
