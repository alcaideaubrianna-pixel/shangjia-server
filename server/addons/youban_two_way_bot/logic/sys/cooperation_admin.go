package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
	twdao "hotgo/addons/youban_two_way_bot/internal/dao"
	"hotgo/addons/youban_two_way_bot/internal/model/entity"
	"hotgo/addons/youban_two_way_bot/model/input/sysin"
	"hotgo/internal/library/cache"
)

const cooperationImportApplicant = "__platform_import__"

func (s *sSysTwoWayBot) AdminCooperationConfigView(ctx context.Context) (*sysin.CooperationConfigModel, error) {
	account, err := currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.cooperationConfigModel(ctx, account.TenantId)
}

func (s *sSysTwoWayBot) AdminCooperationConfigSave(ctx context.Context, in *sysin.CooperationConfigSaveInp) (*sysin.CooperationConfigModel, error) {
	if in == nil {
		return nil, gerror.New("配置不能为空")
	}
	if err := in.Filter(ctx); err != nil {
		return nil, err
	}
	account, err := currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	twoWayBot, err := s.botById(ctx, in.TwoWayBotId, account.TenantId)
	if err != nil {
		return nil, err
	}
	if twoWayBot.Status != 1 {
		return nil, gerror.New("双向机器人未启用")
	}
	in.BotId, err = cooperationPublishBotIdByToken(ctx, account.TenantId, twoWayBot.BotToken)
	if err != nil {
		return nil, err
	}
	if err = ensureCooperationChannels(ctx, account.TenantId, in.ChannelIds); err != nil {
		return nil, err
	}
	now := gtime.Now()
	configColumns := twdao.YoubanTwoWayBotCooperationConfig.Columns()
	channelColumns := twdao.YoubanTwoWayBotCooperationChannel.Columns()
	err = twdao.YoubanTwoWayBotCooperationConfig.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var config *entity.YoubanTwoWayBotCooperationConfig
		if scanErr := tx.Model(twdao.YoubanTwoWayBotCooperationConfig.Table()).Where(configColumns.TenantId, account.TenantId).WhereNull(configColumns.DeletedAt).Scan(&config); scanErr != nil {
			return scanErr
		}
		data := g.Map{configColumns.AccountId: account.Id, configColumns.BotId: in.BotId, configColumns.TwoWayBotId: in.TwoWayBotId, configColumns.NotificationType: in.NotificationType, configColumns.ReviewRequired: in.ReviewRequired, configColumns.Status: in.Status, configColumns.UpdatedBy: account.Id, configColumns.UpdatedAt: now}
		configId := int64(0)
		if config != nil && config.Id > 0 {
			configId = config.Id
			if _, updateErr := tx.Model(twdao.YoubanTwoWayBotCooperationConfig.Table()).Where(configColumns.Id, configId).Data(data).Update(); updateErr != nil {
				return updateErr
			}
		} else {
			data[configColumns.TenantId] = account.TenantId
			data[configColumns.CreatedBy] = account.Id
			data[configColumns.CreatedAt] = now
			id, insertErr := tx.Model(twdao.YoubanTwoWayBotCooperationConfig.Table()).Data(data).InsertAndGetId()
			if insertErr != nil {
				return insertErr
			}
			configId = id
		}
		if _, deleteErr := tx.Model(twdao.YoubanTwoWayBotCooperationChannel.Table()).Where(channelColumns.ConfigId, configId).Delete(); deleteErr != nil {
			return deleteErr
		}
		for _, channelId := range uniqueInt64(in.ChannelIds) {
			if _, insertErr := tx.Model(twdao.YoubanTwoWayBotCooperationChannel.Table()).Data(g.Map{channelColumns.TenantId: account.TenantId, channelColumns.ConfigId: configId, channelColumns.ChannelId: channelId, channelColumns.Status: 1, channelColumns.CreatedAt: now, channelColumns.UpdatedAt: now}).Insert(); insertErr != nil {
				return insertErr
			}
		}
		return nil
	})
	if err != nil {
		return nil, gerror.Wrap(err, "保存平台合作配置失败")
	}
	if refreshErr := gatewayservice.Gateway().Refresh(ctx); refreshErr != nil {
		g.Log().Warningf(ctx, "平台合作配置已保存，但刷新Bot菜单失败：%+v", refreshErr)
	}
	return s.cooperationConfigModel(ctx, account.TenantId)
}

func (s *sSysTwoWayBot) cooperationConfigModel(ctx context.Context, tenantId int64) (*sysin.CooperationConfigModel, error) {
	columns := twdao.YoubanTwoWayBotCooperationConfig.Columns()
	var row *entity.YoubanTwoWayBotCooperationConfig
	if err := twdao.YoubanTwoWayBotCooperationConfig.Ctx(ctx).Where(columns.TenantId, tenantId).WhereNull(columns.DeletedAt).Scan(&row); err != nil {
		return nil, err
	}
	if row == nil {
		return &sysin.CooperationConfigModel{ReviewRequired: 1, Status: 1, NotificationType: "two_way", ChannelIds: []int64{}}, nil
	}
	var bot struct {
		BotName     string `json:"botName"`
		BotUsername string `json:"botUsername"`
	}
	_ = g.DB().Model("hg_youban_publish_bot").Ctx(ctx).Fields("bot_name", "bot_username").Where("id", row.BotId).Scan(&bot)
	channelColumns := twdao.YoubanTwoWayBotCooperationChannel.Columns()
	var channelRows []struct {
		ChannelId int64 `json:"channelId"`
	}
	_ = twdao.YoubanTwoWayBotCooperationChannel.Ctx(ctx).Fields(channelColumns.ChannelId).Where(channelColumns.ConfigId, row.Id).WhereNull(channelColumns.DeletedAt).Scan(&channelRows)
	channelIds := make([]int64, 0, len(channelRows))
	for _, item := range channelRows {
		channelIds = append(channelIds, item.ChannelId)
	}
	return &sysin.CooperationConfigModel{Id: row.Id, BotId: row.BotId, BotName: bot.BotName, BotUsername: bot.BotUsername, TwoWayBotId: row.TwoWayBotId, NotificationType: row.NotificationType, ReviewRequired: row.ReviewRequired, Status: row.Status, ChannelIds: channelIds}, nil
}

func (s *sSysTwoWayBot) AdminCooperationApplicationList(ctx context.Context, in *sysin.CooperationApplicationListInp) ([]*sysin.CooperationApplicationModel, int, error) {
	account, err := currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.CooperationApplicationListInp{}
	}
	columns := twdao.YoubanTwoWayBotCooperationApplication.Columns()
	mod := twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(columns.TenantId, account.TenantId).WhereNot(columns.ApplicantTgUserId, cooperationImportApplicant).WhereNull(columns.DeletedAt)
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(applicant_username LIKE ? OR submitted_bot_username LIKE ? OR submitted_bot_name LIKE ?)", like, like, like)
	}
	if in.ReviewStatus != "" {
		mod = mod.Where(columns.ReviewStatus, in.ReviewStatus)
	}
	if in.JoinStatus != "" {
		mod = mod.Where(columns.JoinStatus, in.JoinStatus)
	}
	var rows []*entity.YoubanTwoWayBotCooperationApplication
	total := 0
	if err = mod.Page(in.Page, in.PerPage).OrderDesc(columns.Id).ScanAndCount(&rows, &total, false); err != nil {
		return nil, 0, err
	}
	list := make([]*sysin.CooperationApplicationModel, 0, len(rows))
	for _, row := range rows {
		channels, _ := cooperationApplicationChannels(ctx, row.Id)
		blacklistColumns := twdao.YoubanTwoWayBotCooperationBlacklist.Columns()
		blocked, _ := twdao.YoubanTwoWayBotCooperationBlacklist.Ctx(ctx).Where(blacklistColumns.ConfigId, row.ConfigId).Where(blacklistColumns.ApplicantTgUserId, row.ApplicantTgUserId).Where(blacklistColumns.Status, 1).Count()
		blacklisted := 0
		if blocked > 0 {
			blacklisted = 1
		}
		list = append(list, &sysin.CooperationApplicationModel{Id: row.Id, ApplicantTgUserId: row.ApplicantTgUserId, ApplicantUsername: row.ApplicantUsername, ApplicantFirstName: row.ApplicantFirstName, ApplicantLastName: row.ApplicantLastName, SubmittedBotUserId: row.SubmittedBotUserId, SubmittedBotUsername: row.SubmittedBotUsername, SubmittedBotName: row.SubmittedBotName, ReviewStatus: row.ReviewStatus, JoinStatus: row.JoinStatus, ErrorMessage: row.ErrorMessage, SubmittedAt: row.SubmittedAt, ReviewedAt: row.ReviewedAt, Blacklisted: blacklisted, Channels: channels})
	}
	return list, total, nil
}

func (s *sSysTwoWayBot) AdminCooperationApplicationApprove(ctx context.Context, in *sysin.CooperationApplicationActionInp) error {
	account, err := currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	return s.approveCooperationApplication(ctx, in.Id, account.TenantId, account.Id, strings.TrimSpace(in.Remark))
}
func (s *sSysTwoWayBot) AdminCooperationApplicationRetry(ctx context.Context, in *sysin.CooperationApplicationActionInp) error {
	account, err := currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	return s.processCooperationApplication(ctx, in.Id, account.TenantId)
}
func (s *sSysTwoWayBot) AdminCooperationApplicationReject(ctx context.Context, in *sysin.CooperationApplicationActionInp) error {
	return s.updateCooperationReview(ctx, in, sysin.CooperationReviewRejected)
}
func (s *sSysTwoWayBot) AdminCooperationApplicationCancel(ctx context.Context, in *sysin.CooperationApplicationActionInp) error {
	return s.updateCooperationReview(ctx, in, sysin.CooperationReviewCanceled)
}
func (s *sSysTwoWayBot) AdminCooperationApplicationTerminate(ctx context.Context, in *sysin.CooperationApplicationActionInp) error {
	if in == nil || in.Id <= 0 {
		return gerror.New("请选择合作申请")
	}
	account, err := currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	return s.terminateCooperationApplication(ctx, in.Id, account.TenantId, account.Id, strings.TrimSpace(in.Remark))
}
func (s *sSysTwoWayBot) AdminCooperationApplicationBlacklist(ctx context.Context, in *sysin.CooperationApplicationActionInp) error {
	if in == nil || in.Id <= 0 {
		return gerror.New("请选择合作申请")
	}
	account, err := currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	return s.blacklistCooperationApplication(ctx, in.Id, account.TenantId, account.Id, strings.TrimSpace(in.Remark))
}

func (s *sSysTwoWayBot) blacklistCooperationApplication(ctx context.Context, id, tenantId, reviewerId int64, reason string) error {
	appColumns := twdao.YoubanTwoWayBotCooperationApplication.Columns()
	var app *entity.YoubanTwoWayBotCooperationApplication
	if err := twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(appColumns.Id, id).Where(appColumns.TenantId, tenantId).WhereNull(appColumns.DeletedAt).Scan(&app); err != nil || app == nil {
		return gerror.New("合作申请不存在")
	}
	columns := twdao.YoubanTwoWayBotCooperationBlacklist.Columns()
	var row *entity.YoubanTwoWayBotCooperationBlacklist
	_ = twdao.YoubanTwoWayBotCooperationBlacklist.Ctx(ctx).Where(columns.ConfigId, app.ConfigId).Where(columns.ApplicantTgUserId, app.ApplicantTgUserId).Scan(&row)
	data := g.Map{columns.TenantId: tenantId, columns.ConfigId: app.ConfigId, columns.ApplicantTgUserId: app.ApplicantTgUserId, columns.ApplicantUsername: app.ApplicantUsername, columns.ApplicantFirstName: app.ApplicantFirstName, columns.ApplicantLastName: app.ApplicantLastName, columns.Reason: reason, columns.Status: 1, columns.UpdatedBy: reviewerId, columns.UpdatedAt: gtime.Now()}
	var err error
	if row != nil && row.Id > 0 {
		_, err = twdao.YoubanTwoWayBotCooperationBlacklist.Ctx(ctx).Where(columns.Id, row.Id).Data(data).Update()
	} else {
		data[columns.CreatedBy] = reviewerId
		data[columns.CreatedAt] = gtime.Now()
		_, err = twdao.YoubanTwoWayBotCooperationBlacklist.Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return err
	}
	if app.ReviewStatus == sysin.CooperationReviewPending {
		_, _ = twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(appColumns.Id, app.Id).Data(g.Map{appColumns.ReviewStatus: sysin.CooperationReviewRejected, appColumns.ReviewRemark: "申请人已被拉黑", appColumns.ReviewedBy: reviewerId, appColumns.ReviewedAt: gtime.Now(), appColumns.UpdatedAt: gtime.Now()}).Update()
	}
	return nil
}
func (s *sSysTwoWayBot) AdminCooperationApplicationUnblacklist(ctx context.Context, in *sysin.CooperationApplicationActionInp) error {
	if in == nil || in.Id <= 0 {
		return gerror.New("请选择合作申请")
	}
	account, err := currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	appColumns := twdao.YoubanTwoWayBotCooperationApplication.Columns()
	var app *entity.YoubanTwoWayBotCooperationApplication
	if err = twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(appColumns.Id, in.Id).Where(appColumns.TenantId, account.TenantId).Scan(&app); err != nil || app == nil {
		return gerror.New("合作申请不存在")
	}
	columns := twdao.YoubanTwoWayBotCooperationBlacklist.Columns()
	_, err = twdao.YoubanTwoWayBotCooperationBlacklist.Ctx(ctx).Where(columns.ConfigId, app.ConfigId).Where(columns.ApplicantTgUserId, app.ApplicantTgUserId).Data(g.Map{columns.Status: 2, columns.UpdatedBy: account.Id, columns.UpdatedAt: gtime.Now()}).Update()
	return err
}
func (s *sSysTwoWayBot) updateCooperationReview(ctx context.Context, in *sysin.CooperationApplicationActionInp, status string) error {
	account, err := currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	columns := twdao.YoubanTwoWayBotCooperationApplication.Columns()
	result, err := twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(columns.Id, in.Id).Where(columns.TenantId, account.TenantId).Where(columns.ReviewStatus, sysin.CooperationReviewPending).Data(g.Map{columns.ReviewStatus: status, columns.ReviewedBy: account.Id, columns.ReviewRemark: strings.TrimSpace(in.Remark), columns.ReviewedAt: gtime.Now(), columns.UpdatedAt: gtime.Now()}).Update()
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return gerror.New("申请状态已变化，请刷新后重试")
	}
	return nil
}

func cooperationPublishBotIdByToken(ctx context.Context, tenantId int64, token string) (int64, error) {
	var row struct {
		Id int64 `json:"id"`
	}
	err := g.DB().Model("hg_youban_publish_bot").Ctx(ctx).
		Fields("id").
		Where("tenant_id", tenantId).
		Where("bot_token", strings.TrimSpace(token)).
		Where("status", 1).
		WhereNull("deleted_at").
		OrderDesc("id").
		Scan(&row)
	if err != nil {
		return 0, err
	}
	if row.Id <= 0 {
		return 0, gerror.New("该双向机器人未关联可用的 Bot Token 配置")
	}
	return row.Id, nil
}
func ensureCooperationChannels(ctx context.Context, tenantId int64, ids []int64) error {
	ids = uniqueInt64(ids)
	count, err := g.DB().Model("hg_youban_publish_channel").Ctx(ctx).WhereIn("id", ids).Where("tenant_id", tenantId).Where("status", 1).WhereNull("deleted_at").Count()
	if err != nil {
		return err
	}
	if count != len(ids) {
		return gerror.New("存在不可用的上架频道")
	}
	return nil
}
func uniqueInt64(values []int64) []int64 {
	seen := map[int64]struct{}{}
	out := make([]int64, 0, len(values))
	for _, v := range values {
		if v <= 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
func cooperationApplicationChannels(ctx context.Context, applicationId int64) ([]*sysin.CooperationApplicationChannelModel, error) {
	columns := twdao.YoubanTwoWayBotCooperationApplicationChannel.Columns()
	var rows []struct {
		ChannelId    int64       `json:"channelId"`
		Status       string      `json:"status"`
		ErrorMessage string      `json:"errorMessage"`
		JoinedAt     *gtime.Time `json:"joinedAt"`
		ChannelTitle string      `json:"channelTitle"`
	}
	err := twdao.YoubanTwoWayBotCooperationApplicationChannel.Ctx(ctx).As("ac").Fields("ac.channel_id,ac.status,ac.error_message,ac.joined_at,c.channel_title").LeftJoin("hg_youban_publish_channel c", "c.id=ac.channel_id").Where("ac."+columns.ApplicationId, applicationId).OrderAsc("ac.id").Scan(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]*sysin.CooperationApplicationChannelModel, 0, len(rows))
	for _, row := range rows {
		out = append(out, &sysin.CooperationApplicationChannelModel{ChannelId: row.ChannelId, ChannelTitle: row.ChannelTitle, Status: row.Status, ErrorMessage: row.ErrorMessage, JoinedAt: row.JoinedAt})
	}
	return out, nil
}

func (s *sSysTwoWayBot) AdminCooperationImport(ctx context.Context, in *sysin.CooperationImportInp) (*sysin.CooperationImportModel, error) {
	if in == nil {
		return nil, gerror.New("导入参数不能为空")
	}
	if err := in.Filter(ctx); err != nil {
		return nil, err
	}
	account, err := currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	configColumns := twdao.YoubanTwoWayBotCooperationConfig.Columns()
	var config *entity.YoubanTwoWayBotCooperationConfig
	if err = twdao.YoubanTwoWayBotCooperationConfig.Ctx(ctx).
		Where(configColumns.TenantId, account.TenantId).
		Where(configColumns.Status, 1).
		WhereNull(configColumns.DeletedAt).
		Scan(&config); err != nil {
		return nil, gerror.Wrap(err, "读取平台合作配置失败")
	}
	if config == nil || config.Id <= 0 {
		return nil, gerror.New("请先保存平台合作配置")
	}
	channels, err := cooperationChannels(ctx, config.Id)
	if err != nil {
		return nil, gerror.Wrap(err, "读取合作频道失败")
	}
	if len(channels) == 0 {
		return nil, gerror.New("平台尚未配置可用频道")
	}
	runtime := &cooperationConfigRuntime{Id: config.Id, TenantId: config.TenantId, BotId: config.BotId, TwoWayBotId: config.TwoWayBotId, ReviewRequired: config.ReviewRequired, NotificationType: config.NotificationType}
	resolved, err := s.resolveCooperationBot(ctx, runtime, in.BotUsername)
	if err != nil {
		return nil, err
	}
	appColumns := twdao.YoubanTwoWayBotCooperationApplication.Columns()
	var app *entity.YoubanTwoWayBotCooperationApplication
	if err = twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).
		Where(appColumns.TenantId, account.TenantId).
		Where(appColumns.ConfigId, config.Id).
		Where(appColumns.SubmittedBotUserId, strconv.FormatInt(resolved.UserId, 10)).
		Where(appColumns.ApplicantTgUserId, cooperationImportApplicant).
		WhereNull(appColumns.DeletedAt).
		OrderDesc(appColumns.Id).
		Scan(&app); err != nil {
		return nil, gerror.Wrap(err, "查询机器人导入记录失败")
	}
	if app != nil && app.Id > 0 && app.JoinStatus == sysin.CooperationJoinSuccess {
		channelColumns := twdao.YoubanTwoWayBotCooperationApplicationChannel.Columns()
		successCount, countErr := twdao.YoubanTwoWayBotCooperationApplicationChannel.Ctx(ctx).
			Where(channelColumns.ApplicationId, app.Id).
			Where(channelColumns.Status, sysin.CooperationJoinSuccess).
			WhereIn(channelColumns.ChannelId, cooperationChannelIds(channels)).
			Count()
		if countErr == nil && successCount == len(channels) {
			return s.cooperationImportResult(ctx, app, "exists"), nil
		}
	}
	rateKey := fmt.Sprintf("ybtwb:cooperation:import:tenant:%d", account.TenantId)
	if value, getErr := cache.Instance().Get(ctx, rateKey); getErr == nil && !value.IsNil() {
		result := &sysin.CooperationImportModel{ApplicationId: appId(app), BotUsername: resolved.Username, BotName: resolved.Name, Status: "rate_limited", Message: "Telegram操作频繁，请稍后重试"}
		return result, nil
	}
	_ = cache.Instance().Set(ctx, rateKey, 1, 8*time.Second)
	if app == nil || app.Id <= 0 {
		app, err = s.createCooperationImportApplication(ctx, config, resolved, channels)
		if err != nil {
			return nil, err
		}
	}
	if err = s.processCooperationImport(ctx, app, channels); err != nil {
		return nil, err
	}
	return s.cooperationImportResult(ctx, app, "completed"), nil
}

func (s *sSysTwoWayBot) createCooperationImportApplication(ctx context.Context, config *entity.YoubanTwoWayBotCooperationConfig, bot *cooperationResolvedBot, channels []*cooperationChannelRuntime) (*entity.YoubanTwoWayBotCooperationApplication, error) {
	columns := twdao.YoubanTwoWayBotCooperationApplication.Columns()
	now := gtime.Now()
	id, err := twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Data(g.Map{
		columns.TenantId:             config.TenantId,
		columns.ConfigId:             config.Id,
		columns.ApplicantTgUserId:    cooperationImportApplicant,
		columns.SubmittedBotUserId:   strconv.FormatInt(bot.UserId, 10),
		columns.SubmittedBotUsername: bot.Username,
		columns.SubmittedBotName:     bot.Name,
		columns.ReviewStatus:         sysin.CooperationReviewNotRequired,
		columns.JoinStatus:           sysin.CooperationJoinNotStarted,
		columns.SubmittedAt:          now,
		columns.CreatedAt:            now,
		columns.UpdatedAt:            now,
	}).InsertAndGetId()
	if err != nil {
		return nil, gerror.Wrap(err, "创建机器人导入记录失败")
	}
	channelColumns := twdao.YoubanTwoWayBotCooperationApplicationChannel.Columns()
	for _, channel := range channels {
		if _, err = twdao.YoubanTwoWayBotCooperationApplicationChannel.Ctx(ctx).Data(g.Map{
			channelColumns.TenantId:      config.TenantId,
			channelColumns.ApplicationId: id,
			channelColumns.ChannelId:     channel.Id,
			channelColumns.Status:        sysin.CooperationJoinNotStarted,
			channelColumns.CreatedAt:     now,
			channelColumns.UpdatedAt:     now,
		}).Insert(); err != nil {
			return nil, gerror.Wrap(err, "创建机器人导入频道记录失败")
		}
	}
	var app *entity.YoubanTwoWayBotCooperationApplication
	if err = twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(columns.Id, id).Scan(&app); err != nil {
		return nil, err
	}
	return app, nil
}

func (s *sSysTwoWayBot) processCooperationImport(ctx context.Context, app *entity.YoubanTwoWayBotCooperationApplication, channels []*cooperationChannelRuntime) error {
	if app == nil || app.Id <= 0 {
		return gerror.New("机器人导入记录不存在")
	}
	appColumns := twdao.YoubanTwoWayBotCooperationApplication.Columns()
	channelColumns := twdao.YoubanTwoWayBotCooperationApplicationChannel.Columns()
	_, _ = twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(appColumns.Id, app.Id).Data(g.Map{appColumns.JoinStatus: sysin.CooperationJoinProcessing, appColumns.UpdatedAt: gtime.Now()}).Update()
	var existing []struct {
		ChannelId int64  `json:"channelId"`
		Status    string `json:"status"`
	}
	if err := twdao.YoubanTwoWayBotCooperationApplicationChannel.Ctx(ctx).Fields(channelColumns.ChannelId, channelColumns.Status).Where(channelColumns.ApplicationId, app.Id).Scan(&existing); err != nil {
		return gerror.Wrap(err, "读取机器人导入频道状态失败")
	}
	statusByChannel := make(map[int64]string, len(existing))
	for _, item := range existing {
		statusByChannel[item.ChannelId] = item.Status
	}
	success, failed := 0, 0
	messages := make([]string, 0)
	for _, channel := range channels {
		if _, ok := statusByChannel[channel.Id]; !ok {
			if _, err := twdao.YoubanTwoWayBotCooperationApplicationChannel.Ctx(ctx).Data(g.Map{
				channelColumns.TenantId:      app.TenantId,
				channelColumns.ApplicationId: app.Id,
				channelColumns.ChannelId:     channel.Id,
				channelColumns.Status:        sysin.CooperationJoinNotStarted,
				channelColumns.CreatedAt:     gtime.Now(),
				channelColumns.UpdatedAt:     gtime.Now(),
			}).Insert(); err != nil {
				return gerror.Wrap(err, "创建新增合作频道记录失败")
			}
		}
		if statusByChannel[channel.Id] == sysin.CooperationJoinSuccess {
			success++
			continue
		}
		joinErr := s.addCooperationBotChannelAdmin(ctx, app.TenantId, channel, app.SubmittedBotUsername)
		status := sysin.CooperationJoinSuccess
		errorMessage := ""
		if joinErr != nil {
			failed++
			status = sysin.CooperationJoinFailed
			errorMessage = cooperationTelegramError(joinErr)
			messages = append(messages, channel.ChannelTitle+"："+errorMessage)
		} else {
			success++
		}
		_, _ = twdao.YoubanTwoWayBotCooperationApplicationChannel.Ctx(ctx).
			Where(channelColumns.ApplicationId, app.Id).
			Where(channelColumns.ChannelId, channel.Id).
			Data(g.Map{channelColumns.Status: status, channelColumns.ErrorMessage: errorMessage, channelColumns.JoinedAt: gtime.Now(), channelColumns.UpdatedAt: gtime.Now()}).Update()
	}
	joinStatus := sysin.CooperationJoinSuccess
	if failed > 0 && success > 0 {
		joinStatus = sysin.CooperationJoinPartialFailed
	} else if failed > 0 {
		joinStatus = sysin.CooperationJoinFailed
	}
	_, err := twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(appColumns.Id, app.Id).Data(g.Map{appColumns.JoinStatus: joinStatus, appColumns.ErrorMessage: strings.Join(messages, "；"), appColumns.UpdatedAt: gtime.Now()}).Update()
	app.JoinStatus = joinStatus
	app.ErrorMessage = strings.Join(messages, "；")
	return err
}

func (s *sSysTwoWayBot) cooperationImportResult(ctx context.Context, app *entity.YoubanTwoWayBotCooperationApplication, status string) *sysin.CooperationImportModel {
	result := &sysin.CooperationImportModel{ApplicationId: appId(app), BotUsername: app.SubmittedBotUsername, BotName: app.SubmittedBotName, Status: status, Message: app.ErrorMessage}
	channels, err := cooperationApplicationChannels(ctx, appId(app))
	if err != nil {
		return result
	}
	result.Total = len(channels)
	for _, channel := range channels {
		channelStatus := channel.Status
		if status == "exists" && channelStatus == sysin.CooperationJoinSuccess {
			channelStatus = "exists"
			result.Exists++
		} else if channelStatus == sysin.CooperationJoinSuccess {
			result.Success++
		} else if channelStatus == sysin.CooperationJoinFailed {
			result.Failed++
		}
		result.Channels = append(result.Channels, &sysin.CooperationImportChannelModel{ChannelId: channel.ChannelId, ChannelTitle: channel.ChannelTitle, Status: channelStatus, ErrorMessage: channel.ErrorMessage})
	}
	return result
}

func appId(app *entity.YoubanTwoWayBotCooperationApplication) int64 {
	if app == nil {
		return 0
	}
	return app.Id
}

func cooperationChannelIds(channels []*cooperationChannelRuntime) []int64 {
	ids := make([]int64, 0, len(channels))
	for _, channel := range channels {
		if channel != nil && channel.Id > 0 {
			ids = append(ids, channel.Id)
		}
	}
	return ids
}
