package sys

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"

	"hotgo/addons/youban_feiniu_sync/model/input/sysin"
	"hotgo/addons/youban_feiniu_sync/service"
	psysin "hotgo/addons/youban_publish/model/input/sysin"
	publishservice "hotgo/addons/youban_publish/service"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
	"hotgo/internal/model/input/form"
	iservice "hotgo/internal/service"
	"hotgo/utility/encrypt"
)

const (
	configTable         = "hg_youban_feiniu_sync_config"
	channelMapTable     = "hg_youban_feiniu_sync_channel_map"
	profileMapTable     = "hg_youban_feiniu_sync_profile_map"
	runTable            = "hg_youban_feiniu_sync_run"
	dailyStatTable      = "hg_youban_feiniu_sync_daily_stat"
	channelStatTable    = "hg_youban_feiniu_sync_channel_daily_stat"
	runItemTable        = "hg_youban_feiniu_sync_run_item"
	publishTaskTable    = "hg_youban_publish_task"
	publishMediaTable   = "hg_youban_publish_media"
	publishAccountTable = "hg_youban_publish_account"

	feiniuSyncNoteTimeout = 2 * time.Minute
)

type sSysSync struct{}

func init()                 { service.RegisterSysSync(NewSysSync()) }
func NewSysSync() *sSysSync { return &sSysSync{} }

func (s *sSysSync) TenantOptions(ctx context.Context, in *sysin.OptionListInp) (list []*sysin.TenantOptionModel, err error) {
	items, _, err := publishservice.SysPublish().AdminTenantList(ctx, &psysin.TenantListInp{Keyword: strings.TrimSpace(in.Keyword), Status: 1, PageReq: form.PageReq{Page: 1, PerPage: 200}})
	if err != nil {
		return nil, gerror.Wrap(err, "读取上架租户选项失败")
	}
	list = make([]*sysin.TenantOptionModel, 0, len(items))
	for _, item := range items {
		label := strings.TrimSpace(item.Name)
		if item.Username != "" {
			label = fmt.Sprintf("%s（%s）", label, item.Username)
		}
		list = append(list, &sysin.TenantOptionModel{Id: item.Id, Name: item.Name, Username: item.Username, AdminAccountId: item.AdminAccountId, Label: label, Value: item.Id})
	}
	return
}

func (s *sSysSync) AdminAccountOptions(ctx context.Context, in *sysin.OptionListInp) (list []*sysin.AccountOptionModel, err error) {
	items, _, err := publishservice.SysPublish().ServerAccountList(ctx, &psysin.AccountListInp{TenantId: in.TenantId, AccountType: psysin.PublishAccountTypeAdmin, Keyword: strings.TrimSpace(in.Keyword), Status: 1, PageReq: form.PageReq{Page: 1, PerPage: 200}})
	if err != nil {
		return nil, gerror.Wrap(err, "读取上架管理员账号选项失败")
	}
	list = make([]*sysin.AccountOptionModel, 0, len(items))
	for _, item := range items {
		label := strings.TrimSpace(item.Nickname)
		if label == "" {
			label = item.Username
		} else if item.Username != "" {
			label = fmt.Sprintf("%s（%s）", label, item.Username)
		}
		list = append(list, &sysin.AccountOptionModel{Id: item.Id, TenantId: item.TenantId, TenantName: item.TenantName, Nickname: item.Nickname, Username: item.Username, AccountType: item.AccountType, Label: label, Value: item.Id})
	}
	return
}

func (s *sSysSync) AccountOptions(ctx context.Context, in *sysin.OptionListInp) (list []*sysin.AccountOptionModel, err error) {
	items, _, err := publishservice.SysPublish().ServerAccountList(ctx, &psysin.AccountListInp{TenantId: in.TenantId, Keyword: strings.TrimSpace(in.Keyword), Status: 1, PageReq: form.PageReq{Page: 1, PerPage: 200}})
	if err != nil {
		return nil, gerror.Wrap(err, "读取上架账号选项失败")
	}
	list = make([]*sysin.AccountOptionModel, 0, len(items))
	for _, item := range items {
		label := strings.TrimSpace(item.Nickname)
		if label == "" {
			label = item.Username
		} else if item.Username != "" {
			label = fmt.Sprintf("%s（%s）", label, item.Username)
		}
		if item.AccountType == psysin.PublishAccountTypeUploader {
			label += " [上架]"
		} else {
			label += " [管理员]"
		}
		list = append(list, &sysin.AccountOptionModel{Id: item.Id, TenantId: item.TenantId, TenantName: item.TenantName, Nickname: item.Nickname, Username: item.Username, AccountType: item.AccountType, Label: label, Value: item.Id})
	}
	return
}

func (s *sSysSync) ConfigList(ctx context.Context, in *sysin.ConfigListInp) (list []*sysin.ConfigModel, totalCount int, err error) {
	if err = ensureTables(ctx); err != nil {
		return
	}
	mod := g.DB().Model(configTable).Safe().Ctx(ctx).WhereNull("deleted_at")
	if in != nil {
		if kw := strings.TrimSpace(in.Keyword); kw != "" {
			mod = mod.WhereLike("name", "%"+kw+"%")
		}
		if in.Status > 0 {
			mod = mod.Where("status", in.Status)
		}
	}
	if totalCount, err = mod.Clone().Count(); err != nil {
		err = gerror.Wrap(err, "读取同步配置数量失败")
		return
	}
	page, perPage, offset := form.CalPage(in.Page, in.PerPage)
	in.Page, in.PerPage = page, perPage
	err = mod.Fields(configFields()).OrderDesc("id").Limit(offset, perPage).Scan(&list)
	if err != nil {
		err = gerror.Wrap(err, "读取同步配置失败")
	}
	return
}

func (s *sSysSync) ConfigView(ctx context.Context, in *sysin.ConfigViewInp) (res *sysin.ConfigModel, err error) {
	if err = ensureTables(ctx); err != nil {
		return
	}
	err = g.DB().Model(configTable).Safe().Ctx(ctx).Fields(configFields()).Where("id", in.Id).WhereNull("deleted_at").Scan(&res)
	if err != nil {
		return nil, gerror.Wrap(err, "读取同步配置失败")
	}
	if res == nil {
		return nil, gerror.New("同步配置不存在")
	}
	return
}

func (s *sSysSync) ConfigSave(ctx context.Context, in *sysin.ConfigSaveInp) (res *sysin.ConfigModel, err error) {
	if err = ensureTables(ctx); err != nil {
		return
	}
	if err = in.Filter(ctx); err != nil {
		return
	}
	now := gtime.Now()
	data := g.Map{
		"name": in.Name, "db_type": normalizeDBType(in.DbType), "db_host": in.DbHost, "db_port": in.DbPort, "db_name": in.DbName, "db_user": in.DbUser,
		"target_tenant_id": in.TargetTenantId, "target_parent_account_id": in.TargetParentAccountId, "auto_create_account": in.AutoCreateAccount,
		"sync_media": in.SyncMedia, "sync_verify_media": in.SyncVerifyMedia, "auto_sync_enabled": in.AutoSyncEnabled, "sync_interval_minutes": in.SyncIntervalMinutes, "batch_size": in.BatchSize, "status": in.Status, "updated_at": now,
	}
	if strings.TrimSpace(in.DbPassword) != "" {
		data["db_password"] = encodePassword(in.DbPassword)
	}
	if in.Id > 0 {
		if _, err = g.DB().Model(configTable).Safe().Ctx(ctx).Where("id", in.Id).Data(data).Update(); err != nil {
			return nil, gerror.Wrap(err, "更新同步配置失败")
		}
	} else {
		data["created_at"] = now
		id, e := g.DB().Model(configTable).Safe().Ctx(ctx).Data(data).InsertAndGetId()
		if e != nil {
			return nil, gerror.Wrap(e, "创建同步配置失败")
		}
		in.Id = id
	}
	cfg, err := s.configRecord(ctx, in.Id)
	if err != nil {
		return nil, err
	}
	if err = syncConfigCron(ctx, cfg, false); err != nil {
		return nil, err
	}
	return s.ConfigView(ctx, &sysin.ConfigViewInp{Id: in.Id})
}

func (s *sSysSync) ConfigDelete(ctx context.Context, in *sysin.ConfigDeleteInp) error {
	if err := ensureTables(ctx); err != nil {
		return err
	}
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要删除的配置")
	}
	_, err := g.DB().Model(configTable).Safe().Ctx(ctx).WhereIn("id", in.Ids).Data(g.Map{"deleted_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "删除同步配置失败")
	}
	for _, id := range in.Ids {
		if err = disableConfigCron(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysSync) ConfigAutoSync(ctx context.Context, in *sysin.ConfigAutoSyncInp) error {
	if err := ensureTables(ctx); err != nil {
		return err
	}
	if in == nil || in.Id <= 0 {
		return gerror.New("配置ID不能为空")
	}
	if in.AutoSyncEnabled != sysin.SyncStatusEnabled && in.AutoSyncEnabled != sysin.SyncStatusDisabled {
		return gerror.New("自动同步状态不合法")
	}
	data := g.Map{"auto_sync_enabled": in.AutoSyncEnabled, "updated_at": gtime.Now()}
	if in.AutoSyncEnabled == sysin.SyncStatusEnabled {
		data["status"] = sysin.SyncStatusEnabled
		data["last_run_at"] = nil
		data["last_error"] = ""
	}
	result, err := g.DB().Model(configTable).Safe().Ctx(ctx).Where("id", in.Id).WhereNull("deleted_at").Data(data).Update()
	if err != nil {
		return gerror.Wrap(err, "更新自动同步状态失败")
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return gerror.New("同步配置不存在")
	}
	cfg, err := s.configRecord(ctx, in.Id)
	if err != nil {
		return err
	}
	if err = syncConfigCron(ctx, cfg, in.AutoSyncEnabled == sysin.SyncStatusEnabled); err != nil {
		return err
	}
	return nil
}

func (s *sSysSync) ConfigTest(ctx context.Context, in *sysin.ConfigTestInp) (res *sysin.ConfigTestModel, err error) {
	testCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	save := in.ConfigSaveInp
	if save.Id > 0 && strings.TrimSpace(save.DbPassword) == "" {
		row, e := g.DB().Model(configTable).Safe().Ctx(testCtx).Where("id", save.Id).One()
		if e == nil && !row.IsEmpty() {
			save.DbPassword = decodePassword(row["db_password"].String())
		}
	}
	if err = save.Filter(ctx); err != nil {
		return
	}
	db, err := sourceDB(testCtx, &save)
	if err != nil {
		return nil, err
	}
	if _, err = db.Ctx(testCtx).Query(testCtx, "SELECT 1"); err != nil {
		return nil, gerror.Wrap(err, "连接 FeiNiu 数据库失败，请检查地址、端口、账号密码和网络连通性")
	}
	return &sysin.ConfigTestModel{Success: true, Message: "连接成功"}, nil
}

func (s *sSysSync) Dashboard(ctx context.Context, in *sysin.DashboardInp) (res *sysin.DashboardModel, err error) {
	if err = ensureTables(ctx); err != nil {
		return
	}
	res = &sysin.DashboardModel{}
	baseConfig := g.DB().Model(configTable).Safe().Ctx(ctx).WhereNull("deleted_at")
	if in.ConfigId > 0 {
		baseConfig = baseConfig.Where("id", in.ConfigId)
	}
	res.ConfigCount, _ = baseConfig.Clone().Count()
	pm := g.DB().Model(profileMapTable).Safe().Ctx(ctx)
	run := g.DB().Model(runTable).Safe().Ctx(ctx)
	if in.ConfigId > 0 {
		pm = pm.Where("config_id", in.ConfigId)
		run = run.Where("config_id", in.ConfigId)
	}
	res.ChannelCount = channelMapGroupCount(ctx, in.ConfigId)
	res.ProfileCount, _ = pm.Count()
	res.RunningCount, _ = run.Clone().Where("status", sysin.RunStatusRunning).Count()
	res.FailedCount, _ = run.Clone().Where("status", sysin.RunStatusFailed).Count()
	_ = run.Fields(runFields()).OrderDesc("id").Limit(1).Scan(&res.LastRun)
	return
}

func (s *sSysSync) DashboardSummary(ctx context.Context, in *sysin.DashboardInp) (res *sysin.DashboardSummaryModel, err error) {
	if err = ensureTables(ctx); err != nil {
		return
	}
	startDate, endDate := normalizeDateRange(in.StartDate, in.EndDate)
	res = &sysin.DashboardSummaryModel{UpdatedAt: gtime.Now().String()}
	baseConfig := g.DB().Model(configTable).Safe().Ctx(ctx).WhereNull("deleted_at")
	if in.ConfigId > 0 {
		baseConfig = baseConfig.Where("id", in.ConfigId)
	}
	res.ConfigCount, _ = baseConfig.Clone().Count()
	pm := g.DB().Model(profileMapTable).Safe().Ctx(ctx)
	running := g.DB().Model(runTable).Safe().Ctx(ctx).Where("status", sysin.RunStatusRunning)
	if in.ConfigId > 0 {
		pm = pm.Where("config_id", in.ConfigId)
		running = running.Where("config_id", in.ConfigId)
	}
	res.ChannelCount = channelMapGroupCount(ctx, in.ConfigId)
	res.ProfileCount, _ = pm.Count()
	res.RunningCount, _ = running.Count()
	stat := g.DB().Model(dailyStatTable).Safe().Ctx(ctx).WhereBetween("stat_date", startDate, endDate)
	if in.ConfigId > 0 {
		stat = stat.Where("config_id", in.ConfigId)
	}
	var row gdb.Record
	_ = stat.Fields("SUM(run_count) AS run_count,SUM(total_count) AS total_count,SUM(success_count) AS success_count,SUM(created_count) AS created_count,SUM(updated_count) AS updated_count,SUM(skipped_count) AS skipped_count,SUM(failed_count) AS failed_count,AVG(avg_duration_ms) AS avg_duration_ms").Scan(&row)
	res.RunCount = row["run_count"].Int()
	res.TotalCount = row["total_count"].Int()
	res.SuccessCount = row["success_count"].Int()
	res.CreatedCount = row["created_count"].Int()
	res.UpdatedCount = row["updated_count"].Int()
	res.SkippedCount = row["skipped_count"].Int()
	res.FailedCount = row["failed_count"].Int()
	res.AvgDurationMs = row["avg_duration_ms"].Int64()
	if res.TotalCount > 0 {
		res.SuccessRate = float64(res.SuccessCount) * 100 / float64(res.TotalCount)
	}
	_ = latestRunModel(g.DB().Model(runTable).Safe().Ctx(ctx), in.ConfigId).Scan(&res.LastRun)
	return
}

func (s *sSysSync) DashboardTrend(ctx context.Context, in *sysin.DashboardInp) (list []*sysin.DashboardTrendModel, err error) {
	if err = ensureTables(ctx); err != nil {
		return
	}
	startDate, endDate := normalizeDateRange(in.StartDate, in.EndDate)
	mod := g.DB().Model(dailyStatTable).Safe().Ctx(ctx).WhereBetween("stat_date", startDate, endDate)
	if in.ConfigId > 0 {
		mod = mod.Where("config_id", in.ConfigId)
	}
	err = mod.Fields("stat_date AS date,SUM(total_count) AS total_count,SUM(created_count) AS created_count,SUM(updated_count) AS updated_count,SUM(skipped_count) AS skipped_count,SUM(failed_count) AS failed_count").Group("stat_date").OrderAsc("stat_date").Scan(&list)
	if err != nil {
		err = gerror.Wrap(err, "读取采集趋势失败")
	}
	return
}

func (s *sSysSync) DashboardChannelRank(ctx context.Context, in *sysin.DashboardInp, limit int) (list []*sysin.DashboardChannelRankModel, err error) {
	if err = ensureTables(ctx); err != nil {
		return
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	startDate, endDate := normalizeDateRange(in.StartDate, in.EndDate)
	mod := g.DB().Model(channelStatTable).Safe().Ctx(ctx).WhereBetween("stat_date", startDate, endDate)
	if in.ConfigId > 0 {
		mod = mod.Where("config_id", in.ConfigId)
	}
	channelGroup := "CASE WHEN feiniu_tg_chat_id > 0 THEN feiniu_tg_chat_id ELSE feiniu_channel_id END"
	err = mod.Fields("MIN(feiniu_channel_id) AS feiniu_channel_id,MAX(feiniu_tg_chat_id) AS feiniu_tg_chat_id,MAX(feiniu_channel_title) AS feiniu_channel_title,MAX(youban_account_id) AS youban_account_id,MAX(youban_account_username) AS youban_account_username,SUM(total_count) AS total_count,SUM(created_count) AS created_count,SUM(updated_count) AS updated_count,SUM(skipped_count) AS skipped_count,SUM(failed_count) AS failed_count").Group(channelGroup).OrderDesc("total_count").Limit(limit).Scan(&list)
	if err != nil {
		err = gerror.Wrap(err, "读取频道排行失败")
	}
	return
}

func (s *sSysSync) DashboardRecentRuns(ctx context.Context, in *sysin.DashboardInp, limit int) (list []*sysin.RunModel, err error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	mod := g.DB().Model(runTable).Safe().Ctx(ctx).Fields(runFields())
	if in.ConfigId > 0 {
		mod = mod.Where("config_id", in.ConfigId)
	}
	err = mod.OrderDesc("id").Limit(limit).Scan(&list)
	if err != nil {
		err = gerror.Wrap(err, "读取最近运行失败")
	}
	return
}

func (s *sSysSync) ChannelMapList(ctx context.Context, in *sysin.ChannelMapListInp) (list []*sysin.ChannelMapModel, totalCount int, err error) {
	if err = ensureTables(ctx); err != nil {
		return
	}
	var rows []*sysin.ChannelMapModel
	if err = channelMapListModel(ctx, in).OrderDesc("updated_at").OrderDesc("id").Scan(&rows); err != nil {
		return nil, 0, gerror.Wrap(err, "读取频道映射失败")
	}
	grouped := groupChannelMapModels(rows)
	if err = fillChannelMapAccountNoteCounts(ctx, grouped); err != nil {
		return nil, 0, err
	}
	totalCount = len(grouped)

	page, perPage, offset := form.CalPage(in.Page, in.PerPage)
	in.Page, in.PerPage = page, perPage
	if offset >= totalCount {
		return []*sysin.ChannelMapModel{}, totalCount, nil
	}
	end := offset + perPage
	if end > totalCount {
		end = totalCount
	}
	return grouped[offset:end], totalCount, nil
}

func channelMapListModel(ctx context.Context, in *sysin.ChannelMapListInp) *gdb.Model {
	mod := g.DB().Model(channelMapTable).Safe().Ctx(ctx)
	if in.ConfigId > 0 {
		mod = mod.Where("config_id", in.ConfigId)
	}
	if kw := strings.TrimSpace(in.Keyword); kw != "" {
		mod = mod.Where("feiniu_channel_title LIKE ? OR youban_account_username LIKE ?", "%"+kw+"%", "%"+kw+"%")
	}
	if strings.TrimSpace(in.SyncStatus) != "" {
		mod = mod.Where("sync_status", strings.TrimSpace(in.SyncStatus))
	}
	return mod
}

func channelMapGroupKey(item *sysin.ChannelMapModel) string {
	if item == nil {
		return ""
	}
	if item.FeiniuTgChatId > 0 {
		return fmt.Sprintf("%d:chat:%d", item.ConfigId, item.FeiniuTgChatId)
	}
	if username := strings.ToLower(strings.TrimSpace(item.FeiniuUsername)); username != "" {
		return fmt.Sprintf("%d:username:%s", item.ConfigId, username)
	}
	if title := strings.ToLower(strings.TrimSpace(item.FeiniuChannelTitle)); title != "" {
		return fmt.Sprintf("%d:title:%s", item.ConfigId, title)
	}
	return fmt.Sprintf("%d:channel:%d", item.ConfigId, item.FeiniuChannelId)
}

func channelMapGroupCount(ctx context.Context, configId int64) int {
	in := &sysin.ChannelMapListInp{ConfigId: configId}
	var rows []*sysin.ChannelMapModel
	if err := channelMapListModel(ctx, in).Scan(&rows); err != nil {
		return 0
	}
	return len(groupChannelMapModels(rows))
}

func groupChannelMapModels(rows []*sysin.ChannelMapModel) []*sysin.ChannelMapModel {
	groups := make(map[string]*sysin.ChannelMapModel)
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		key := channelMapGroupKey(row)
		if key == "" {
			continue
		}
		if current, ok := groups[key]; ok {
			mergeChannelMapModel(current, row)
			continue
		}
		groups[key] = row
		keys = append(keys, key)
	}

	list := make([]*sysin.ChannelMapModel, 0, len(keys))
	for _, key := range keys {
		list = append(list, groups[key])
	}
	return list
}

func fillChannelMapAccountNoteCounts(ctx context.Context, list []*sysin.ChannelMapModel) error {
	accountIds := make([]int64, 0, len(list))
	configIds := make([]int64, 0, len(list))
	seenAccounts := make(map[int64]struct{})
	seenConfigs := make(map[int64]struct{})
	for _, item := range list {
		if item == nil || item.YoubanAccountId <= 0 || item.ConfigId <= 0 {
			continue
		}
		if _, ok := seenAccounts[item.YoubanAccountId]; !ok {
			seenAccounts[item.YoubanAccountId] = struct{}{}
			accountIds = append(accountIds, item.YoubanAccountId)
		}
		if _, ok := seenConfigs[item.ConfigId]; !ok {
			seenConfigs[item.ConfigId] = struct{}{}
			configIds = append(configIds, item.ConfigId)
		}
	}
	if len(accountIds) == 0 || len(configIds) == 0 {
		return nil
	}

	var rows []gdb.Record
	if err := g.DB().Model(profileMapTable).Safe().Ctx(ctx).
		Fields("config_id,youban_account_id,COUNT(1) AS note_count").
		WhereIn("config_id", configIds).
		WhereIn("youban_account_id", accountIds).
		WhereGT("youban_account_id", 0).
		Group("config_id,youban_account_id").
		Scan(&rows); err != nil {
		return gerror.Wrap(err, "读取账号笔记数量失败")
	}
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		key := fmt.Sprintf("%d:%d", row["config_id"].Int64(), row["youban_account_id"].Int64())
		counts[key] = row["note_count"].Int()
	}
	for _, item := range list {
		if item == nil || item.YoubanAccountId <= 0 || item.ConfigId <= 0 {
			continue
		}
		item.AccountNoteCount = counts[fmt.Sprintf("%d:%d", item.ConfigId, item.YoubanAccountId)]
	}
	return nil
}

func mergeChannelMapModel(dst *sysin.ChannelMapModel, src *sysin.ChannelMapModel) {
	if dst == nil || src == nil {
		return
	}
	if dst.FeiniuTgChatId <= 0 && src.FeiniuTgChatId > 0 {
		dst.FeiniuTgChatId = src.FeiniuTgChatId
	}
	if dst.FeiniuUsername == "" && src.FeiniuUsername != "" {
		dst.FeiniuUsername = src.FeiniuUsername
	}
	if dst.FeiniuChannelId <= 0 || (src.FeiniuChannelId > 0 && src.FeiniuChannelId < dst.FeiniuChannelId) {
		dst.FeiniuChannelId = src.FeiniuChannelId
	}
	if dst.YoubanAccountId <= 0 && src.YoubanAccountId > 0 {
		dst.YoubanAccountId = src.YoubanAccountId
		dst.YoubanAccountUsername = src.YoubanAccountUsername
	}
	if dst.LastSourceNoteId < src.LastSourceNoteId {
		dst.LastSourceNoteId = src.LastSourceNoteId
	}
	if dst.LastSourceUpdateTime == nil && src.LastSourceUpdateTime != nil {
		dst.LastSourceUpdateTime = src.LastSourceUpdateTime
	}
	if dst.SyncStatus != "failed" && src.SyncStatus == "failed" {
		dst.SyncStatus = src.SyncStatus
	}
	if dst.ErrorMessage == "" && src.ErrorMessage != "" {
		dst.ErrorMessage = src.ErrorMessage
	}
}

func (s *sSysSync) ChannelClear(ctx context.Context, in *sysin.ChannelClearInp) (res *sysin.ChannelClearModel, err error) {
	if err = ensureTables(ctx); err != nil {
		return
	}
	if in == nil || in.ConfigId <= 0 {
		return nil, gerror.New("请选择要清空的同步配置")
	}
	if _, err = s.configRecord(ctx, in.ConfigId); err != nil {
		return nil, err
	}
	res = &sysin.ChannelClearModel{}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		now := gtime.Now()
		profileCount, err := tx.Model(profileMapTable).Safe().Ctx(ctx).
			Where("config_id", in.ConfigId).
			WhereGT("youban_profile_id", 0).
			Fields("COUNT(DISTINCT youban_profile_id)").
			Value()
		if err != nil {
			return gerror.Wrap(err, "读取同步资料数量失败")
		}
		taskCount, err := tx.Model(profileMapTable).Safe().Ctx(ctx).
			Where("config_id", in.ConfigId).
			WhereGT("youban_task_id", 0).
			Fields("COUNT(DISTINCT youban_task_id)").
			Value()
		if err != nil {
			return gerror.Wrap(err, "读取同步任务数量失败")
		}
		accountCount, err := tx.Model(channelMapTable).Safe().Ctx(ctx).
			Where("config_id", in.ConfigId).
			WhereGT("youban_account_id", 0).
			Fields("COUNT(DISTINCT youban_account_id)").
			Value()
		if err != nil {
			return gerror.Wrap(err, "读取自动账号数量失败")
		}
		if _, err = tx.Exec(channelClearTaskMediaSQL(), now, in.ConfigId); err != nil {
			return gerror.Wrap(err, "清理上架媒体失败")
		}
		if _, err = tx.Exec(channelClearTaskSQL(), now, now, in.ConfigId); err != nil {
			return gerror.Wrap(err, "清理上架任务失败")
		}
		if _, err = tx.Exec(channelClearProfileMediaSQL(dao.ContentMedia.Table(), dao.ContentMedia.Columns().DeletedAt), now, in.ConfigId); err != nil {
			return gerror.Wrap(err, "清理资料媒体失败")
		}
		profileColumns := dao.ContentProfile.Columns()
		if _, err = tx.Exec(channelClearProfileSQL(dao.ContentProfile.Table(), profileColumns.DeletedAt, profileColumns.UpdatedAt), now, now, in.ConfigId); err != nil {
			return gerror.Wrap(err, "清理资料失败")
		}
		if _, err = tx.Exec(channelClearAccountSQL(), now, now, in.ConfigId); err != nil {
			return gerror.Wrap(err, "清理自动上架账号失败")
		}
		if _, err = tx.Model(runItemTable).Safe().Ctx(ctx).Where("config_id", in.ConfigId).Delete(); err != nil {
			return gerror.Wrap(err, "清理运行明细失败")
		}
		if _, err = tx.Model(runTable).Safe().Ctx(ctx).Where("config_id", in.ConfigId).Delete(); err != nil {
			return gerror.Wrap(err, "清理运行记录失败")
		}
		if _, err = tx.Model(dailyStatTable).Safe().Ctx(ctx).Where("config_id", in.ConfigId).Delete(); err != nil {
			return gerror.Wrap(err, "清理统计记录失败")
		}
		if _, err = tx.Model(channelStatTable).Safe().Ctx(ctx).Where("config_id", in.ConfigId).Delete(); err != nil {
			return gerror.Wrap(err, "清理频道统计失败")
		}
		if _, err = tx.Model(profileMapTable).Safe().Ctx(ctx).Where("config_id", in.ConfigId).Delete(); err != nil {
			return gerror.Wrap(err, "清理资料映射失败")
		}
		if _, err = tx.Model(channelMapTable).Safe().Ctx(ctx).Where("config_id", in.ConfigId).Delete(); err != nil {
			return gerror.Wrap(err, "清理频道映射失败")
		}
		if _, err = tx.Model(configTable).Safe().Ctx(ctx).Where("id", in.ConfigId).Data(g.Map{"last_run_at": nil, "last_success_at": nil, "last_error": "", "updated_at": now}).Update(); err != nil {
			return gerror.Wrap(err, "重置同步配置状态失败")
		}
		res.ProfileCount = profileCount.Int()
		res.TaskCount = taskCount.Int()
		res.AccountCount = accountCount.Int()
		return nil
	})
	return
}

func channelClearTaskMediaSQL() string {
	return fmt.Sprintf(
		`UPDATE "%s" SET "deleted_at" = ? WHERE "deleted_at" IS NULL AND "task_id" IN (SELECT DISTINCT "youban_task_id" FROM "%s" WHERE "config_id" = ? AND "youban_task_id" > 0)`,
		publishMediaTable,
		profileMapTable,
	)
}

func channelClearTaskSQL() string {
	return fmt.Sprintf(
		`UPDATE "%s" SET "deleted_at" = ?, "updated_at" = ? WHERE "deleted_at" IS NULL AND "id" IN (SELECT DISTINCT "youban_task_id" FROM "%s" WHERE "config_id" = ? AND "youban_task_id" > 0)`,
		publishTaskTable,
		profileMapTable,
	)
}

func channelClearProfileMediaSQL(targetTable string, deletedAtColumn string) string {
	return fmt.Sprintf(
		`UPDATE "%s" SET "%s" = ? WHERE "%s" IS NULL AND "profile_id" IN (SELECT DISTINCT "youban_profile_id" FROM "%s" WHERE "config_id" = ? AND "youban_profile_id" > 0)`,
		targetTable,
		deletedAtColumn,
		deletedAtColumn,
		profileMapTable,
	)
}

func channelClearProfileSQL(targetTable string, deletedAtColumn string, updatedAtColumn string) string {
	return fmt.Sprintf(
		`UPDATE "%s" SET "%s" = ?, "%s" = ? WHERE "%s" IS NULL AND "id" IN (SELECT DISTINCT "youban_profile_id" FROM "%s" WHERE "config_id" = ? AND "youban_profile_id" > 0) AND "source_type" = 'feiniu'`,
		targetTable,
		deletedAtColumn,
		updatedAtColumn,
		deletedAtColumn,
		profileMapTable,
	)
}

func channelClearAccountSQL() string {
	return fmt.Sprintf(
		`UPDATE "%s" SET "deleted_at" = ?, "updated_at" = ? WHERE "deleted_at" IS NULL AND "remark" = 'FeiNiu 自动同步频道账号' AND "id" IN (SELECT DISTINCT "youban_account_id" FROM "%s" WHERE "config_id" = ? AND "youban_account_id" > 0)`,
		publishAccountTable,
		channelMapTable,
	)
}

func (s *sSysSync) ChannelCopy(ctx context.Context, in *sysin.ChannelCopyInp) (res *sysin.ChannelCopyModel, err error) {
	if err = ensureTables(ctx); err != nil {
		return
	}
	if in == nil || in.ConfigId <= 0 || in.YoubanAccountId <= 0 || in.TargetAccountId <= 0 {
		return nil, gerror.New("请选择源账号和目标账号")
	}
	if in.TargetAccountId == in.YoubanAccountId {
		return nil, gerror.New("目标账号不能与源账号相同")
	}
	srcRow, err := s.channelMapRecord(ctx, in)
	if err != nil {
		return nil, err
	}
	targetAccount, err := s.publishAccountRecord(ctx, in.TargetAccountId)
	if err != nil {
		return nil, err
	}
	if in.TargetTenantId > 0 && targetAccount["tenant_id"].Int64() != in.TargetTenantId {
		return nil, gerror.New("目标账号不属于所选租户")
	}
	if targetAccount["status"].Int() != 1 {
		return nil, gerror.New("目标上架账号已停用")
	}
	res = &sysin.ChannelCopyModel{}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		profileRows, err := tx.Model(profileMapTable).Safe().Ctx(ctx).
			Where("config_id", in.ConfigId).
			Where("youban_account_id", in.YoubanAccountId).
			WhereGT("youban_profile_id", 0).
			OrderAsc("feiniu_note_id").
			All()
		if err != nil {
			return gerror.Wrap(err, "读取待复制资料失败")
		}
		for _, mapRow := range profileRows {
			profileId := mapRow["youban_profile_id"].Int64()
			taskId := mapRow["youban_task_id"].Int64()
			if profileId <= 0 || taskId <= 0 {
				continue
			}
			mediaCount, err := s.copyProfileBundle(ctx, tx, srcRow, targetAccount, mapRow)
			if err != nil {
				return err
			}
			res.ProfileCount++
			res.TaskCount++
			res.MediaCount += mediaCount
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	return
}

func (s *sSysSync) ChannelDisable(ctx context.Context, in *sysin.ChannelDisableInp) (res *sysin.ChannelDisableModel, err error) {
	if err = ensureTables(ctx); err != nil {
		return
	}
	if in == nil || in.ConfigId <= 0 || in.YoubanAccountId <= 0 {
		return nil, gerror.New("请选择要停用的账号")
	}
	srcRow, err := s.channelMapRecord(ctx, &sysin.ChannelCopyInp{ChannelMapId: in.ChannelMapId, ConfigId: in.ConfigId, YoubanAccountId: in.YoubanAccountId})
	if err != nil {
		return nil, err
	}
	account, err := s.publishAccountRecord(ctx, in.YoubanAccountId)
	if err != nil {
		return nil, err
	}
	res = &sysin.ChannelDisableModel{}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		now := gtime.Now()
		profileRows, err := tx.Model(profileMapTable).Safe().Ctx(ctx).
			Where("config_id", in.ConfigId).
			Where("youban_account_id", in.YoubanAccountId).
			WhereGT("youban_profile_id", 0).
			OrderAsc("feiniu_note_id").
			All()
		if err != nil {
			return gerror.Wrap(err, "读取待停用资料失败")
		}
		profileIds := make([]int64, 0, len(profileRows))
		taskIds := make([]int64, 0, len(profileRows))
		for _, mapRow := range profileRows {
			if id := mapRow["youban_profile_id"].Int64(); id > 0 {
				profileIds = append(profileIds, id)
			}
			if id := mapRow["youban_task_id"].Int64(); id > 0 {
				taskIds = append(taskIds, id)
			}
		}
		profileIds = uniqueInt64s(profileIds)
		taskIds = uniqueInt64s(taskIds)
		if len(taskIds) > 0 {
			res.MediaCount, _ = tx.Model(publishMediaTable).Safe().Ctx(ctx).WhereIn("task_id", taskIds).WhereNull("deleted_at").Count()
			if _, err = tx.Model(publishMediaTable).Safe().Ctx(ctx).WhereIn("task_id", taskIds).WhereNull("deleted_at").Data(g.Map{"deleted_at": now, "updated_at": now, "updated_by": contexts.GetUserId(ctx)}).Update(); err != nil {
				return gerror.Wrap(err, "停用上架媒体失败")
			}
			if _, err = tx.Model(publishTaskTable).Safe().Ctx(ctx).WhereIn("id", taskIds).WhereNull("deleted_at").Data(g.Map{"deleted_at": now, "updated_at": now, "updated_by": contexts.GetUserId(ctx)}).Update(); err != nil {
				return gerror.Wrap(err, "停用上架任务失败")
			}
		}
		if len(profileIds) > 0 {
			mediaColumns := dao.ContentMedia.Columns()
			if _, err = tx.Model(dao.ContentMedia.Table()).Safe().Ctx(ctx).WhereIn(mediaColumns.ProfileId, profileIds).WhereNull(mediaColumns.DeletedAt).Data(g.Map{mediaColumns.DeletedAt: now, mediaColumns.UpdatedAt: now}).Update(); err != nil {
				return gerror.Wrap(err, "停用资料媒体失败")
			}
			profileColumns := dao.ContentProfile.Columns()
			if _, err = tx.Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).WhereIn(profileColumns.Id, profileIds).WhereNull(profileColumns.DeletedAt).Data(g.Map{profileColumns.Status: 2, profileColumns.DeletedAt: now, profileColumns.UpdatedAt: now}).Update(); err != nil {
				return gerror.Wrap(err, "停用资料失败")
			}
		}
		if _, err = tx.Model(publishAccountTable).Safe().Ctx(ctx).Where("id", account["id"].Int64()).WhereNull("deleted_at").Data(g.Map{"status": 2, "updated_at": now, "deleted_at": now, "updated_by": contexts.GetUserId(ctx), "deleted_by": contexts.GetUserId(ctx)}).Update(); err != nil {
			return gerror.Wrap(err, "停用上架账号失败")
		}
		if _, err = tx.Model(channelMapTable).Safe().Ctx(ctx).Where("id", srcRow["id"].Int64()).Data(g.Map{"sync_status": "failed", "error_message": "账号已停用", "updated_at": now}).Update(); err != nil {
			return gerror.Wrap(err, "更新频道映射失败")
		}
		res.ProfileCount = len(profileIds)
		res.TaskCount = len(taskIds)
		res.AccountCount = 1
		return nil
	})
	if err != nil {
		return nil, err
	}
	iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	return
}

func (s *sSysSync) channelMapRecord(ctx context.Context, in *sysin.ChannelCopyInp) (gdb.Record, error) {
	mod := g.DB().Model(channelMapTable).Safe().Ctx(ctx).Where("config_id", in.ConfigId).Where("youban_account_id", in.YoubanAccountId)
	if in.ChannelMapId > 0 {
		mod = mod.Where("id", in.ChannelMapId)
	}
	row, err := mod.OrderDesc("id").One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取频道映射失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("频道映射不存在")
	}
	return row, nil
}

func (s *sSysSync) publishAccountRecord(ctx context.Context, accountId int64) (gdb.Record, error) {
	if accountId <= 0 {
		return nil, gerror.New("账号ID不能为空")
	}
	row, err := g.DB().Model(publishAccountTable).Safe().Ctx(ctx).Where("id", accountId).WhereNull("deleted_at").One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取上架账号失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("上架账号不存在")
	}
	return row, nil
}

func (s *sSysSync) copyProfileBundle(ctx context.Context, tx gdb.TX, srcChannel gdb.Record, targetAccount gdb.Record, mapRow gdb.Record) (mediaCount int, err error) {
	sourceProfileId := mapRow["youban_profile_id"].Int64()
	sourceTaskId := mapRow["youban_task_id"].Int64()
	if sourceProfileId <= 0 || sourceTaskId <= 0 {
		return 0, nil
	}
	profileColumns := dao.ContentProfile.Columns()
	sourceProfile, err := tx.Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).Unscoped().Where(profileColumns.Id, sourceProfileId).One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取资料失败")
	}
	if sourceProfile.IsEmpty() {
		return 0, gerror.New("源资料不存在")
	}
	sourceTask, err := tx.Model(publishTaskTable).Safe().Ctx(ctx).Where("id", sourceTaskId).WhereNull("deleted_at").One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取上架任务失败")
	}
	if sourceTask.IsEmpty() {
		return 0, gerror.New("源任务不存在")
	}
	now := gtime.Now()
	newProfileNo, err := s.newCopyProfileNo(ctx, tx)
	if err != nil {
		return 0, err
	}
	profileData := sourceProfile.Map()
	delete(profileData, profileColumns.Id)
	profileData[profileColumns.ProfileNo] = newProfileNo
	profileData[profileColumns.SourceType] = "youban_publish"
	profileData[profileColumns.SourceNoteId] = 0
	profileData[profileColumns.SourceNoteUuid] = guid.S()
	profileData[profileColumns.SourceKey] = fmt.Sprintf("youban_publish:copy:profile:%d:%d:%d", sourceProfileId, targetAccount["id"].Int64(), now.UnixNano())
	profileData[profileColumns.DuplicateOfId] = 0
	profileData[profileColumns.SourceEditedAt] = sourceProfile[profileColumns.SourceEditedAt]
	profileData[profileColumns.DeletedAt] = nil
	profileData[profileColumns.CreatedAt] = now
	profileData[profileColumns.UpdatedAt] = now
	if profileData[profileColumns.SourceCreatedAt] == nil {
		profileData[profileColumns.SourceCreatedAt] = now
	}
	if profileData[profileColumns.SourceUpdatedAt] == nil {
		profileData[profileColumns.SourceUpdatedAt] = now
	}
	profileData[profileColumns.AdminRemark] = ""
	profileData[profileColumns.Status] = sourceProfile[profileColumns.Status].Int()
	newProfileId, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Data(profileData).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "复制资料失败")
	}
	if newProfileId <= 0 {
		return 0, gerror.New("复制资料失败：未获取新资料ID")
	}
	taskData := sourceTask.Map()
	delete(taskData, "id")
	taskData["tenant_id"] = targetAccount["tenant_id"].Int64()
	taskData["merchant_id"] = targetAccount["tenant_id"].Int64()
	taskData["account_id"] = targetAccount["id"].Int64()
	taskData["profile_id"] = newProfileId
	taskData["client_request_id"] = fmt.Sprintf("feiniu-copy:%d:%d:%d", sourceTaskId, targetAccount["id"].Int64(), now.UnixNano())
	taskData["created_by"] = contexts.GetUserId(ctx)
	taskData["updated_by"] = contexts.GetUserId(ctx)
	taskData["created_at"] = now
	taskData["updated_at"] = now
	taskData["deleted_at"] = nil
	if taskData["submitted_at"] == nil {
		taskData["submitted_at"] = sourceTask["submitted_at"]
	}
	if taskData["published_at"] == nil {
		taskData["published_at"] = sourceTask["published_at"]
	}
	taskData["media_count"] = 0
	newTaskId, err := tx.Model(publishTaskTable).Ctx(ctx).Data(taskData).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "复制上架任务失败")
	}
	if newTaskId <= 0 {
		return 0, gerror.New("复制任务失败：未获取新任务ID")
	}
	copyMediaCount, copyImageCount, copyVideoCount, err := s.copyPublishMediaRows(ctx, tx, sourceTaskId, newTaskId, newProfileId, targetAccount["tenant_id"].Int64(), targetAccount["id"].Int64())
	if err != nil {
		return 0, err
	}
	if err = s.copyContentMediaRows(ctx, tx, sourceProfileId, newProfileId); err != nil {
		return 0, err
	}
	_, _ = tx.Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).Where(profileColumns.Id, newProfileId).Data(g.Map{profileColumns.ImageCount: copyImageCount, profileColumns.VideoCount: copyVideoCount, profileColumns.UpdatedAt: now}).Update()
	_, _ = tx.Model(publishTaskTable).Safe().Ctx(ctx).Where("id", newTaskId).Data(g.Map{"media_count": copyMediaCount, "updated_at": now}).Update()
	return copyMediaCount, nil
}

func (s *sSysSync) copyPublishMediaRows(ctx context.Context, tx gdb.TX, sourceTaskId, newTaskId, newProfileId, tenantId, accountId int64) (mediaCount int, imageCount int, videoCount int, err error) {
	rows, err := tx.Model(publishMediaTable).Safe().Ctx(ctx).Where("task_id", sourceTaskId).WhereNull("deleted_at").OrderAsc("sort_index").OrderAsc("id").All()
	if err != nil {
		return 0, 0, 0, gerror.Wrap(err, "读取上架媒体失败")
	}
	now := gtime.Now()
	for _, row := range rows {
		mediaData := row.Map()
		delete(mediaData, "id")
		mediaData["tenant_id"] = tenantId
		mediaData["merchant_id"] = tenantId
		mediaData["account_id"] = accountId
		mediaData["task_id"] = newTaskId
		mediaData["profile_id"] = newProfileId
		mediaData["created_by"] = contexts.GetUserId(ctx)
		mediaData["updated_by"] = contexts.GetUserId(ctx)
		mediaData["created_at"] = now
		mediaData["updated_at"] = now
		mediaData["deleted_at"] = nil
		if _, err = tx.Model(publishMediaTable).Ctx(ctx).Data(mediaData).Insert(); err != nil {
			return 0, 0, 0, gerror.Wrap(err, "复制媒体失败")
		}
		mediaCount++
		if strings.EqualFold(row["media_type"].String(), "video") {
			videoCount++
		} else {
			imageCount++
		}
	}
	return mediaCount, imageCount, videoCount, nil
}

func (s *sSysSync) copyContentMediaRows(ctx context.Context, tx gdb.TX, sourceProfileId, newProfileId int64) error {
	if sourceProfileId <= 0 || newProfileId <= 0 {
		return nil
	}
	mediaColumns := dao.ContentMedia.Columns()
	rows, err := tx.Model(dao.ContentMedia.Table()).Safe().Ctx(ctx).Unscoped().Where(mediaColumns.ProfileId, sourceProfileId).OrderAsc(mediaColumns.SortIndex).OrderAsc(mediaColumns.Id).All()
	if err != nil {
		return gerror.Wrap(err, "读取资料媒体失败")
	}
	now := gtime.Now()
	for _, row := range rows {
		mediaData := row.Map()
		delete(mediaData, mediaColumns.Id)
		mediaData[mediaColumns.ProfileId] = newProfileId
		mediaData[mediaColumns.CreatedAt] = now
		mediaData[mediaColumns.UpdatedAt] = now
		mediaData[mediaColumns.DeletedAt] = nil
		if _, err = tx.Model(dao.ContentMedia.Table()).Ctx(ctx).Data(mediaData).Insert(); err != nil {
			return gerror.Wrap(err, "复制资料媒体失败")
		}
	}
	return nil
}

func (s *sSysSync) newCopyProfileNo(ctx context.Context, tx gdb.TX) (string, error) {
	for i := 0; i < 100; i++ {
		code, err := randomProfileNo()
		if err != nil {
			return "", err
		}
		exists, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Unscoped().Where("profile_no", code).Count()
		if err != nil {
			return "", gerror.Wrap(err, "检查资料编号失败")
		}
		if exists == 0 {
			return code, nil
		}
	}
	return "", gerror.New("生成资料编号失败，请重试")
}

func randomProfileNo() (string, error) {
	letter, err := cryptorand.Int(cryptorand.Reader, big.NewInt(26))
	if err != nil {
		return "", gerror.Wrap(err, "生成资料编号失败")
	}
	number, err := cryptorand.Int(cryptorand.Reader, big.NewInt(99999))
	if err != nil {
		return "", gerror.Wrap(err, "生成资料编号失败")
	}
	return fmt.Sprintf("%c%05d", byte('A'+letter.Int64()), number.Int64()+1), nil
}

func uniqueInt64s(values []int64) []int64 {
	if len(values) == 0 {
		return []int64{}
	}
	seen := make(map[int64]struct{}, len(values))
	list := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		list = append(list, value)
	}
	return list
}

func (s *sSysSync) RunList(ctx context.Context, in *sysin.RunListInp) (list []*sysin.RunModel, totalCount int, err error) {
	if err = ensureTables(ctx); err != nil {
		return
	}
	mod := g.DB().Model(runTable).Safe().Ctx(ctx)
	if in.ConfigId > 0 {
		mod = mod.Where("config_id", in.ConfigId)
	}
	if strings.TrimSpace(in.Status) != "" {
		mod = mod.Where("status", strings.TrimSpace(in.Status))
	}
	if strings.TrimSpace(in.StartDate) != "" || strings.TrimSpace(in.EndDate) != "" {
		startDate, endDate := normalizeDateRange(in.StartDate, in.EndDate)
		mod = mod.WhereBetween("started_at", startDate+" 00:00:00", endDate+" 23:59:59")
	}
	if totalCount, err = mod.Clone().Count(); err != nil {
		return nil, 0, gerror.Wrap(err, "读取运行记录数量失败")
	}
	page, perPage, offset := form.CalPage(in.Page, in.PerPage)
	in.Page, in.PerPage = page, perPage
	err = mod.Fields(runFields()).OrderDesc("id").Limit(offset, perPage).Scan(&list)
	if err != nil {
		err = gerror.Wrap(err, "读取运行记录失败")
	}
	return
}

func (s *sSysSync) RunView(ctx context.Context, in *sysin.RunViewInp) (res *sysin.RunModel, err error) {
	if err = ensureTables(ctx); err != nil {
		return
	}
	err = g.DB().Model(runTable).Safe().Ctx(ctx).Fields(runFields()).Where("id", in.Id).Scan(&res)
	if err != nil {
		return nil, gerror.Wrap(err, "读取运行详情失败")
	}
	if res == nil {
		return nil, gerror.New("运行记录不存在")
	}
	return
}

func (s *sSysSync) RunItemList(ctx context.Context, in *sysin.RunItemListInp) (list []*sysin.RunItemModel, totalCount int, err error) {
	if err = ensureTables(ctx); err != nil {
		return
	}
	mod := g.DB().Model(runItemTable).Safe().Ctx(ctx).Where("run_id", in.RunId)
	if strings.TrimSpace(in.Status) != "" {
		mod = mod.Where("status", strings.TrimSpace(in.Status))
	}
	if strings.TrimSpace(in.Action) != "" {
		mod = mod.Where("action", strings.TrimSpace(in.Action))
	}
	if in.FeiniuChannelId > 0 {
		mod = mod.Where("feiniu_channel_id", in.FeiniuChannelId)
	}
	if kw := strings.TrimSpace(in.Keyword); kw != "" {
		mod = mod.Where("feiniu_note_code LIKE ? OR feiniu_channel_title LIKE ? OR error_message LIKE ?", "%"+kw+"%", "%"+kw+"%", "%"+kw+"%")
	}
	if totalCount, err = mod.Clone().Count(); err != nil {
		return nil, 0, gerror.Wrap(err, "读取运行明细数量失败")
	}
	page, perPage, offset := form.CalPage(in.Page, in.PerPage)
	in.Page, in.PerPage = page, perPage
	err = mod.OrderDesc("id").Limit(offset, perPage).Scan(&list)
	if err != nil {
		err = gerror.Wrap(err, "读取运行明细失败")
	}
	return
}

func (s *sSysSync) StartRun(ctx context.Context, in *sysin.RunStartInp) (res *sysin.RunStartModel, err error) {
	if err = ensureTables(ctx); err != nil {
		return
	}
	if in == nil || in.ConfigId <= 0 {
		return nil, gerror.New("配置ID不能为空")
	}
	if err = s.cleanupStaleRunningRuns(ctx, in.ConfigId, 30*time.Minute); err != nil {
		return nil, err
	}
	if running, checkErr := s.hasRunningRun(ctx, in.ConfigId); checkErr != nil {
		return nil, checkErr
	} else if running {
		return nil, gerror.New("该配置正在同步中，请稍后再试")
	}
	runId, err := s.createRun(ctx, in.ConfigId, "manual")
	if err != nil {
		return nil, err
	}
	if err = s.executeRun(ctx, runId, in.ConfigId, in.Limit); err != nil {
		return nil, err
	}
	return &sysin.RunStartModel{RunId: runId}, nil
}

func (s *sSysSync) CronRunConfig(ctx context.Context, configId int64) error {
	if err := ensureTablesWithCron(ctx, false); err != nil {
		return err
	}
	if configId <= 0 {
		return gerror.New("配置ID不能为空")
	}
	if err := s.cleanupStaleRunningRuns(ctx, configId, 30*time.Minute); err != nil {
		return err
	}
	cfg, err := s.configRecord(ctx, configId)
	if err != nil {
		return err
	}
	if cfg["status"].Int() != sysin.SyncStatusEnabled || cfg["auto_sync_enabled"].Int() != sysin.SyncStatusEnabled {
		return nil
	}
	if running, err := s.hasRunningRun(ctx, configId); err != nil {
		return err
	} else if running {
		return nil
	}
	runId, err := s.createRun(ctx, configId, "cron")
	if err != nil {
		return err
	}
	if err = s.markCronRunStarted(ctx, configId); err != nil {
		g.Log().Warning(ctx, err)
	}
	return s.executeRun(ctx, runId, configId, 0)
}

func (s *sSysSync) CronRun(ctx context.Context) error {
	if err := ensureTablesWithCron(ctx, false); err != nil {
		return err
	}
	var configs []gdb.Record
	if err := g.DB().Model(configTable).Safe().Ctx(ctx).Where("status", sysin.SyncStatusEnabled).Where("auto_sync_enabled", sysin.SyncStatusEnabled).WhereNull("deleted_at").OrderAsc("id").Scan(&configs); err != nil {
		return gerror.Wrap(err, "读取启用同步配置失败")
	}
	for _, cfg := range configs {
		if !cronConfigDue(cfg) {
			continue
		}
		if err := s.cleanupStaleRunningRuns(ctx, cfg["id"].Int64(), cronRunStaleTimeout(cfg)); err != nil {
			g.Log().Warning(ctx, err)
			continue
		}
		if running, err := s.hasRunningRun(ctx, cfg["id"].Int64()); err != nil {
			g.Log().Warning(ctx, err)
			continue
		} else if running {
			continue
		}
		runId, err := s.createRun(ctx, cfg["id"].Int64(), "cron")
		if err != nil {
			g.Log().Warning(ctx, err)
			continue
		}
		if err = s.markCronRunStarted(ctx, cfg["id"].Int64()); err != nil {
			g.Log().Warning(ctx, err)
		}
		if err = s.executeRun(ctx, runId, cfg["id"].Int64(), 0); err != nil {
			g.Log().Warning(ctx, err)
		}
	}
	return nil
}

func cronConfigDue(cfg gdb.Record) bool {
	interval := cfg["sync_interval_minutes"].Int()
	if interval <= 0 {
		interval = 10
	}
	lastRunAt := cfg["last_run_at"].GTime()
	if lastRunAt == nil {
		return true
	}
	return time.Since(lastRunAt.Time) >= time.Duration(interval)*time.Minute
}

func cronRunStaleTimeout(cfg gdb.Record) time.Duration {
	interval := cfg["sync_interval_minutes"].Int()
	if interval <= 0 {
		interval = 10
	}
	timeout := time.Duration(interval*2) * time.Minute
	if timeout < 30*time.Minute {
		timeout = 30 * time.Minute
	}
	return timeout
}

func (s *sSysSync) hasRunningRun(ctx context.Context, configId int64) (bool, error) {
	if configId <= 0 {
		return false, nil
	}
	count, err := g.DB().Model(runTable).Safe().Ctx(ctx).
		Where("config_id", configId).
		Where("status", sysin.RunStatusRunning).
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查 FeiNiu 同步运行状态失败")
	}
	return count > 0, nil
}

func (s *sSysSync) cleanupStaleRunningRuns(ctx context.Context, configId int64, timeout time.Duration) error {
	if configId <= 0 || timeout <= 0 {
		return nil
	}
	cutoff := gtime.Now().Add(-timeout)
	_, err := g.DB().Model(runTable).Safe().Ctx(ctx).
		Where("config_id", configId).
		Where("status", sysin.RunStatusRunning).
		WhereLT("started_at", cutoff).
		Data(g.Map{"status": sysin.RunStatusFailed, "finished_at": gtime.Now(), "error_message": "同步任务超时自动结束", "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "清理过期运行记录失败")
	}
	return nil
}

func (s *sSysSync) markCronRunStarted(ctx context.Context, configId int64) error {
	if configId <= 0 {
		return nil
	}
	now := gtime.Now()
	_, err := g.DB().Model(configTable).Safe().Ctx(ctx).
		Where("id", configId).
		Data(g.Map{"last_run_at": now, "updated_at": now}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新 FeiNiu 自动同步运行时间失败")
	}
	return nil
}

func (s *sSysSync) markBatchPHashDuplicates(ctx context.Context, source gdb.DB, rows []gdb.Record) map[int64]string {
	skip := make(map[int64]string)
	latest := make(map[string]gdb.Record)
	for _, row := range rows {
		signature, _, err := s.displayPHashSignature(ctx, source, row["note_id"].Int64())
		if err != nil || signature == "" {
			continue
		}
		if old, ok := latest[signature]; ok {
			if sourceTime(row).Time.After(sourceTime(old).Time) {
				skip[old["note_id"].Int64()] = fmt.Sprintf("phash_duplicate_batch newer_note:%d", row["note_id"].Int64())
				latest[signature] = row
			} else {
				skip[row["note_id"].Int64()] = fmt.Sprintf("phash_duplicate_batch newer_note:%d", old["note_id"].Int64())
			}
			continue
		}
		latest[signature] = row
	}
	return skip
}

func (s *sSysSync) displayPHashSignature(ctx context.Context, source gdb.DB, noteId int64) (string, []string, error) {
	var rows []gdb.Record
	err := source.Model("tg_content_block b").Safe().Ctx(ctx).
		LeftJoin("tg_content_asset a", "a.asset_id=b.asset_id").
		Fields("a.perceptual_hash,a.asset_type,a.mime_type,b.meta_json").
		Where("b.note_id", noteId).
		Where("b.asset_id > 0").
		OrderAsc("b.sort_index").
		Scan(&rows)
	if err != nil {
		return "", nil, gerror.Wrap(err, "读取 FeiNiu 图片 pHash 失败")
	}
	list := make([]string, 0, len(rows))
	for _, item := range rows {
		if mediaTypeOf(item["asset_type"].String(), item["mime_type"].String()) != "image" {
			continue
		}
		if strings.Contains(strings.ToLower(item["meta_json"].String()), "validation") {
			continue
		}
		hash := strings.TrimSpace(item["perceptual_hash"].String())
		if hash != "" {
			list = append(list, hash)
		}
	}
	if len(list) == 0 {
		return "", list, nil
	}
	sort.Strings(list)
	return strings.Join(list, ","), list, nil
}

func (s *sSysSync) findExistingPHashDuplicate(ctx context.Context, source gdb.DB, row gdb.Record) (int64, string, error) {
	signature, hashes, err := s.displayPHashSignature(ctx, source, row["note_id"].Int64())
	if err != nil || signature == "" {
		return 0, "", err
	}
	var candidates []gdb.Record
	err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Fields("profile_id").
		Where("purpose", "display").
		WhereNull("deleted_at").
		WhereIn("perceptual_hash", hashes).
		Group("profile_id").
		Scan(&candidates)
	if err != nil {
		return 0, "", gerror.Wrap(err, "检查 pHash 重复资料失败")
	}
	columns := dao.ContentProfile.Columns()
	for _, item := range candidates {
		profileId := item["profile_id"].Int64()
		if profileId <= 0 {
			continue
		}
		profile, e := dao.ContentProfile.Ctx(ctx).Where(columns.Id, profileId).WhereNull(columns.DeletedAt).One()
		if e != nil {
			return 0, "", gerror.Wrap(e, "读取 pHash 重复资料失败")
		}
		if !profile.IsEmpty() && profile[columns.SourceType].String() == "feiniu" && profile[columns.SourceNoteId].Int64() == row["note_id"].Int64() {
			continue
		}
		targetSig, e := s.targetDisplayPHashSignature(ctx, profileId)
		if e != nil {
			return 0, "", e
		}
		if targetSig == signature {
			return profileId, fmt.Sprintf("phash_duplicate_existing profile:%d", profileId), nil
		}
	}
	return 0, "", nil
}

func (s *sSysSync) targetDisplayPHashSignature(ctx context.Context, profileId int64) (string, error) {
	var rows []gdb.Record
	err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Fields("perceptual_hash").
		Where("profile_id", profileId).
		Where("purpose", "display").
		WhereNull("deleted_at").
		Scan(&rows)
	if err != nil {
		return "", gerror.Wrap(err, "读取目标资料 pHash 失败")
	}
	list := make([]string, 0, len(rows))
	for _, item := range rows {
		if hash := strings.TrimSpace(item["perceptual_hash"].String()); hash != "" {
			list = append(list, hash)
		}
	}
	if len(list) == 0 {
		return "", nil
	}
	sort.Strings(list)
	return strings.Join(list, ","), nil
}

func (s *sSysSync) createRun(ctx context.Context, configId int64, runType string) (int64, error) {
	now := gtime.Now()
	return g.DB().Model(runTable).Safe().Ctx(ctx).Data(g.Map{"config_id": configId, "run_type": runType, "status": sysin.RunStatusRunning, "started_at": now, "created_at": now}).InsertAndGetId()
}

func (s *sSysSync) executeRun(ctx context.Context, runId int64, configId int64, limit int) (err error) {
	started := time.Now()
	cfg, err := s.configRecord(ctx, configId)
	if err != nil {
		return s.finishRun(ctx, runId, sysin.RunStatusFailed, g.Map{"error_message": err.Error()})
	}
	save := configRecordToSave(cfg)
	db, err := sourceDB(ctx, save)
	if err != nil {
		_ = s.markConfigRunFailed(ctx, configId, err.Error())
		return s.finishRun(ctx, runId, sysin.RunStatusFailed, g.Map{"error_message": err.Error()})
	}
	s.ensureSourceIndexes(ctx, db, save.DbType, sourceIndexKey(save))
	batchSize := cfg["batch_size"].Int()
	if batchSize <= 0 {
		batchSize = 100
	}
	if limit > 0 && limit < batchSize {
		batchSize = limit
	}
	cursorNoteId, err := s.bootstrapConfigCursor(ctx, cfg)
	if err != nil {
		_ = s.markConfigRunFailed(ctx, configId, err.Error())
		return s.finishRun(ctx, runId, sysin.RunStatusFailed, g.Map{"error_message": err.Error()})
	}
	rows, err := s.fetchSourceNotes(ctx, db, configId, cursorNoteId, batchSize)
	if err != nil {
		_ = s.markConfigRunFailed(ctx, configId, err.Error())
		return s.finishRun(ctx, runId, sysin.RunStatusFailed, g.Map{"error_message": err.Error()})
	}
	batchSkip := s.markBatchPHashDuplicates(ctx, db, rows)
	stats := g.Map{"total_count": len(rows)}
	created, updated, skipped, failed := 0, 0, 0, 0
	logs := make([]string, 0, len(rows)+1)
	channelStats := map[int64]*channelRunStat{}
	cursorBlocked := false
	for _, row := range rows {
		itemStarted := time.Now()
		result, e := "", error(nil)
		if reason := batchSkip[row["note_id"].Int64()]; reason != "" {
			result = "skipped"
			e = s.upsertProfileMapSkipped(ctx, configId, row, reason)
		} else {
			noteCtx, cancel := context.WithTimeout(ctx, feiniuSyncNoteTimeout)
			result, e = s.syncOneNote(noteCtx, db, cfg, row)
			cancel()
		}
		status := "success"
		errMsg := ""
		if e != nil {
			failed++
			status = sysin.RunStatusFailed
			result = "failed"
			if errors.Is(e, context.DeadlineExceeded) {
				errMsg = fmt.Sprintf("单条资料同步超时（%s）", feiniuSyncNoteTimeout)
				e = gerror.New(errMsg)
			} else {
				errMsg = e.Error()
			}
			logs = append(logs, fmt.Sprintf("note:%d failed:%s", row["note_id"].Int64(), e.Error()))
			_ = s.upsertProfileMapError(ctx, configId, row, e.Error())
		} else {
			switch result {
			case "created":
				created++
			case "updated":
				updated++
			default:
				skipped++
			}
			if !cursorBlocked {
				noteId := row["note_id"].Int64()
				if noteId > cursorNoteId {
					cursorNoteId = noteId
					if cursorErr := s.saveConfigCursor(ctx, configId, cursorNoteId); cursorErr != nil {
						failed++
						status = sysin.RunStatusFailed
						result = "failed"
						errMsg = cursorErr.Error()
						logs = append(logs, fmt.Sprintf("note:%d failed:%s", noteId, cursorErr.Error()))
						e = cursorErr
					}
				}
			}
		}
		if e != nil {
			cursorBlocked = true
		}
		_ = s.createRunItem(ctx, runId, configId, row, result, status, errMsg, time.Since(itemStarted).Milliseconds())
		accumulateChannelStat(channelStats, row, result)
		if (created+updated+skipped+failed)%50 == 0 {
			g.Log().Infof(ctx, "FeiNiu 同步进度 runId:%d configId:%d total:%d created:%d updated:%d skipped:%d failed:%d", runId, configId, created+updated+skipped+failed, created, updated, skipped, failed)
		}
	}
	status := sysin.RunStatusSuccess
	if failed > 0 {
		status = sysin.RunStatusFailed
	}
	stats["created_count"] = created
	stats["updated_count"] = updated
	stats["skipped_count"] = skipped
	stats["failed_count"] = failed
	stats["runtime_log"] = strings.Join(logs, "\n")
	stats["finished_at"] = gtime.Now()
	stats["status"] = status
	if failed > 0 {
		stats["error_message"] = fmt.Sprintf("同步完成，失败 %d 条", failed)
	}
	_, _ = g.DB().Model(configTable).Safe().Ctx(ctx).Where("id", configId).Data(g.Map{"last_run_at": gtime.Now(), "last_success_at": gtime.Now(), "last_error": stats["error_message"], "updated_at": gtime.Now()}).Update()
	_ = s.flushRunStats(ctx, runId, configId, len(rows), created, updated, skipped, failed, time.Since(started).Milliseconds(), channelStats)
	g.Log().Info(ctx, "FeiNiu 同步完成", g.Map{"runId": runId, "configId": configId, "cost": time.Since(started).String(), "created": created, "updated": updated, "skipped": skipped, "failed": failed})
	_, err = g.DB().Model(runTable).Safe().Ctx(ctx).Where("id", runId).Data(stats).Update()
	return err
}

func (s *sSysSync) finishRun(ctx context.Context, runId int64, status string, data g.Map) error {
	if data == nil {
		data = g.Map{}
	}
	data["status"] = status
	data["finished_at"] = gtime.Now()
	_, err := g.DB().Model(runTable).Safe().Ctx(ctx).Where("id", runId).Data(data).Update()
	return err
}

func (s *sSysSync) markConfigRunFailed(ctx context.Context, configId int64, message string) error {
	if configId <= 0 {
		return nil
	}
	now := gtime.Now()
	_, err := g.DB().Model(configTable).Safe().Ctx(ctx).
		Where("id", configId).
		Data(g.Map{"last_run_at": now, "last_error": message, "updated_at": now}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新 FeiNiu 同步失败状态失败")
	}
	return nil
}

func (s *sSysSync) fetchSourceNotes(ctx context.Context, db gdb.DB, configId int64, lastNoteId int64, limit int) ([]gdb.Record, error) {
	fields := strings.Join([]string{
		"n.note_id", "n.note_uuid", "n.note_code", "n.title", "n.plain_text", "n.html_text",
		"n.category_code", "n.province", "n.city", "n.age", "n.height", "n.weight", "n.cup_size",
		"n.expected_living_cost", "n.has_verification_video", "n.image_count", "n.video_count",
		"n.text_block_count", "n.group_params", "n.tag_params", "n.storage_policy", "n.remark",
		"n.create_by", "n.create_time", "n.update_by", "n.update_time", "n.edited_at",
	}, ",")
	mod := db.Model("tg_content_note n").Safe().Ctx(ctx).
		Fields(fields).
		Where("n.status", "0")
	if lastNoteId > 0 {
		mod = mod.Where("n.note_id > ?", lastNoteId)
	}
	var rows []gdb.Record
	err := mod.OrderAsc("n.note_id").Limit(limit).Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取 FeiNiu 资料失败")
	}
	if err = s.fillSourceNoteMeta(ctx, db, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *sSysSync) fillSourceNoteMeta(ctx context.Context, db gdb.DB, rows []gdb.Record) error {
	noteIds := sourceNoteIds(rows)
	if len(noteIds) == 0 {
		return nil
	}
	var sources []gdb.Record
	if err := db.Model("tg_content_source").Safe().Ctx(ctx).
		Fields("note_id,MIN(source_key) AS source_key,MIN(channel_id) AS channel_id,MIN(tg_chat_id) AS tg_chat_id,MIN(tg_grouped_id) AS tg_grouped_id,MIN(tg_message_id) AS tg_message_id").
		WhereIn("note_id", noteIds).
		Group("note_id").
		Scan(&sources); err != nil {
		return gerror.Wrap(err, "读取 FeiNiu 资料来源失败")
	}
	sourceByNote := make(map[int64]gdb.Record, len(sources))
	channelIds := make([]int64, 0, len(sources))
	channelSeen := make(map[int64]struct{}, len(sources))
	for _, row := range sources {
		noteId := row["note_id"].Int64()
		sourceByNote[noteId] = row
		channelId := row["channel_id"].Int64()
		if channelId > 0 {
			if _, ok := channelSeen[channelId]; !ok {
				channelSeen[channelId] = struct{}{}
				channelIds = append(channelIds, channelId)
			}
		}
	}
	channels, err := s.sourceChannelsById(ctx, db, channelIds)
	if err != nil {
		return err
	}
	for _, row := range rows {
		source := sourceByNote[row["note_id"].Int64()]
		setRecordValue(row, "source_key", "")
		setRecordValue(row, "source_channel_id", 0)
		setRecordValue(row, "source_tg_chat_id", 0)
		setRecordValue(row, "source_grouped_id", 0)
		setRecordValue(row, "source_message_id", 0)
		setRecordValue(row, "channel_title", "")
		setRecordValue(row, "channel_username", "")
		setRecordValue(row, "channel_invite_link", "")
		setRecordValue(row, "channel_chat_type", "")
		if source == nil {
			continue
		}
		setRecordValue(row, "source_key", source["source_key"].String())
		setRecordValue(row, "source_channel_id", source["channel_id"].Int64())
		setRecordValue(row, "source_tg_chat_id", source["tg_chat_id"].Int64())
		setRecordValue(row, "source_grouped_id", source["tg_grouped_id"].Int64())
		setRecordValue(row, "source_message_id", source["tg_message_id"].Int64())
		channel := channels[source["channel_id"].Int64()]
		if channel == nil {
			continue
		}
		setRecordValue(row, "channel_title", channel["title"].String())
		setRecordValue(row, "channel_username", channel["username"].String())
		setRecordValue(row, "channel_invite_link", channel["invite_link"].String())
		setRecordValue(row, "channel_chat_type", channel["chat_type"].String())
	}
	return nil
}

func sourceNoteIds(rows []gdb.Record) []int64 {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := row["note_id"].Int64(); id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func recordInt64s(rows []gdb.Record, field string) []int64 {
	seen := make(map[int64]struct{}, len(rows))
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		id := row[field].Int64()
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func (s *sSysSync) sourceChannelsById(ctx context.Context, db gdb.DB, channelIds []int64) (map[int64]gdb.Record, error) {
	items := make(map[int64]gdb.Record)
	if len(channelIds) == 0 {
		return items, nil
	}
	var channels []gdb.Record
	if err := db.Model("tg_channel").Safe().Ctx(ctx).
		Fields("channel_id,title,username,invite_link,chat_type").
		WhereIn("channel_id", channelIds).
		Scan(&channels); err != nil {
		return nil, gerror.Wrap(err, "读取 FeiNiu 频道失败")
	}
	for _, row := range channels {
		items[row["channel_id"].Int64()] = row
	}
	return items, nil
}

func setRecordValue(row gdb.Record, key string, value interface{}) {
	row[key] = gvar.New(value)
}

func (s *sSysSync) syncOneNote(ctx context.Context, source gdb.DB, cfg gdb.Record, row gdb.Record) (string, error) {
	sourceUpdatedAt := sourceTime(row)
	contentHash := noteHash(row)
	existed, err := g.DB().Model(profileMapTable).Safe().Ctx(ctx).Where("config_id", cfg["id"].Int64()).Where("feiniu_note_id", row["note_id"].Int64()).One()
	if err != nil {
		return "", gerror.Wrap(err, "检查资料映射失败")
	}
	forceSync := false
	if !existed.IsEmpty() && existed["content_hash"].String() == contentHash {
		needRepair, err := s.needMediaRepair(ctx, existed, row)
		if err != nil {
			return "", err
		}
		if !needRepair {
			if err = s.syncPresentationFields(ctx, row, existed); err != nil {
				return "", err
			}
			if err = s.syncSourceMessageLinks(ctx, cfg, row, existed["youban_profile_id"].Int64(), existed["youban_task_id"].Int64(), existed["youban_account_id"].Int64()); err != nil {
				return "", err
			}
			return "skipped", s.touchChannelCursor(ctx, cfg, row, sourceUpdatedAt)
		}
		forceSync = true
	}
	if !forceSync {
		dupProfileId, dupMsg, err := s.findExistingPHashDuplicate(ctx, source, row)
		if err != nil {
			return "", err
		}
		if dupProfileId > 0 {
			return "skipped", s.upsertProfileMapSkipped(ctx, cfg["id"].Int64(), row, dupMsg)
		}
	}
	accountId, username, err := s.ensureChannelAccount(ctx, cfg, row, sourceUpdatedAt)
	if err != nil {
		return "", err
	}
	profileId, taskId, err := s.upsertProfile(ctx, cfg, row, accountId, contentHash, sourceUpdatedAt)
	if err != nil {
		return "", err
	}
	if cfg["sync_media"].Int() == 1 {
		_ = s.syncMedia(ctx, source, cfg, row, profileId, taskId, accountId)
	}
	if err = s.syncSourceMessageLinks(ctx, cfg, row, profileId, taskId, accountId); err != nil {
		return "", err
	}
	data := g.Map{"config_id": cfg["id"].Int64(), "feiniu_note_id": row["note_id"].Int64(), "feiniu_note_uuid": row["note_uuid"].String(), "feiniu_note_code": row["note_code"].String(), "feiniu_source_key": sourceKey(row), "feiniu_channel_id": row["source_channel_id"].Int64(), "feiniu_tg_chat_id": row["source_tg_chat_id"].Int64(), "youban_profile_id": profileId, "youban_task_id": taskId, "youban_account_id": accountId, "source_updated_at": sourceUpdatedAt, "content_hash": contentHash, "sync_status": "success", "error_message": "", "dedupe_key": "", "duplicate_profile_id": 0, "updated_at": gtime.Now()}
	if err = saveProfileMap(ctx, cfg["id"].Int64(), row["note_id"].Int64(), existed, data); err != nil {
		return "", gerror.Wrap(err, "保存资料映射失败")
	}
	_ = s.touchChannelCursor(ctx, cfg, row, sourceUpdatedAt)
	if username != "" {
		_ = username
	}
	if existed.IsEmpty() {
		return "created", nil
	}
	return "updated", nil
}

func (s *sSysSync) needMediaRepair(ctx context.Context, mapRow gdb.Record, row gdb.Record) (bool, error) {
	expectedMediaCount := row["image_count"].Int() + row["video_count"].Int()
	profileId := mapRow["youban_profile_id"].Int64()
	taskId := mapRow["youban_task_id"].Int64()
	needRepair, err := needMediaRepairByCount(ctx, profileId, taskId, expectedMediaCount)
	if err != nil || needRepair {
		return needRepair, err
	}
	return needMediaPerceptualHashRepair(ctx, profileId, taskId)
}

func needMediaRepairByCount(ctx context.Context, profileId int64, taskId int64, expectedMediaCount int) (bool, error) {
	if expectedMediaCount <= 0 {
		return false, nil
	}
	if profileId <= 0 || taskId <= 0 {
		return true, nil
	}
	actualMediaCount, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).
		Where("task_id", taskId).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查上架媒体状态失败")
	}
	return needMediaRepairCount(expectedMediaCount, actualMediaCount), nil
}

func needMediaRepairCount(expectedMediaCount int, actualMediaCount int) bool {
	if expectedMediaCount <= 0 {
		return false
	}
	return actualMediaCount != expectedMediaCount
}

func (s *sSysSync) syncPresentationFields(ctx context.Context, row gdb.Record, mapRow gdb.Record) error {
	title := feiNiuSyncTitle(row)
	now := gtime.Now()
	if profileId := mapRow["youban_profile_id"].Int64(); profileId > 0 {
		columns := dao.ContentProfile.Columns()
		if _, err := g.DB().Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).Where(columns.Id, profileId).Data(g.Map{columns.Title: title, columns.UpdatedAt: now}).Update(); err != nil {
			return gerror.Wrap(err, "更新同步资料标题失败")
		}
	}
	if taskId := mapRow["youban_task_id"].Int64(); taskId > 0 {
		if _, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", taskId).Data(g.Map{"title": title, "customer_remark": "", "updated_at": now}).Update(); err != nil {
			return gerror.Wrap(err, "更新同步任务标题失败")
		}
	}
	return nil
}

func (s *sSysSync) ensureChannelAccount(ctx context.Context, cfg gdb.Record, row gdb.Record, sourceUpdatedAt *gtime.Time) (int64, string, error) {
	channelId := row["source_channel_id"].Int64()
	chatId := row["source_tg_chat_id"].Int64()
	title := strings.TrimSpace(row["channel_title"].String())
	if title == "" {
		title = fmt.Sprintf("FeiNiu频道%d", channelId)
	}
	current, err := g.DB().Model(channelMapTable).Safe().Ctx(ctx).Where("config_id", cfg["id"].Int64()).Where("feiniu_channel_id", channelId).One()
	if err != nil {
		return 0, "", gerror.Wrap(err, "读取频道映射失败")
	}
	if chatId > 0 {
		mapped, err := g.DB().Model(channelMapTable).Safe().Ctx(ctx).
			Where("config_id", cfg["id"].Int64()).
			Where("feiniu_tg_chat_id", chatId).
			WhereGT("youban_account_id", 0).
			OrderAsc("id").
			One()
		if err != nil {
			return 0, "", gerror.Wrap(err, "读取同频道映射失败")
		}
		if !mapped.IsEmpty() {
			accountId := mapped["youban_account_id"].Int64()
			username := mapped["youban_account_username"].String()
			if current.IsEmpty() || current["id"].Int64() != mapped["id"].Int64() || current["youban_account_id"].Int64() != accountId || current["feiniu_tg_chat_id"].Int64() != chatId {
				if err = s.saveChannelMapping(ctx, cfg, row, current, accountId, username, title, sourceUpdatedAt); err != nil {
					return 0, "", err
				}
			}
			return accountId, username, nil
		}
	}
	if !current.IsEmpty() && current["youban_account_id"].Int64() > 0 {
		accountId := current["youban_account_id"].Int64()
		username := current["youban_account_username"].String()
		if chatId > 0 && current["feiniu_tg_chat_id"].Int64() != chatId {
			if err = s.saveChannelMapping(ctx, cfg, row, current, accountId, username, title, sourceUpdatedAt); err != nil {
				return 0, "", err
			}
		}
		return accountId, username, nil
	}
	if cfg["auto_create_account"].Int() != 1 {
		return 0, "", gerror.New("频道未映射上架账号")
	}
	username := feiNiuAccountUsername(title, channelId, chatId)
	if accountId, accountUsername, findErr := s.findExistingFeiniuAccount(ctx, cfg, title, username); findErr != nil {
		return 0, "", findErr
	} else if accountId > 0 {
		if err = s.saveChannelMapping(ctx, cfg, row, current, accountId, accountUsername, title, sourceUpdatedAt); err != nil {
			return 0, "", err
		}
		return accountId, accountUsername, nil
	}
	account, err := publishservice.SysPublish().ServerAccountSave(ctx, &psysin.AccountSaveInp{TenantId: cfg["target_tenant_id"].Int64(), ParentId: cfg["target_parent_account_id"].Int64(), AccountType: psysin.PublishAccountTypeUploader, Nickname: title, Username: username, Password: fmt.Sprintf("FN%d", time.Now().UnixNano()), DailyPublishLimit: 0, CanDirectPublish: 1, Status: 1})
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "存在") {
		var rowAcc gdb.Record
		rowAcc, err = g.DB().Model("hg_youban_publish_account").Safe().Ctx(ctx).Where("tenant_id", cfg["target_tenant_id"].Int64()).Where("username", username).WhereNull("deleted_at").One()
		if err == nil && !rowAcc.IsEmpty() {
			account = &psysin.AccountSaveModel{Id: rowAcc["id"].Int64()}
		}
	}
	if err != nil {
		return 0, "", gerror.Wrap(err, "创建上架账号失败")
	}
	if err = s.saveChannelMapping(ctx, cfg, row, current, account.Id, username, title, sourceUpdatedAt); err != nil {
		return 0, "", err
	}
	return account.Id, username, nil
}

func (s *sSysSync) findExistingFeiniuAccount(ctx context.Context, cfg gdb.Record, title string, username string) (int64, string, error) {
	base := g.DB().Model("hg_youban_publish_account").Safe().Ctx(ctx).
		Where("tenant_id", cfg["target_tenant_id"].Int64()).
		Where("account_type", psysin.PublishAccountTypeUploader).
		WhereNull("deleted_at")

	if strings.TrimSpace(username) != "" {
		row, err := base.Clone().Where("username", username).Fields("id,username").One()
		if err != nil {
			return 0, "", gerror.Wrap(err, "读取 FeiNiu 上架账号失败")
		}
		if !row.IsEmpty() {
			return row["id"].Int64(), row["username"].String(), nil
		}
	}

	if strings.TrimSpace(title) == "" {
		return 0, "", nil
	}
	row, err := base.Clone().Where("nickname", title).Fields("id,username").OrderAsc("id").One()
	if err != nil {
		return 0, "", gerror.Wrap(err, "读取 FeiNiu 上架账号失败")
	}
	if row.IsEmpty() {
		return 0, "", nil
	}
	return row["id"].Int64(), row["username"].String(), nil
}

func (s *sSysSync) saveChannelMapping(ctx context.Context, cfg gdb.Record, row gdb.Record, current gdb.Record, accountId int64, username string, title string, sourceUpdatedAt *gtime.Time) error {
	data := g.Map{"config_id": cfg["id"].Int64(), "feiniu_channel_id": row["source_channel_id"].Int64(), "feiniu_tg_chat_id": row["source_tg_chat_id"].Int64(), "feiniu_channel_title": title, "feiniu_username": row["channel_username"].String(), "youban_tenant_id": cfg["target_tenant_id"].Int64(), "youban_account_id": accountId, "youban_account_username": username, "last_source_update_time": sourceUpdatedAt, "last_source_note_id": row["note_id"].Int64(), "sync_status": "success", "error_message": "", "updated_at": gtime.Now()}
	var err error
	if current.IsEmpty() {
		data["created_at"] = gtime.Now()
		_, err = g.DB().Model(channelMapTable).Safe().Ctx(ctx).Data(data).Insert()
	} else {
		_, err = g.DB().Model(channelMapTable).Safe().Ctx(ctx).Where("id", current["id"].Int64()).Data(data).Update()
	}
	if err != nil {
		return gerror.Wrap(err, "保存频道映射失败")
	}
	return nil
}

func (s *sSysSync) touchChannelCursor(ctx context.Context, cfg gdb.Record, row gdb.Record, sourceUpdatedAt *gtime.Time) error {
	_, err := g.DB().Model(channelMapTable).Safe().Ctx(ctx).Where("config_id", cfg["id"].Int64()).Where("feiniu_channel_id", row["source_channel_id"].Int64()).Data(g.Map{"last_source_update_time": sourceUpdatedAt, "last_source_note_id": row["note_id"].Int64(), "updated_at": gtime.Now()}).Update()
	return err
}

func (s *sSysSync) saveConfigCursor(ctx context.Context, configId int64, noteId int64) error {
	if configId <= 0 || noteId <= 0 {
		return nil
	}
	_, err := g.DB().Model(configTable).Safe().Ctx(ctx).Where("id", configId).Data(g.Map{"last_source_note_id": noteId, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "保存同步游标失败")
	}
	return nil
}

func (s *sSysSync) bootstrapConfigCursor(ctx context.Context, cfg gdb.Record) (int64, error) {
	current := cfg["last_source_note_id"].Int64()
	if current > 0 {
		return current, nil
	}
	configId := cfg["id"].Int64()
	if configId <= 0 {
		return 0, nil
	}
	row, err := g.DB().Model(profileMapTable).Safe().Ctx(ctx).
		Fields("MAX(feiniu_note_id) AS last_source_note_id").
		Where("config_id", configId).
		One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取同步游标失败")
	}
	if row.IsEmpty() {
		return 0, nil
	}
	cursor := row["last_source_note_id"].Int64()
	if cursor <= 0 {
		return 0, nil
	}
	if err = s.saveConfigCursor(ctx, configId, cursor); err != nil {
		return 0, err
	}
	cfg["last_source_note_id"] = gvar.New(cursor)
	return cursor, nil
}

func (s *sSysSync) upsertProfile(ctx context.Context, cfg gdb.Record, row gdb.Record, accountId int64, contentHash string, publishedAt *gtime.Time) (int64, int64, error) {
	columns := dao.ContentProfile.Columns()
	sourceKey := sourceKey(row)
	now := gtime.Now()
	title := feiNiuSyncTitle(row)
	plainText := strings.TrimSpace(row["plain_text"].String())
	data := g.Map{
		columns.ProfileNo: profileNo(row), columns.SourceType: "feiniu", columns.SourceNoteId: row["note_id"].Int64(), columns.SourceNoteUuid: row["note_uuid"].String(), columns.SourceKey: sourceKey,
		columns.SourceTextHash: contentHash, columns.ChannelId: row["source_channel_id"].Int64(), columns.Title: title, columns.Summary: summary(plainText), columns.PlainText: plainText, columns.HtmlText: row["html_text"].String(),
		columns.SourceCategoryCode: row["category_code"].String(), columns.Province: row["province"].String(), columns.City: row["city"].String(), columns.Age: row["age"].Int(), columns.Height: row["height"].Int(), columns.Weight: row["weight"].Int(), columns.CupSize: row["cup_size"].String(),
		columns.ExpectedLivingCost: row["expected_living_cost"].Int(), columns.HasVerificationVideo: yesNo(row["has_verification_video"].String()), columns.ImageCount: row["image_count"].Int(), columns.VideoCount: row["video_count"].Int(),
		columns.GroupParams: row["group_params"].String(), columns.TagParams: "", columns.TextBlockCount: row["text_block_count"].Int(), columns.StoragePolicy: row["storage_policy"].String(), columns.SourceRemark: row["remark"].String(),
		columns.SourceCreateBy: row["create_by"].String(), columns.SourceUpdateBy: row["update_by"].String(), columns.SourceCreatedAt: row["create_time"].GTime(), columns.SourceUpdatedAt: row["update_time"].GTime(), columns.SourceEditedAt: row["edited_at"].GTime(),
		columns.Visibility: consts.ContentVisibilityPublic, columns.ReviewStatus: consts.ContentReviewApproved, columns.ImportStatus: "feiniu_sync", columns.AdminRemark: "", columns.PublishedAt: publishedAt, columns.Status: 1, columns.DeletedAt: nil, columns.UpdatedAt: now,
	}
	profileModel := g.DB().Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).Unscoped()
	existing, err := profileModel.Clone().
		Where(columns.SourceType, "feiniu").
		Where(columns.SourceNoteId, row["note_id"].Int64()).
		One()
	if err != nil {
		return 0, 0, gerror.Wrap(err, "读取上架资料失败")
	}
	profileId := existing[columns.Id].Int64()
	if existing.IsEmpty() {
		data[columns.CreatedAt] = now
		profileId, err = profileModel.Clone().Data(data).InsertAndGetId()
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			existing, err = profileModel.Clone().
				Where(columns.SourceType, "feiniu").
				Where(columns.SourceNoteId, row["note_id"].Int64()).
				One()
			if err == nil && !existing.IsEmpty() {
				profileId = existing[columns.Id].Int64()
				_, err = profileModel.Clone().Where(columns.Id, profileId).Data(data).Update()
			}
		}
		if err == nil && profileId <= 0 {
			profileId, err = s.profileIdBySourceNote(ctx, row["note_id"].Int64())
		}
	} else {
		_, err = profileModel.Clone().Where(columns.Id, profileId).Data(data).Update()
	}
	if err != nil {
		return 0, 0, gerror.Wrap(err, "保存上架资料失败")
	}
	if profileId <= 0 {
		return 0, 0, gerror.New("保存上架资料失败：未获取资料ID")
	}
	taskId, err := s.upsertTask(ctx, cfg, row, accountId, profileId, title, plainText, publishedAt)
	if err != nil {
		return 0, 0, err
	}
	return profileId, taskId, nil
}

func (s *sSysSync) profileIdBySourceNote(ctx context.Context, noteId int64) (int64, error) {
	if noteId <= 0 {
		return 0, nil
	}
	columns := dao.ContentProfile.Columns()
	row, err := g.DB().Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).Unscoped().
		Where(columns.SourceType, "feiniu").
		Where(columns.SourceNoteId, noteId).
		OrderDesc(columns.Id).
		One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取上架资料ID失败")
	}
	return row[columns.Id].Int64(), nil
}

func (s *sSysSync) upsertTask(ctx context.Context, cfg gdb.Record, row gdb.Record, accountId, profileId int64, title, plainText string, publishedAt *gtime.Time) (int64, error) {
	if profileId <= 0 {
		return 0, gerror.New("创建上架任务失败：资料ID不能为空")
	}
	if accountId <= 0 {
		return 0, gerror.New("创建上架任务失败：上架账号不能为空")
	}
	clientRequestId := noteRequestKey(row)
	now := gtime.Now()
	data := g.Map{"tenant_id": cfg["target_tenant_id"].Int64(), "merchant_id": cfg["target_tenant_id"].Int64(), "account_id": accountId, "profile_id": profileId, "client_request_id": clientRequestId, "title": title, "province": row["province"].String(), "city": row["city"].String(), "plain_text": plainText, "media_count": row["image_count"].Int() + row["video_count"].Int(), "channel_id_json": "[]", "customer_remark": "", "anti_scan_enabled": 0, "tg_push_enabled": 0, "tg_status": "skipped", "status": psysin.PublishTaskStatusPublished, "submitted_at": publishedAt, "published_at": publishedAt, "deleted_at": nil, "updated_at": now}
	taskModel := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Unscoped()
	existing, err := taskModel.Clone().Where("tenant_id", cfg["target_tenant_id"].Int64()).Where("client_request_id", clientRequestId).One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取上架任务失败")
	}
	if existing.IsEmpty() {
		data["created_at"] = now
		id, e := taskModel.Clone().Data(data).InsertAndGetId()
		if e != nil && strings.Contains(strings.ToLower(e.Error()), "duplicate") {
			existing, err = taskModel.Clone().Where("tenant_id", cfg["target_tenant_id"].Int64()).Where("client_request_id", clientRequestId).One()
			if err == nil && !existing.IsEmpty() {
				id = existing["id"].Int64()
				_, err = taskModel.Clone().Where("id", id).Data(data).Update()
				return id, gerror.Wrap(err, "恢复上架任务失败")
			}
		}
		return id, gerror.Wrap(e, "创建上架任务失败")
	}
	_, err = taskModel.Clone().Where("id", existing["id"].Int64()).Data(data).Update()
	if err != nil {
		return 0, gerror.Wrap(err, "更新上架任务失败")
	}
	return existing["id"].Int64(), nil
}

func (s *sSysSync) syncMedia(ctx context.Context, source gdb.DB, cfg gdb.Record, row gdb.Record, profileId, taskId, accountId int64) error {
	var blocks []gdb.Record
	err := source.Model("tg_content_block b").Safe().Ctx(ctx).LeftJoin("tg_content_asset a", "a.asset_id=b.asset_id").LeftJoin("tg_content_asset_cos ac", "ac.asset_id=a.asset_id").Fields("b.block_id,b.block_type,b.sort_index,b.meta_json,a.asset_id,a.asset_type,a.mime_type,a.file_name,a.file_size,a.origin_uri,a.preview_uri,a.local_file_path,a.archive_chat_id,a.archive_message_id,a.binary_md5,a.perceptual_hash,a.width,a.height,a.duration,ac.cos_path").Where("b.note_id", row["note_id"].Int64()).Where("b.asset_id > 0").OrderAsc("b.sort_index").Scan(&blocks)
	if err != nil {
		return gerror.Wrap(err, "读取 FeiNiu 媒体失败")
	}
	if len(blocks) == 0 {
		return nil
	}
	_, _ = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Where("task_id", taskId).Where("profile_id", profileId).Data(g.Map{"deleted_at": gtime.Now()}).Update()
	now := gtime.Now()
	imageCount, videoCount := 0, 0
	for i, item := range blocks {
		mediaType := mediaTypeOf(item["asset_type"].String(), item["mime_type"].String())
		if mediaType == "video" {
			videoCount++
		} else {
			imageCount++
		}
		purpose := "display"
		if strings.Contains(strings.ToLower(item["meta_json"].String()), "validation") {
			purpose = "verify"
		}
		if purpose == "verify" && cfg["sync_verify_media"].Int() != 1 {
			continue
		}
		fileURL := feiNiuImportedMediaURL(item)
		storagePath := feiNiuImportedMediaStoragePath(item)
		posterURL, posterStoragePath := feiNiuImportedVideoPoster(item, mediaType)
		perceptualHash := item["perceptual_hash"].String()
		assetResult, processErr := publishservice.SysPublish().ProcessStoredMediaAssets(ctx, &psysin.StoredMediaAssetsInp{
			MediaType: mediaType,
			LocalPath: item["local_file_path"].String(),
			FileName:  mediaName(item),
		})
		if processErr != nil {
			return gerror.Wrap(processErr, "统一处理 FeiNiu 媒体失败")
		}
		if assetResult != nil && assetResult.Processed {
			if assetResult.PerceptualHash != "" {
				perceptualHash = assetResult.PerceptualHash
			}
			if assetResult.PosterUrl != "" {
				posterURL = assetResult.PosterUrl
				posterStoragePath = assetResult.PosterStoragePath
			}
		}
		if perceptualHash == "" {
			remoteAssets, remoteErr := publishservice.SysPublish().ProcessRemoteMediaAssets(ctx, &psysin.RemoteMediaAssetsInp{
				MediaType: mediaType,
				FileURL:   fileURL,
				PosterURL: posterURL,
			})
			if remoteErr != nil {
				logURL := fileURL
				if logURL == "" {
					logURL = posterURL
				}
				g.Log().Warningf(ctx, "远程处理 FeiNiu 媒体感知哈希失败 media:%s url:%s err:%+v", mediaType, logURL, remoteErr)
			}
			if remoteAssets != nil && remoteAssets.Processed {
				perceptualHash = remoteAssets.PerceptualHash
			}
		}
		data := g.Map{"tenant_id": cfg["target_tenant_id"].Int64(), "merchant_id": cfg["target_tenant_id"].Int64(), "account_id": accountId, "task_id": taskId, "profile_id": profileId, "attachment_id": 0, "media_type": mediaType, "purpose": purpose, "name": mediaName(item), "file_url": fileURL, "original_file_url": fileURL, "poster_url": posterURL, "poster_storage_path": posterStoragePath, "storage_path": storagePath, "original_storage_path": storagePath, "mime_type": item["mime_type"].String(), "md5": item["binary_md5"].String(), "perceptual_hash": perceptualHash, "tg_file_id": "", "tg_cache_status": "invalid", "size": item["file_size"].Int64(), "sort_index": i + 1, "status": 1, "created_at": now, "updated_at": now}
		mediaId, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Data(data).InsertAndGetId()
		if err != nil {
			return gerror.Wrap(err, "保存 FeiNiu 媒体失败")
		}
		if mediaId > 0 {
			if err = publishservice.SysPublish().SyncMediaPHashBucketByMediaId(ctx, mediaId); err != nil {
				return err
			}
		}
	}
	_, _ = dao.ContentProfile.Ctx(ctx).Where(dao.ContentProfile.Columns().Id, profileId).Data(g.Map{dao.ContentProfile.Columns().ImageCount: imageCount, dao.ContentProfile.Columns().VideoCount: videoCount, dao.ContentProfile.Columns().UpdatedAt: now}).Update()
	_, _ = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", taskId).Data(g.Map{"media_count": imageCount + videoCount, "updated_at": now}).Update()
	return nil
}

func saveProfileMap(ctx context.Context, configId int64, noteId int64, current gdb.Record, data g.Map) error {
	var err error
	if current.IsEmpty() {
		data["created_at"] = gtime.Now()
		_, err = g.DB().Model(profileMapTable).Safe().Ctx(ctx).Data(data).Insert()
		if err == nil {
			return nil
		}
		if !isUniqueConflictError(err) {
			return err
		}
		delete(data, "created_at")
		_, err = g.DB().Model(profileMapTable).Safe().Ctx(ctx).Where("config_id", configId).Where("feiniu_note_id", noteId).Data(data).Update()
		return err
	}
	_, err = g.DB().Model(profileMapTable).Safe().Ctx(ctx).Where("id", current["id"].Int64()).Data(data).Update()
	return err
}

func isUniqueConflictError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint") || strings.Contains(msg, "duplicate entry")
}

func (s *sSysSync) upsertProfileMapSkipped(ctx context.Context, configId int64, row gdb.Record, msg string) error {
	now := gtime.Now()
	data := g.Map{"config_id": configId, "feiniu_note_id": row["note_id"].Int64(), "feiniu_note_uuid": row["note_uuid"].String(), "feiniu_note_code": row["note_code"].String(), "feiniu_source_key": sourceKey(row), "feiniu_channel_id": row["source_channel_id"].Int64(), "feiniu_tg_chat_id": row["source_tg_chat_id"].Int64(), "source_updated_at": sourceTime(row), "content_hash": noteHash(row), "sync_status": "skipped", "error_message": msg, "updated_at": now}
	current, err := g.DB().Model(profileMapTable).Safe().Ctx(ctx).Where("config_id", configId).Where("feiniu_note_id", row["note_id"].Int64()).One()
	if err != nil {
		return gerror.Wrap(err, "读取资料映射失败")
	}
	if err = saveProfileMap(ctx, configId, row["note_id"].Int64(), current, data); err != nil {
		return gerror.Wrap(err, "保存跳过资料映射失败")
	}
	return s.touchChannelCursor(ctx, gdb.Record{"id": gvar.New(configId)}, row, sourceTime(row))
}

func (s *sSysSync) upsertProfileMapError(ctx context.Context, configId int64, row gdb.Record, msg string) error {
	data := g.Map{"config_id": configId, "feiniu_note_id": row["note_id"].Int64(), "feiniu_note_uuid": row["note_uuid"].String(), "feiniu_note_code": row["note_code"].String(), "feiniu_source_key": sourceKey(row), "feiniu_channel_id": row["source_channel_id"].Int64(), "feiniu_tg_chat_id": row["source_tg_chat_id"].Int64(), "sync_status": "failed", "error_message": msg, "source_updated_at": sourceTime(row), "updated_at": gtime.Now()}
	existing, _ := g.DB().Model(profileMapTable).Safe().Ctx(ctx).Where("config_id", configId).Where("feiniu_note_id", row["note_id"].Int64()).One()
	return saveProfileMap(ctx, configId, row["note_id"].Int64(), existing, data)
}

type channelRunStat struct {
	FeiniuChannelId       int64
	FeiniuTgChatId        int64
	FeiniuChannelTitle    string
	YoubanAccountId       int64
	YoubanAccountUsername string
	TotalCount            int
	CreatedCount          int
	UpdatedCount          int
	SkippedCount          int
	FailedCount           int
	LastNoteId            int64
	LastSourceUpdateTime  *gtime.Time
}

func accumulateChannelStat(stats map[int64]*channelRunStat, row gdb.Record, action string) {
	channelId := row["source_channel_id"].Int64()
	chatId := row["source_tg_chat_id"].Int64()
	statKey := channelId
	if chatId > 0 {
		statKey = chatId
	}
	item := stats[statKey]
	if item == nil {
		item = &channelRunStat{FeiniuChannelId: channelId, FeiniuTgChatId: chatId, FeiniuChannelTitle: row["channel_title"].String()}
		stats[statKey] = item
	}
	item.TotalCount++
	item.LastNoteId = row["note_id"].Int64()
	item.LastSourceUpdateTime = sourceTime(row)
	switch action {
	case "created":
		item.CreatedCount++
	case "updated":
		item.UpdatedCount++
	case "failed":
		item.FailedCount++
	default:
		item.SkippedCount++
	}
}

func (s *sSysSync) createRunItem(ctx context.Context, runId, configId int64, row gdb.Record, action, status, errMsg string, durationMs int64) error {
	profileMap, _ := g.DB().Model(profileMapTable).Safe().Ctx(ctx).Where("config_id", configId).Where("feiniu_note_id", row["note_id"].Int64()).One()
	data := g.Map{
		"run_id": runId, "config_id": configId, "feiniu_note_id": row["note_id"].Int64(), "feiniu_note_code": row["note_code"].String(),
		"feiniu_channel_id": row["source_channel_id"].Int64(), "feiniu_channel_title": row["channel_title"].String(), "action": action, "status": status,
		"error_message": errMsg, "source_updated_at": sourceTime(row), "duration_ms": durationMs, "created_at": gtime.Now(),
	}
	if !profileMap.IsEmpty() {
		data["youban_profile_id"] = profileMap["youban_profile_id"].Int64()
		data["youban_task_id"] = profileMap["youban_task_id"].Int64()
	}
	_, err := g.DB().Model(runItemTable).Safe().Ctx(ctx).Data(data).Insert()
	return err
}

func (s *sSysSync) flushRunStats(ctx context.Context, runId, configId int64, total, created, updated, skipped, failed int, durationMs int64, channels map[int64]*channelRunStat) error {
	statDate := time.Now().Format("2006-01-02")
	if err := s.upsertDailyStat(ctx, statDate, runId, configId, total, created, updated, skipped, failed, durationMs, len(channels)); err != nil {
		return err
	}
	for _, item := range channels {
		if err := s.fillChannelAccountForStat(ctx, configId, item); err != nil {
			g.Log().Warning(ctx, err)
		}
		if err := s.upsertChannelDailyStat(ctx, statDate, configId, item); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysSync) fillChannelAccountForStat(ctx context.Context, configId int64, item *channelRunStat) error {
	mod := g.DB().Model(channelMapTable).Safe().Ctx(ctx).Where("config_id", configId)
	if item.FeiniuTgChatId > 0 {
		mod = mod.Where("feiniu_tg_chat_id", item.FeiniuTgChatId).OrderAsc("id")
	} else {
		mod = mod.Where("feiniu_channel_id", item.FeiniuChannelId)
	}
	row, err := mod.One()
	if err != nil || row.IsEmpty() {
		return err
	}
	item.YoubanAccountId = row["youban_account_id"].Int64()
	item.YoubanAccountUsername = row["youban_account_username"].String()
	if item.FeiniuChannelTitle == "" {
		item.FeiniuChannelTitle = row["feiniu_channel_title"].String()
	}
	return nil
}

func (s *sSysSync) upsertDailyStat(ctx context.Context, statDate string, runId, configId int64, total, created, updated, skipped, failed int, durationMs int64, channelCount int) error {
	now := gtime.Now()
	existing, err := g.DB().Model(dailyStatTable).Safe().Ctx(ctx).Where("config_id", configId).Where("stat_date", statDate).One()
	if err != nil {
		return gerror.Wrap(err, "读取每日统计失败")
	}
	success := total - failed
	data := g.Map{"stat_date": statDate, "config_id": configId, "run_count": 1, "total_count": total, "success_count": success, "created_count": created, "updated_count": updated, "skipped_count": skipped, "failed_count": failed, "channel_count": channelCount, "profile_count": total, "avg_duration_ms": durationMs, "last_run_id": runId, "last_run_at": now, "updated_at": now}
	if existing.IsEmpty() {
		data["created_at"] = now
		_, err = g.DB().Model(dailyStatTable).Safe().Ctx(ctx).Data(data).Insert()
		return err
	}
	runCount := existing["run_count"].Int()
	avg := durationMs
	if runCount > 0 {
		avg = (existing["avg_duration_ms"].Int64()*int64(runCount) + durationMs) / int64(runCount+1)
	}
	_, err = g.DB().Model(dailyStatTable).Safe().Ctx(ctx).Where("id", existing["id"].Int64()).Data(g.Map{"run_count": gdb.Raw("run_count+1"), "total_count": gdb.Raw(fmt.Sprintf("total_count+%d", total)), "success_count": gdb.Raw(fmt.Sprintf("success_count+%d", success)), "created_count": gdb.Raw(fmt.Sprintf("created_count+%d", created)), "updated_count": gdb.Raw(fmt.Sprintf("updated_count+%d", updated)), "skipped_count": gdb.Raw(fmt.Sprintf("skipped_count+%d", skipped)), "failed_count": gdb.Raw(fmt.Sprintf("failed_count+%d", failed)), "channel_count": channelCount, "profile_count": gdb.Raw(fmt.Sprintf("profile_count+%d", total)), "avg_duration_ms": avg, "last_run_id": runId, "last_run_at": now, "updated_at": now}).Update()
	return err
}

func (s *sSysSync) upsertChannelDailyStat(ctx context.Context, statDate string, configId int64, item *channelRunStat) error {
	now := gtime.Now()
	existing, err := g.DB().Model(channelStatTable).Safe().Ctx(ctx).Where("config_id", configId).Where("stat_date", statDate).Where("feiniu_channel_id", item.FeiniuChannelId).One()
	if err != nil {
		return gerror.Wrap(err, "读取频道每日统计失败")
	}
	data := g.Map{"stat_date": statDate, "config_id": configId, "feiniu_channel_id": item.FeiniuChannelId, "feiniu_tg_chat_id": item.FeiniuTgChatId, "feiniu_channel_title": item.FeiniuChannelTitle, "youban_account_id": item.YoubanAccountId, "youban_account_username": item.YoubanAccountUsername, "total_count": item.TotalCount, "created_count": item.CreatedCount, "updated_count": item.UpdatedCount, "skipped_count": item.SkippedCount, "failed_count": item.FailedCount, "last_note_id": item.LastNoteId, "last_source_update_time": item.LastSourceUpdateTime, "updated_at": now}
	if existing.IsEmpty() {
		data["created_at"] = now
		_, err = g.DB().Model(channelStatTable).Safe().Ctx(ctx).Data(data).Insert()
		return err
	}
	_, err = g.DB().Model(channelStatTable).Safe().Ctx(ctx).Where("id", existing["id"].Int64()).Data(g.Map{"feiniu_tg_chat_id": item.FeiniuTgChatId, "feiniu_channel_title": item.FeiniuChannelTitle, "youban_account_id": item.YoubanAccountId, "youban_account_username": item.YoubanAccountUsername, "total_count": gdb.Raw(fmt.Sprintf("total_count+%d", item.TotalCount)), "created_count": gdb.Raw(fmt.Sprintf("created_count+%d", item.CreatedCount)), "updated_count": gdb.Raw(fmt.Sprintf("updated_count+%d", item.UpdatedCount)), "skipped_count": gdb.Raw(fmt.Sprintf("skipped_count+%d", item.SkippedCount)), "failed_count": gdb.Raw(fmt.Sprintf("failed_count+%d", item.FailedCount)), "last_note_id": item.LastNoteId, "last_source_update_time": item.LastSourceUpdateTime, "updated_at": now}).Update()
	return err
}

func normalizeDateRange(start, end string) (string, string) {
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)
	today := time.Now()
	if end == "" {
		end = today.Format("2006-01-02")
	}
	if start == "" {
		start = today.AddDate(0, 0, -6).Format("2006-01-02")
	}
	return start, end
}

func latestRunModel(mod *gdb.Model, configId int64) *gdb.Model {
	if configId > 0 {
		mod = mod.Where("config_id", configId)
	}
	return mod.Fields(runFields()).OrderDesc("id").Limit(1)
}

func sourceDB(ctx context.Context, in *sysin.ConfigSaveInp) (gdb.DB, error) {
	dbType := normalizeDBType(in.DbType)
	group := fmt.Sprintf("feiniu_sync_%s_%s_%d", encrypt.Md5ToString(fmt.Sprintf("%s:%d:%s:%s", in.DbHost, in.DbPort, in.DbName, in.DbUser))[:12], dbType, time.Now().UnixNano())
	node := gdb.ConfigNode{Type: dbType, Host: in.DbHost, Port: fmt.Sprintf("%d", in.DbPort), User: in.DbUser, Pass: in.DbPassword, Name: in.DbName, Role: "master", Charset: "utf8mb4", Protocol: "tcp", MaxIdleConnCount: 1, MaxOpenConnCount: 3, QueryTimeout: 60 * time.Second, ExecTimeout: 60 * time.Second}
	if dbType == "pgsql" {
		node.Extra = "connect_timeout=5"
	} else {
		node.Extra = "parseTime=true&timeout=5s&readTimeout=60s&writeTimeout=60s"
	}
	if err := gdb.AddConfigNode(group, node); err != nil {
		return nil, gerror.Wrap(err, "注册 FeiNiu 数据源失败")
	}
	return g.DB(group), nil
}

func (s *sSysSync) configRecord(ctx context.Context, id int64) (gdb.Record, error) {
	row, err := g.DB().Model(configTable).Safe().Ctx(ctx).Where("id", id).WhereNull("deleted_at").One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取同步配置失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("同步配置不存在")
	}
	return row, nil
}
func configRecordToSave(row gdb.Record) *sysin.ConfigSaveInp {
	return &sysin.ConfigSaveInp{Id: row["id"].Int64(), Name: row["name"].String(), DbType: row["db_type"].String(), DbHost: row["db_host"].String(), DbPort: row["db_port"].Int(), DbName: row["db_name"].String(), DbUser: row["db_user"].String(), DbPassword: decodePassword(row["db_password"].String()), TargetTenantId: row["target_tenant_id"].Int64(), TargetParentAccountId: row["target_parent_account_id"].Int64(), AutoCreateAccount: row["auto_create_account"].Int(), SyncMedia: row["sync_media"].Int(), SyncVerifyMedia: row["sync_verify_media"].Int(), AutoSyncEnabled: row["auto_sync_enabled"].Int(), SyncIntervalMinutes: row["sync_interval_minutes"].Int(), BatchSize: row["batch_size"].Int(), Status: row["status"].Int()}
}
func configFields() string {
	return "id,name,db_type,db_host,db_port,db_name,db_user,target_tenant_id,target_parent_account_id,auto_create_account,sync_media,sync_verify_media,auto_sync_enabled,sync_interval_minutes,batch_size,last_source_note_id,status,last_run_at,last_success_at,last_error,created_at,updated_at"
}
func runFields() string {
	return "id,config_id,run_type,status,total_count,created_count,updated_count,skipped_count,failed_count,started_at,finished_at,error_message,runtime_log,created_at"
}
func normalizeDBType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	if t == "postgresql" {
		return "pgsql"
	}
	if t == "" {
		return "mysql"
	}
	return t
}
func encodePassword(p string) string { return base64.StdEncoding.EncodeToString([]byte(p)) }
func decodePassword(p string) string {
	b, err := base64.StdEncoding.DecodeString(p)
	if err != nil {
		return p
	}
	return string(b)
}

func noteRequestKey(row gdb.Record) string {
	return fmt.Sprintf("feiniu:note:%d", row["note_id"].Int64())
}

func feiNiuSyncTitle(row gdb.Record) string {
	if title := strings.TrimSpace(row["note_code"].String()); title != "" {
		return title
	}
	return fmt.Sprintf("FeiNiu资料%d", row["note_id"].Int64())
}

func sourceKey(row gdb.Record) string {
	if v := strings.TrimSpace(row["source_key"].String()); v != "" {
		return "feiniu:" + v
	}
	return fmt.Sprintf("feiniu:note:%d", row["note_id"].Int64())
}
func sourceTime(row gdb.Record) *gtime.Time {
	if t := row["edited_at"].GTime(); t != nil {
		return t
	}
	if t := row["update_time"].GTime(); t != nil {
		return t
	}
	if t := row["create_time"].GTime(); t != nil {
		return t
	}
	return gtime.Now()
}
func noteHash(row gdb.Record) string {
	return encrypt.Md5ToString(strings.Join([]string{row["title"].String(), row["plain_text"].String(), row["update_time"].String(), row["edited_at"].String()}, "|"))
}
func profileNo(row gdb.Record) string {
	if code := strings.TrimSpace(row["note_code"].String()); code != "" {
		return "FN" + code
	}
	return fmt.Sprintf("FN%08d", row["note_id"].Int64())
}
func summary(text string) string {
	r := []rune(strings.TrimSpace(text))
	if len(r) > 80 {
		return string(r[:80])
	}
	return string(r)
}
func yesNo(v string) int {
	v = strings.ToUpper(strings.TrimSpace(v))
	if v == "Y" || v == "1" || v == "TRUE" {
		return 1
	}
	return 0
}
func mediaTypeOf(assetType, mime string) string {
	v := strings.ToLower(assetType + " " + mime)
	if strings.Contains(v, "video") {
		return "video"
	}
	return "image"
}
func mediaName(row gdb.Record) string {
	if v := strings.TrimSpace(row["file_name"].String()); v != "" {
		return v
	}
	return fmt.Sprintf("feiniu_asset_%d", row["asset_id"].Int64())
}
func mediaURL(row gdb.Record) string {
	for _, k := range []string{"cos_path", "origin_uri", "preview_uri", "local_file_path"} {
		if v := strings.TrimSpace(row[k].String()); v != "" {
			return v
		}
	}
	if row["archive_chat_id"].Int64() != 0 && row["archive_message_id"].Int64() != 0 {
		return fmt.Sprintf("tg://%d/%d", row["archive_chat_id"].Int64(), row["archive_message_id"].Int64())
	}
	return ""
}
