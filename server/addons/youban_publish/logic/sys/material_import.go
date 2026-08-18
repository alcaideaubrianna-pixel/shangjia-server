package sys

import (
	"context"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/entity"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
)

const (
	materialImportPageLimit = 100
)

func ensureMaterialImportTaskChannelColumn(ctx context.Context) error {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		_, err := g.DB().Exec(ctx, `ALTER TABLE "hg_youban_publish_material_import_task" ADD COLUMN IF NOT EXISTS "channel_id_json" text`)
		return gerror.Wrap(err, "检查资料导入上架频道字段失败")
	}
	_, err := g.DB().Exec(ctx, "ALTER TABLE `hg_youban_publish_material_import_task` ADD COLUMN `channel_id_json` text COMMENT '导入资料默认上架频道ID JSON' AFTER `source_username`")
	if err != nil && !isIgnorableImportTaskServerIPColumnError(err) {
		return gerror.Wrap(err, "检查资料导入上架频道字段失败")
	}
	return nil
}

func (s *sSysPublish) materialImportTargetChannelIds(ctx context.Context, requested []int64, tenantId int64) ([]int64, error) {
	channelIds := uniqueIds(requested)
	if len(channelIds) == 0 {
		var rows []struct {
			Id int64 `json:"id"`
		}
		if err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
			Fields("id").
			Where("tenant_id", tenantId).
			Where("publish_direction", "up").
			Where("publish_visible", 1).
			Where("is_default_selected", 1).
			Where("status", 1).
			WhereNull("deleted_at").
			OrderAsc("id").
			Scan(&rows); err != nil {
			return nil, gerror.Wrap(err, "读取默认上架频道失败")
		}
		for _, row := range rows {
			if row.Id > 0 {
				channelIds = append(channelIds, row.Id)
			}
		}
	}
	if len(channelIds) == 0 {
		return nil, gerror.New("请至少选择一个上架频道")
	}
	if err := s.ensureProfileChannels(ctx, channelIds, tenantId); err != nil {
		return nil, err
	}
	return channelIds, nil
}

func (s *sSysPublish) materialImportStoredChannelIds(ctx context.Context, requested []int64, tenantId int64) ([]int64, error) {
	channelIds, err := s.materialImportTargetChannelIds(ctx, requested, tenantId)
	if err != nil {
		return nil, err
	}
	defaultChannelIds, err := s.materialImportTargetChannelIds(ctx, nil, tenantId)
	if err != nil {
		return nil, err
	}
	if sameChannelIds(channelIds, defaultChannelIds) {
		return []int64{}, nil
	}
	return channelIds, nil
}

func sameChannelIds(left, right []int64) bool {
	left = uniqueIds(left)
	right = uniqueIds(right)
	if len(left) != len(right) {
		return false
	}
	rightSet := make(map[int64]struct{}, len(right))
	for _, channelId := range right {
		rightSet[channelId] = struct{}{}
	}
	for _, channelId := range left {
		if _, ok := rightSet[channelId]; !ok {
			return false
		}
	}
	return true
}

func (s *sSysPublish) AdminMaterialImportTaskList(ctx context.Context, in *sysin.MaterialImportListInp) (list []*sysin.MaterialImportTaskModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.MaterialImportListInp{}
	}
	return s.materialImportTaskList(ctx, account.TenantId, in)
}

func (s *sSysPublish) AdminMaterialImportTaskCreate(ctx context.Context, in *sysin.MaterialImportTaskSaveInp) (id int64, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return 0, err
	}
	if in == nil {
		return 0, gerror.New("资料导入参数不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return 0, err
	}
	if err = ensureMaterialImportTaskChannelColumn(ctx); err != nil {
		return 0, err
	}
	tgAccount, err := s.adminTgAccountById(ctx, in.TgAccountId, account.TenantId)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(tgAccount.SessionKey) == "" {
		return 0, gerror.New("请选择已登录的TG账号")
	}
	cache, err := s.tgChannelCacheByChannelId(ctx, account.TenantId, in.TgAccountId, in.SourceChatId)
	if err != nil {
		return 0, err
	}
	if in.AccountId <= 0 {
		in.AccountId = account.Id
	}
	if err = s.ensureEditableAccount(ctx, in.AccountId, account.TenantId); err != nil {
		return 0, err
	}
	channelIds, err := s.materialImportStoredChannelIds(ctx, in.ChannelIds, account.TenantId)
	if err != nil {
		return 0, err
	}
	channelIdJSON, err := encodeBotIds(channelIds)
	if err != nil {
		return 0, err
	}
	now := gtime.Now()
	data := g.Map{
		"tenant_id":       account.TenantId,
		"account_id":      in.AccountId,
		"tg_account_id":   in.TgAccountId,
		"source_chat_id":  cache.ChannelId,
		"source_title":    strings.TrimSpace(cache.ChannelTitle),
		"source_username": strings.TrimSpace(cache.ChannelUsername),
		"channel_id_json": channelIdJSON,
		"pull_limit_days": in.PullLimitDays,
		"status":          sysin.MaterialImportStatusPending,
		"stage":           sysin.MaterialImportStageCreated,
		"updated_by":      account.Id,
		"updated_at":      now,
		"error_message":   "",
		"result_json":     "",
		"next_run_at":     nil,
	}
	if in.Id > 0 {
		task, taskErr := s.materialImportTaskById(ctx, in.Id, account.TenantId)
		if taskErr != nil {
			return 0, taskErr
		}
		if !materialImportTaskCanEdit(task.Status) {
			return 0, gerror.New("运行中的资料导入任务不允许编辑")
		}
		if _, err = g.DB().Model(pdao.YoubanPublishMaterialImportTask.Table()).Safe().Ctx(ctx).
			Where("id", in.Id).
			Where("tenant_id", account.TenantId).
			Data(data).
			Update(); err != nil {
			return 0, gerror.Wrap(err, "更新资料导入任务失败")
		}
		return in.Id, nil
	}
	data["created_by"] = account.Id
	data["created_at"] = now
	id, err = g.DB().Model(pdao.YoubanPublishMaterialImportTask.Table()).Safe().Ctx(ctx).Data(data).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "创建资料导入任务失败")
	}
	if err = s.materialImportMarkRunning(ctx, id, contexts.GetUserId(ctx), sysin.MaterialImportStagePulling); err != nil {
		return 0, err
	}
	if err = s.enqueueMaterialImportTask(ctx, id, 0); err != nil {
		return 0, gerror.Wrap(err, "启动资料导入任务失败")
	}
	return id, nil
}

func (s *sSysPublish) ServerMaterialImportTaskCreate(ctx context.Context, in *sysin.MaterialImportTaskServerCreateInp) (id int64, err error) {
	if err = s.requireSystemSuperAdmin(ctx); err != nil {
		return 0, err
	}
	if in == nil {
		return 0, gerror.New("资料导入参数不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return 0, err
	}
	if err = ensureMaterialImportTaskChannelColumn(ctx); err != nil {
		return 0, err
	}

	target, err := s.publishAccountById(ctx, in.AccountId)
	if err != nil {
		return 0, err
	}
	if target.TenantId != in.TenantId {
		return 0, gerror.New("归属账号不属于所选账号归属")
	}
	if target.Status != 1 {
		return 0, gerror.New("目标账号已停用")
	}
	tgAccount, err := s.adminTgAccountById(ctx, in.TgAccountId, target.TenantId)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(tgAccount.SessionKey) == "" || tgAccount.Status != sysin.PublishTgAccountStatusAuthorized {
		return 0, gerror.New("请选择已登录且授权有效的TG账号")
	}
	cache, err := s.materialImportChannelCacheByReference(ctx, target.TenantId, tgAccount, in.ChannelUrl)
	if err != nil {
		return 0, err
	}
	channelIds, err := s.materialImportStoredChannelIds(ctx, in.ChannelIds, target.TenantId)
	if err != nil {
		return 0, err
	}
	channelIdJSON, err := encodeBotIds(channelIds)
	if err != nil {
		return 0, err
	}

	now := gtime.Now()
	data := g.Map{
		"tenant_id":       target.TenantId,
		"account_id":      target.Id,
		"tg_account_id":   tgAccount.Id,
		"source_chat_id":  cache.ChannelId,
		"source_title":    strings.TrimSpace(cache.ChannelTitle),
		"source_username": strings.TrimSpace(cache.ChannelUsername),
		"channel_id_json": channelIdJSON,
		"pull_limit_days": in.PullLimitDays,
		"status":          sysin.MaterialImportStatusPending,
		"stage":           sysin.MaterialImportStageCreated,
		"updated_by":      contexts.GetUserId(ctx),
		"created_by":      contexts.GetUserId(ctx),
		"created_at":      now,
		"updated_at":      now,
		"error_message":   "",
		"result_json":     "",
		"next_run_at":     nil,
	}
	id, err = g.DB().Model(pdao.YoubanPublishMaterialImportTask.Table()).Safe().Ctx(ctx).Data(data).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "创建资料导入任务失败")
	}
	return id, nil
}

func (s *sSysPublish) publishAccountById(ctx context.Context, id int64) (*sysin.AccountModel, error) {
	if id <= 0 {
		return nil, gerror.New("请选择归属账号")
	}
	var account *sysin.AccountModel
	columns := pdao.YoubanPublishAccount.Columns()
	err := pdao.YoubanPublishAccount.Ctx(ctx).
		Where(columns.Id, id).
		WhereNull(columns.DeletedAt).
		Scan(&account)
	if err != nil {
		return nil, gerror.Wrap(err, "读取归属账号失败")
	}
	if account == nil || account.Id <= 0 {
		return nil, gerror.New("归属账号不存在")
	}
	return account, nil
}

func (s *sSysPublish) materialImportChannelCacheByReference(ctx context.Context, tenantId int64, tgAccount *sysin.TgAccountModel, raw string) (*sysin.ChannelCacheModel, error) {
	channelId, username, err := parseMaterialImportChannelReference(raw)
	if err != nil {
		return nil, err
	}
	find := func() (*sysin.ChannelCacheModel, error) {
		var cache *sysin.ChannelCacheModel
		mod := g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
			Where("tenant_id", tenantId).
			Where("tg_account_id", tgAccount.Id)
		if channelId != "" {
			mod = mod.WhereIn("channel_id", tgChannelCacheLookupIds(channelId))
		} else {
			mod = mod.Where("(LOWER(channel_username)=? OR LOWER(channel_username)=?)", username, "@"+username)
		}
		if err := mod.Scan(&cache); err != nil {
			return nil, gerror.Wrap(err, "读取频道缓存失败")
		}
		return cache, nil
	}
	cache, err := find()
	if err != nil {
		return nil, err
	}
	if cache != nil && cache.Id > 0 {
		return cache, nil
	}

	channels, err := s.fetchTgAccountChannelCaches(ctx, tgAccount)
	if err != nil {
		return nil, gerror.Wrap(err, "刷新TG频道缓存失败")
	}
	now := gtime.Now()
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		if err = s.upsertTgDialogCache(ctx, tenantId, tgAccount.Id, channel, now); err != nil {
			return nil, err
		}
	}
	cache, err = find()
	if err != nil {
		return nil, err
	}
	if cache == nil || cache.Id <= 0 {
		return nil, gerror.New("TG频道不存在，或当前TG账号未加入该频道")
	}
	return cache, nil
}

func parseMaterialImportChannelReference(raw string) (channelId string, username string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", gerror.New("请输入TG频道连接")
	}
	if strings.HasPrefix(raw, "@") {
		username = strings.TrimPrefix(raw, "@")
		return "", strings.ToLower(username), nil
	}
	if value, convErr := strconv.ParseInt(raw, 10, 64); convErr == nil && value != 0 {
		return raw, "", nil
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, parseErr := url.Parse(raw)
	if parseErr != nil || parsed.Host == "" {
		return "", "", gerror.New("TG频道连接格式不正确")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "", "", gerror.New("TG频道连接格式不正确")
	}
	if strings.EqualFold(parts[0], "c") {
		if len(parts) < 2 {
			return "", "", gerror.New("TG频道连接格式不正确")
		}
		value, convErr := strconv.ParseInt(parts[1], 10, 64)
		if convErr != nil || value <= 0 {
			return "", "", gerror.New("TG频道连接格式不正确")
		}
		return "-100" + strconv.FormatInt(value, 10), "", nil
	}
	username = strings.TrimPrefix(strings.TrimSpace(parts[0]), "@")
	if username == "" || strings.HasPrefix(username, "+") {
		return "", "", gerror.New("暂不支持私密邀请链接，请输入公开频道链接或频道ID")
	}
	return "", strings.ToLower(username), nil
}

func (s *sSysPublish) AdminMaterialImportTaskView(ctx context.Context, in *sysin.MaterialImportTaskViewInp) (res *sysin.MaterialImportTaskModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.materialImportTaskView(ctx, in.Id, account.TenantId)
}

func (s *sSysPublish) AdminMaterialImportTaskStart(ctx context.Context, in *sysin.MaterialImportTaskActionInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	task, err := s.materialImportTaskById(ctx, in.Id, account.TenantId)
	if err != nil {
		return err
	}
	if materialImportTaskIsTerminal(task.Status) {
		return gerror.New("当前任务已完成或已取消，请重新创建")
	}
	if err = s.materialImportMarkRunning(ctx, task.Id, account.Id, sysin.MaterialImportStagePulling); err != nil {
		return err
	}
	return s.enqueueMaterialImportTask(ctx, task.Id, 0)
}

func (s *sSysPublish) AdminMaterialImportTaskCancel(ctx context.Context, in *sysin.MaterialImportTaskActionInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	task, err := s.materialImportTaskById(ctx, in.Id, account.TenantId)
	if err != nil {
		return err
	}
	if materialImportTaskIsTerminal(task.Status) {
		return nil
	}
	return s.materialImportMarkCanceled(ctx, task.Id, account.Id)
}

func (s *sSysPublish) AdminMaterialImportTaskRetry(ctx context.Context, in *sysin.MaterialImportTaskActionInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	task, err := s.materialImportTaskById(ctx, in.Id, account.TenantId)
	if err != nil {
		return err
	}
	if task.Status == sysin.MaterialImportStatusSuccess {
		return gerror.New("成功任务无需重试")
	}
	stage := task.Stage
	if stage == "" || stage == sysin.MaterialImportStageFinished {
		stage = sysin.MaterialImportStagePulling
	}
	if err = s.materialImportMarkRunning(ctx, task.Id, account.Id, stage); err != nil {
		return err
	}
	return s.enqueueMaterialImportTask(ctx, task.Id, 0)
}

func (s *sSysPublish) materialImportTaskList(ctx context.Context, tenantId int64, in *sysin.MaterialImportListInp) (list []*sysin.MaterialImportTaskModel, totalCount int, err error) {
	if err = ensureMaterialImportTaskChannelColumn(ctx); err != nil {
		return nil, 0, err
	}
	mod := s.materialImportTaskBaseQuery(ctx, tenantId, in)
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取资料导入任务总数失败")
	}
	rows, err := mod.
		Fields("t.*,ta.display_name AS tg_account_nickname,ta.telegram_username AS tg_account_username,a.nickname AS account_name,tn.name AS tenant_name").
		Page(in.Page, in.PerPage).
		OrderDesc("t.id").
		All()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取资料导入任务列表失败")
	}
	list = make([]*sysin.MaterialImportTaskModel, 0, len(rows))
	for _, row := range rows {
		if item := materialImportTaskModelFromRecord(row); item != nil {
			list = append(list, item)
		}
	}
	return list, totalCount, nil
}

func (s *sSysPublish) materialImportTaskView(ctx context.Context, id int64, tenantId int64) (*sysin.MaterialImportTaskModel, error) {
	row, err := s.materialImportTaskRecord(ctx, id, tenantId)
	if err != nil {
		return nil, err
	}
	task := materialImportTaskModelFromRecord(row)
	if task == nil {
		return nil, gerror.New("资料导入任务不存在")
	}
	groups, err := s.materialImportGroupList(ctx, id)
	if err != nil {
		return nil, err
	}
	task.Groups = groups
	return task, nil
}

func (s *sSysPublish) materialImportTaskBaseQuery(ctx context.Context, tenantId int64, in *sysin.MaterialImportListInp) *gdb.Model {
	if in == nil {
		in = &sysin.MaterialImportListInp{}
	}
	mod := g.DB().Model(pdao.YoubanPublishMaterialImportTask.Table()+" t").Safe().Ctx(ctx).
		LeftJoin(publishTgAccountTable+" ta", "ta.id=t.tg_account_id AND ta.deleted_at IS NULL").
		LeftJoin(publishAccountTable+" a", "a.id=t.account_id AND a.deleted_at IS NULL").
		LeftJoin(publishTenantTable+" tn", "tn.id=t.tenant_id AND tn.deleted_at IS NULL")
	if tenantId > 0 {
		mod = mod.Where("t.tenant_id", tenantId)
	}
	if in.AccountId > 0 {
		mod = mod.Where("t.account_id", in.AccountId)
	}
	if in.TgAccountId > 0 {
		mod = mod.Where("t.tg_account_id", in.TgAccountId)
	}
	if status := strings.TrimSpace(in.Status); status != "" {
		mod = mod.Where("t.status", status)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(t.source_title LIKE ? OR t.source_username LIKE ? OR ta.display_name LIKE ? OR ta.telegram_username LIKE ? OR a.nickname LIKE ?)", like, like, like, like, like)
	}
	return mod
}

func (s *sSysPublish) materialImportTaskRecord(ctx context.Context, id int64, tenantId int64) (gdb.Record, error) {
	if err := ensureMaterialImportTaskChannelColumn(ctx); err != nil {
		return nil, err
	}
	mod := s.materialImportTaskBaseQuery(ctx, tenantId, nil).
		Fields("t.*,ta.display_name AS tg_account_nickname,ta.telegram_username AS tg_account_username,a.nickname AS account_name,tn.name AS tenant_name").
		Where("t.id", id).
		Limit(1)
	row, err := mod.One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取资料导入任务失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("资料导入任务不存在")
	}
	return row, nil
}

func (s *sSysPublish) materialImportTaskById(ctx context.Context, id int64, tenantId int64) (*sysin.MaterialImportTaskModel, error) {
	row, err := s.materialImportTaskRecord(ctx, id, tenantId)
	if err != nil {
		return nil, err
	}
	item := materialImportTaskModelFromRecord(row)
	if item == nil {
		return nil, gerror.New("资料导入任务不存在")
	}
	return item, nil
}

func (s *sSysPublish) materialImportGroupList(ctx context.Context, taskId int64) ([]*sysin.MaterialImportGroupModel, error) {
	groupCols := pdao.YoubanPublishMaterialImportGroup.Columns()
	rows, err := g.DB().Model(pdao.YoubanPublishMaterialImportGroup.Table()).Safe().Ctx(ctx).
		Where(groupCols.TaskId, taskId).
		OrderAsc(groupCols.Id).
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取资料导入分组失败")
	}
	list := make([]*sysin.MaterialImportGroupModel, 0, len(rows))
	for _, row := range rows {
		if item := materialImportGroupModelFromRecord(row); item != nil {
			list = append(list, item)
		}
	}
	return list, nil
}

func materialImportTaskModelFromRecord(row gdb.Record) *sysin.MaterialImportTaskModel {
	if row.IsEmpty() {
		return nil
	}
	var entityItem entity.YoubanPublishMaterialImportTask
	if err := gconv.Struct(row, &entityItem); err != nil {
		return nil
	}
	item := &sysin.MaterialImportTaskModel{
		YoubanPublishMaterialImportTask: entityItem,
		ChannelIdJson:                   row["channel_id_json"].String(),
		ChannelIds:                      decodeInt64JSON(row["channel_id_json"].String()),
		TenantName:                      row["tenant_name"].String(),
		AccountName:                     row["account_name"].String(),
		TgAccountNickname:               row["tg_account_nickname"].String(),
		TgAccountUsername:               row["tg_account_username"].String(),
	}
	item.Percent = materialImportTaskPercent(item)
	return item
}

func materialImportGroupModelFromRecord(row gdb.Record) *sysin.MaterialImportGroupModel {
	if row.IsEmpty() {
		return nil
	}
	var entityItem entity.YoubanPublishMaterialImportGroup
	if err := gconv.Struct(row, &entityItem); err != nil {
		return nil
	}
	item := &sysin.MaterialImportGroupModel{YoubanPublishMaterialImportGroup: entityItem}
	if item.MediaTotal > 0 {
		item.Percent = math.Min(100, float64(item.MediaDone+item.MediaFailed)*100/float64(item.MediaTotal))
	}
	return item
}

func materialImportTaskPercent(task *sysin.MaterialImportTaskModel) float64 {
	if task == nil {
		return 0
	}
	stage := strings.TrimSpace(task.Stage)
	status := strings.TrimSpace(task.Status)
	switch {
	case status == sysin.MaterialImportStatusSuccess:
		return 100
	case stage == sysin.MaterialImportStageMedia:
		if task.MediaTotal > 0 {
			percent := 60 + float64(task.MediaDone+task.MediaFailed)*40/float64(task.MediaTotal)
			if percent > 100 {
				return 100
			}
			return math.Round(percent*100) / 100
		}
		return 60
	case stage == sysin.MaterialImportStagePulling || status == sysin.MaterialImportStatusRunning || status == sysin.MaterialImportStatusWaiting:
		if task.MessageTotal > 0 {
			percent := float64(task.MessageDone) * 60 / float64(task.MessageTotal)
			if percent < 3 {
				percent = 3
			}
			if percent > 60 {
				return 60
			}
			return math.Round(percent*100) / 100
		}
		if task.MessageDone > 0 {
			return 3
		}
		return 3
	}
	total := task.MessageTotal + task.GroupTotal + task.MediaTotal
	done := task.MessageDone + task.GroupDone + task.MediaDone
	if total <= 0 {
		return 0
	}
	percent := float64(done) * 100 / float64(total)
	if percent > 100 {
		return 100
	}
	return math.Round(percent*100) / 100
}

func materialImportTaskCanEdit(status string) bool {
	switch strings.TrimSpace(status) {
	case "", sysin.MaterialImportStatusPending, sysin.MaterialImportStatusFailed, sysin.MaterialImportStatusCanceled, sysin.MaterialImportStatusWaiting:
		return true
	default:
		return false
	}
}

func materialImportTaskIsTerminal(status string) bool {
	switch strings.TrimSpace(status) {
	case sysin.MaterialImportStatusSuccess, sysin.MaterialImportStatusCanceled:
		return true
	default:
		return false
	}
}

func (s *sSysPublish) materialImportMarkRunning(ctx context.Context, taskId int64, operatorId int64, stage string) error {
	now := gtime.Now()
	_, err := g.DB().Model(pdao.YoubanPublishMaterialImportTask.Table()).Safe().Ctx(ctx).
		Where("id", taskId).
		Data(g.Map{
			"status":        sysin.MaterialImportStatusRunning,
			"stage":         stage,
			"error_message": "",
			"next_run_at":   nil,
			"updated_by":    operatorId,
			"updated_at":    now,
			"started_at":    now,
		}).
		Update()
	return err
}

func (s *sSysPublish) materialImportMarkCanceled(ctx context.Context, taskId int64, operatorId int64) error {
	now := gtime.Now()
	_, err := g.DB().Model(pdao.YoubanPublishMaterialImportTask.Table()).Safe().Ctx(ctx).
		Where("id", taskId).
		Data(g.Map{
			"status":      sysin.MaterialImportStatusCanceled,
			"stage":       sysin.MaterialImportStageCancelled,
			"next_run_at": nil,
			"updated_by":  operatorId,
			"updated_at":  now,
			"finished_at": now,
		}).
		Update()
	return err
}

func (s *sSysPublish) materialImportMarkSuccess(ctx context.Context, taskId int64, operatorId int64, result g.Map) error {
	now := gtime.Now()
	_, err := g.DB().Model(pdao.YoubanPublishMaterialImportTask.Table()).Safe().Ctx(ctx).
		Where("id", taskId).
		Data(g.Map{
			"status":        sysin.MaterialImportStatusSuccess,
			"stage":         sysin.MaterialImportStageFinished,
			"error_message": "",
			"next_run_at":   nil,
			"result_json":   gconv.String(result),
			"updated_by":    operatorId,
			"updated_at":    now,
			"finished_at":   now,
		}).
		Update()
	return err
}

func (s *sSysPublish) materialImportMarkWaiting(ctx context.Context, taskId int64, operatorId int64, delaySeconds int, message string, stage string) error {
	now := gtime.Now()
	nextRunAt := now.Add(time.Duration(delaySeconds) * time.Second)
	_, err := g.DB().Model(pdao.YoubanPublishMaterialImportTask.Table()).Safe().Ctx(ctx).
		Where("id", taskId).
		Data(g.Map{
			"status":        sysin.MaterialImportStatusWaiting,
			"stage":         stage,
			"error_message": message,
			"next_run_at":   nextRunAt,
			"updated_by":    operatorId,
			"updated_at":    now,
		}).
		Update()
	return err
}
