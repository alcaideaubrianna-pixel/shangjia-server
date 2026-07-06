package sys

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/gclient"
	"github.com/gogf/gf/v2/os/gtime"
)

const (
	importStageCreated  = "created"
	importStageLogin    = "login"
	importStageList     = "list"
	importStageDetail   = "detail"
	importStageMedia    = "media_cos"
	importStageTgMatch  = "tg_match"
	importStageFinished = "finished"
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
	if err = in.Filter(ctx); err != nil {
		return 0, err
	}
	if in.AccountId > 0 {
		if err = s.ensureAccountBelongsTenant(ctx, in.AccountId, account.TenantId); err != nil {
			return 0, err
		}
	}
	if len(in.ChannelIds) > 0 {
		if err = s.ensureChannelsBelongTenant(ctx, in.ChannelIds, account.TenantId); err != nil {
			return 0, err
		}
	}
	channelJSON, err := json.Marshal(uniqueIds(in.ChannelIds))
	if err != nil {
		return 0, err
	}
	now := gtime.Now()
	data := g.Map{
		"tenant_id":         account.TenantId,
		"account_id":        in.AccountId,
		"source_name":       in.SourceName,
		"base_url":          in.BaseUrl,
		"username":          in.Username,
		"password_cipher":   encodeImportPassword(in.Password),
		"limit_count":       in.LimitCount,
		"per_page":          in.PerPage,
		"proxy_enabled":     in.ProxyEnabled,
		"proxy_pool":        in.ProxyPool,
		"media_concurrency": in.MediaConcurrency,
		"channel_id_json":   string(channelJSON),
		"status":            sysin.ImportTaskStatusPending,
		"stage":             importStageCreated,
		"remark":            in.Remark,
		"created_by":        account.Id,
		"updated_by":        account.Id,
		"created_at":        now,
		"updated_at":        now,
	}
	if len(in.TgRange) == 2 {
		data["tg_start_at"] = in.TgRange[0]
		data["tg_end_at"] = in.TgRange[1]
	}
	id, err = pdao.YoubanPublishImportTask.Ctx(ctx).Data(data).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "创建旧站导入任务失败")
	}
	return id, s.enqueueImportTask(ctx, id, 0)
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

	// 第一版先完成任务框架、登录和列表采集进度；资料落库和COS/TG匹配继续在该执行器内扩展。
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

func (s *sSysPublish) importTaskList(ctx context.Context, in *sysin.ImportTaskListInp) (list []*sysin.ImportTaskModel, totalCount int, err error) {
	mod := pdao.YoubanPublishImportTask.Ctx(ctx).As("t").
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
	if err = mod.Fields("t.*,a.nickname AS account_name").
		Page(in.Page, in.PerPage).
		OrderDesc("t.id").
		Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取旧站导入任务失败")
	}
	fillImportTaskPercent(list)
	return list, totalCount, nil
}

func (s *sSysPublish) importTaskView(ctx context.Context, id int64, tenantId int64, accountId int64) (*sysin.ImportTaskModel, error) {
	var item *sysin.ImportTaskModel
	mod := pdao.YoubanPublishImportTask.Ctx(ctx).As("t").
		LeftJoin(publishAccountTable+" a", "a.id=t.account_id").
		Where("t.id", id).
		Where("t.tenant_id", tenantId).
		WhereNull("t.deleted_at")
	if accountId > 0 {
		mod = mod.Where("t.account_id", accountId)
	}
	if err := mod.Fields("t.*,a.nickname AS account_name").Scan(&item); err != nil {
		return nil, gerror.Wrap(err, "获取旧站导入任务详情失败")
	}
	if item == nil {
		return nil, gerror.New("旧站导入任务不存在")
	}
	fillImportTaskPercent([]*sysin.ImportTaskModel{item})
	return item, nil
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

func (s *sSysPublish) resetImportTask(ctx context.Context, id int64, operatorId int64) error {
	_, err := pdao.YoubanPublishImportTask.Ctx(ctx).Where("id", id).Data(g.Map{
		"status":        sysin.ImportTaskStatusPending,
		"stage":         importStageCreated,
		"error_message": "",
		"updated_by":    operatorId,
		"updated_at":    gtime.Now(),
	}).Update()
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
	baseURL  string
	username string
	password string
	perPage  int
	client   *gclient.Client
}

type legacyCMSListPage struct {
	PageTotal int
	ItemTotal int
	Items     []int64
}

func newLegacyCMSImporter(row gdb.Record) *legacyCMSImporter {
	return &legacyCMSImporter{
		baseURL:  strings.TrimRight(row["base_url"].String(), "/"),
		username: row["username"].String(),
		password: decodeImportPassword(row["password_cipher"].String()),
		perPage:  row["per_page"].Int(),
		client:   g.Client().SetTimeout(60 * time.Second),
	}
}

func (i *legacyCMSImporter) login(ctx context.Context) error {
	resp, err := i.client.Post(ctx, i.baseURL+"/user/login", g.Map{
		"username": i.username,
		"password": i.password,
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

func (i *legacyCMSImporter) fetchListPage(ctx context.Context, page int) (*legacyCMSListPage, error) {
	if i.perPage <= 0 {
		i.perPage = 12
	}
	resp, err := i.client.Get(ctx, i.baseURL+"/user/contents", "per_page", i.perPage, "page", page)
	if err != nil {
		return nil, gerror.Wrap(err, "读取旧站列表失败")
	}
	defer resp.Close()
	if resp.StatusCode != 200 {
		return nil, gerror.Newf("旧站列表响应异常：%d", resp.StatusCode)
	}
	html := resp.ReadAllString()
	return parseLegacyCMSList(html), nil
}

func parseLegacyCMSList(html string) *legacyCMSListPage {
	res := &legacyCMSListPage{PageTotal: 1}
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

var (
	regexpLegacyViewID = regexp.MustCompile(`/user/content/view/(\d+)`)
	regexpLegacyPage   = regexp.MustCompile(`第\s*\d+\s*/\s*(\d+)\s*页（共\s*(\d+)\s*条`)
)

func int64In(id int64, list []int64) bool {
	for _, item := range list {
		if item == id {
			return true
		}
	}
	return false
}
