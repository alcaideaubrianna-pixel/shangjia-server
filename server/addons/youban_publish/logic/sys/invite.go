package sys

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/grand"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
)

const webInviteTable = "hg_youban_bot_invite_code"

const (
	webInviteSourceWeb = "web"
	webInviteSourceBot = "bot"

	webInviteStatusActive  = "active"
	webInviteStatusUsed    = "used"
	webInviteStatusExpired = "expired"
)

func (s *sSysPublish) InviteInfo(ctx context.Context) (res *sysin.InviteInfoModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	res = &sysin.InviteInfoModel{
		ExpireDays:     s.webInviteExpireDays(ctx),
		WebInviteHint:  "邀请码支持 7 天有效期自动轮换，可在个人中心复制使用。",
		CanGenerateBot: true,
	}
	item, err := s.ensureWebInviteCode(ctx, account, webInviteSourceWeb)
	if err != nil {
		return nil, err
	}
	res.Code = item.Code
	res.Source = item.Source
	res.ExpiresAt = item.ExpiresAt
	res.InviteUrl = item.InviteUrl
	res.InviteCount, res.UsedCount = s.webInviteCountStats(ctx, account.Id)
	activities, activityCfg, activityErr := s.tenantVipActivities(ctx, account)
	if activityErr != nil {
		return nil, activityErr
	}
	for _, activity := range activities {
		if activity.Code == tenantVipEventInviteBindGift || activity.Code == tenantVipEventInviteFirstPay {
			res.Activities = append(res.Activities, activity)
		}
	}
	res.ActivityBannerTitle = activityCfg.ActivityBannerTitle
	res.ActivityBannerText = activityCfg.ActivityBannerText
	return res, nil
}

func (s *sSysPublish) InviteList(ctx context.Context, in *sysin.InviteListInp) (list []*sysin.InviteModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.InviteListInp{}
	}
	if err = in.Filter(ctx); err != nil {
		return nil, 0, err
	}
	_ = s.refreshWebExpiredInviteCodes(ctx, account.Id)
	mod := s.inviteRelationModel(ctx).
		Where("i.inviter_account_id", account.Id)
	if source := normalizeWebInviteSource(in.Source); source != "" {
		mod = mod.Where("i.source", source)
	}
	if status := strings.TrimSpace(in.Status); status != "" {
		mod = filterInviteRelationStatus(mod, status)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(i.code LIKE ? OR i.inviter_username LIKE ? OR i.inviter_nickname LIKE ? OR COALESCE(u.used_username,i.used_username) LIKE ? OR ua.username LIKE ? OR ua.nickname LIKE ? OR ut.name LIKE ?)", like, like, like, like, like, like, like)
	}
	var rows []*webInviteRow
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("COALESCE(u.used_at,i.created_at)").OrderDesc("i.id").ScanAndCount(&rows, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "获取邀请码列表失败")
	}
	list = make([]*sysin.InviteModel, 0, len(rows))
	for _, row := range rows {
		list = append(list, row.toModel())
	}
	if err = s.enrichInviteActivityStatus(ctx, account.TenantId, list); err != nil {
		return nil, 0, err
	}
	return
}

func (s *sSysPublish) enrichInviteActivityStatus(ctx context.Context, inviterTenantId int64, list []*sysin.InviteModel) error {
	if len(list) == 0 {
		return nil
	}
	accountIds := make([]int64, 0, len(list))
	tenantIds := make([]int64, 0, len(list))
	for _, item := range list {
		if item.UsedAccountId > 0 {
			accountIds = append(accountIds, item.UsedAccountId)
		}
		if item.UsedTenantId > 0 {
			tenantIds = append(tenantIds, item.UsedTenantId)
		}
	}
	type bindRow struct {
		AccountId int64       `json:"account_id"`
		BoundAt   *gtime.Time `json:"bound_at"`
	}
	binds := make(map[int64]*gtime.Time)
	if len(accountIds) > 0 {
		var rows []*bindRow
		if err := g.DB().Model("hg_youban_bot_account_bind").Safe().Ctx(ctx).
			Fields("account_id,MIN(created_at) AS bound_at").
			Where("app", consts.AppApi).
			WhereIn("account_id", accountIds).
			Where("status", consts.StatusEnabled).
			WhereNull("deleted_at").
			Group("account_id").
			Scan(&rows); err != nil {
			return gerror.Wrap(err, "读取邀请用户TG绑定状态失败")
		}
		for _, row := range rows {
			binds[row.AccountId] = row.BoundAt
		}
	}
	type paidRow struct {
		TenantId int64       `json:"tenant_id"`
		PaidAt   *gtime.Time `json:"paid_at"`
	}
	paid := make(map[int64]*gtime.Time)
	if len(tenantIds) > 0 {
		orderColumns := dao.AdminOrder.Columns()
		var rows []*paidRow
		if err := dao.AdminOrder.Ctx(ctx).
			Fields(orderColumns.ProductId+" AS tenant_id,MIN("+orderColumns.UpdatedAt+") AS paid_at").
			Where(orderColumns.OrderType, tenantVipOrderType).
			Where(orderColumns.Status, consts.OrderStatusPay).
			WhereGT(orderColumns.Money, 0).
			WhereIn(orderColumns.ProductId, tenantIds).
			Group(orderColumns.ProductId).
			Scan(&rows); err != nil {
			return gerror.Wrap(err, "读取邀请用户首付状态失败")
		}
		for _, row := range rows {
			paid[row.TenantId] = row.PaidAt
		}
	}
	type rewardRow struct {
		EventType       string `json:"event_type"`
		TriggerTenantId int64  `json:"trigger_tenant_id"`
		Days            int    `json:"days"`
	}
	rewards := make(map[string]int)
	if len(tenantIds) > 0 {
		var rows []*rewardRow
		if err := g.DB().Model(tenantVipEventTable).Safe().Ctx(ctx).
			Fields("event_type,trigger_tenant_id,COALESCE(SUM(change_days),0) AS days").
			Where("tenant_id", inviterTenantId).
			WhereIn("trigger_tenant_id", tenantIds).
			WhereIn("event_type", []string{tenantVipEventInviteBindGift, tenantVipEventInviteFirstPay}).
			Group("event_type,trigger_tenant_id").
			Scan(&rows); err != nil {
			return gerror.Wrap(err, "读取邀请奖励状态失败")
		}
		for _, row := range rows {
			rewards[fmt.Sprintf("%s:%d", row.EventType, row.TriggerTenantId)] = row.Days
		}
	}
	for _, item := range list {
		item.TelegramBoundAt = binds[item.UsedAccountId]
		item.FirstPaidAt = paid[item.UsedTenantId]
		item.BindRewardDays = rewards[fmt.Sprintf("%s:%d", tenantVipEventInviteBindGift, item.UsedTenantId)]
		item.FirstPayRewardDays = rewards[fmt.Sprintf("%s:%d", tenantVipEventInviteFirstPay, item.UsedTenantId)]
		item.RewardDaysTotal = item.BindRewardDays + item.FirstPayRewardDays
	}
	return nil
}

func (s *sSysPublish) AdminInviteList(ctx context.Context, in *sysin.InviteListInp) (list []*sysin.InviteModel, totalCount int, err error) {
	if _, err = s.currentAdminAccount(ctx); err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.InviteListInp{}
	}
	if err = in.Filter(ctx); err != nil {
		return nil, 0, err
	}
	_ = s.refreshWebExpiredInviteCodesByTenant(ctx, 0)
	mod := s.inviteRelationModel(ctx)
	if source := normalizeWebInviteSource(in.Source); source != "" {
		mod = mod.Where("i.source", source)
	}
	if status := strings.TrimSpace(in.Status); status != "" {
		mod = filterInviteRelationStatus(mod, status)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(i.code LIKE ? OR i.inviter_username LIKE ? OR i.inviter_nickname LIKE ? OR COALESCE(u.used_username,i.used_username) LIKE ? OR i.registration_telegram_user_id LIKE ? OR i.registration_telegram_username LIKE ? OR ua.username LIKE ? OR ua.nickname LIKE ? OR it.name LIKE ? OR ut.name LIKE ?)", like, like, like, like, like, like, like, like, like, like)
	}
	var rows []*webInviteRow
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("COALESCE(u.used_at,i.created_at)").OrderDesc("i.id").ScanAndCount(&rows, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "获取邀请关系列表失败")
	}
	list = make([]*sysin.InviteModel, 0, len(rows))
	for _, row := range rows {
		list = append(list, row.toModel())
	}
	return
}

func (s *sSysPublish) CreateInviteCode(ctx context.Context, in *sysin.InviteCreateInp) (res *sysin.InviteCreateModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		in = &sysin.InviteCreateInp{}
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	return s.ensureWebInviteCode(ctx, account, in.Source)
}

func (s *sSysPublish) ensureWebInviteCode(ctx context.Context, account *sysin.AccountModel, source string) (*sysin.InviteCreateModel, error) {
	if account == nil || account.Id <= 0 {
		return nil, gerror.New("账号信息不存在")
	}
	if account.AccountType != sysin.PublishAccountTypeAdmin {
		return nil, gerror.New("仅上架端管理员可生成邀请码")
	}
	source = normalizeWebInviteSource(source)
	if source == "" {
		source = webInviteSourceWeb
	}
	if row, err := s.latestWebActiveInvite(ctx, account.Id, source); err != nil {
		return nil, err
	} else if row != nil {
		return s.webInviteCreateModel(ctx, row), nil
	}
	expireDays := s.webInviteExpireDays(ctx)
	now := gtime.Now()
	expiresAt := now.Add(time.Duration(expireDays) * 24 * time.Hour)
	code, err := s.webUniqueInviteCode(ctx, s.webInviteCodeLength(ctx))
	if err != nil {
		return nil, err
	}
	data := g.Map{
		"code":               code,
		"source":             source,
		"inviter_app":        "api",
		"inviter_tenant_id":  account.TenantId,
		"inviter_account_id": account.Id,
		"inviter_username":   account.Username,
		"inviter_nickname":   account.Nickname,
		"status":             webInviteStatusActive,
		"expires_at":         expiresAt,
		"created_at":         now,
		"updated_at":         now,
	}
	if _, err = g.DB().Model(webInviteTable).Safe().Ctx(ctx).Data(data).Insert(); err != nil {
		return nil, gerror.Wrap(err, "创建邀请码失败")
	}
	return &sysin.InviteCreateModel{
		Code:      code,
		Source:    source,
		ExpiresAt: expiresAt,
		InviteUrl: s.webInviteUrl(ctx, code),
	}, nil
}

func (s *sSysPublish) webInviteExpireDays(ctx context.Context) int {
	_ = ctx
	return 7
}

func (s *sSysPublish) webInviteCodeLength(ctx context.Context) int {
	_ = ctx
	return 6
}

func (s *sSysPublish) webInviteUrl(ctx context.Context, code string) string {
	_ = ctx
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	return "/auth/register?inviteCode=" + url.QueryEscape(code)
}

func (s *sSysPublish) webUniqueInviteCode(ctx context.Context, length int) (string, error) {
	if length < 6 {
		length = 6
	}
	if length > 16 {
		length = 16
	}
	for i := 0; i < 30; i++ {
		code := strings.ToUpper(grand.S(length))
		count, err := g.DB().Model(webInviteTable).Safe().Ctx(ctx).Where("code", code).WhereNull("deleted_at").Count()
		if err != nil {
			return "", gerror.Wrap(err, "生成邀请码失败")
		}
		if count == 0 {
			return code, nil
		}
	}
	return "", gerror.New("生成邀请码失败，请重试")
}

func (s *sSysPublish) latestWebActiveInvite(ctx context.Context, accountId int64, source string) (*webInviteRow, error) {
	var row *webInviteRow
	mod := g.DB().Model(webInviteTable).Safe().Ctx(ctx).
		Where("inviter_app", "api").
		Where("inviter_account_id", accountId).
		Where("source", normalizeWebInviteSource(source)).
		Where("status", webInviteStatusActive).
		WhereNull("deleted_at").
		OrderDesc("id")
	if err := mod.Scan(&row); err != nil {
		return nil, gerror.Wrap(err, "读取邀请码失败")
	}
	if row == nil || row.Id <= 0 {
		return nil, nil
	}
	if row.ExpiresAt != nil && row.ExpiresAt.Before(gtime.Now()) {
		_, _ = g.DB().Model(webInviteTable).Safe().Ctx(ctx).Where("id", row.Id).Data(g.Map{"status": webInviteStatusExpired, "updated_at": gtime.Now()}).Update()
		return nil, nil
	}
	return row, nil
}

func (s *sSysPublish) webInviteCountStats(ctx context.Context, accountId int64) (inviteCount int, usedCount int) {
	mod := g.DB().Model(webInviteTable).Safe().Ctx(ctx).Where("inviter_app", "api").Where("inviter_account_id", accountId).WhereNull("deleted_at")
	inviteCount, _ = mod.Clone().Count()
	usedCount, _ = g.DB().Model(botInviteUsageTable).Safe().Ctx(ctx).Where("inviter_app", "api").Where("inviter_account_id", accountId).WhereNull("deleted_at").Count()
	return
}

func (s *sSysPublish) inviteRelationModel(ctx context.Context) *gdb.Model {
	return g.DB().Model(webInviteTable+" i").Safe().Ctx(ctx).
		LeftJoin(botInviteUsageTable+" u", "u.invite_id=i.id AND u.deleted_at IS NULL").
		LeftJoin(publishTenantTable+" it", "it.id=i.inviter_tenant_id AND it.deleted_at IS NULL").
		LeftJoin(publishTenantTable+" ut", "ut.id=COALESCE(u.used_tenant_id,i.used_tenant_id) AND ut.deleted_at IS NULL").
		LeftJoin(publishAccountTable+" ua", "ua.id=COALESCE(u.used_account_id,i.used_account_id) AND ua.deleted_at IS NULL").
		Fields(inviteRelationFields()).
		WhereNull("i.deleted_at")
}

func inviteRelationFields() string {
	return "COALESCE(u.id,i.id) AS id,i.code,i.source,i.inviter_app,i.inviter_tenant_id,it.name AS inviter_tenant_name," +
		"i.inviter_account_id,i.inviter_username,i.inviter_nickname," +
		"COALESCE(u.used_tenant_id,i.used_tenant_id) AS used_tenant_id,ut.name AS used_tenant_name," +
		"COALESCE(u.used_account_id,i.used_account_id) AS used_account_id,COALESCE(u.used_username,i.used_username) AS used_username," +
		"ua.username AS used_account_username,ua.nickname AS used_account_nickname," +
		"i.registration_telegram_user_id,i.registration_telegram_username," +
		"CASE WHEN u.id IS NOT NULL THEN 'used' ELSE i.status END AS status,i.expires_at," +
		"COALESCE(u.used_at,i.used_at) AS used_at,i.created_at"
}

func filterInviteRelationStatus(model *gdb.Model, status string) *gdb.Model {
	switch status {
	case webInviteStatusUsed:
		return model.Where("(u.id IS NOT NULL OR i.status=?)", webInviteStatusUsed)
	case webInviteStatusActive, webInviteStatusExpired:
		return model.Where("u.id IS NULL").Where("i.status", status)
	default:
		return model
	}
}

func (s *sSysPublish) refreshWebExpiredInviteCodes(ctx context.Context, accountId int64) error {
	_, err := g.DB().Model(webInviteTable).Safe().Ctx(ctx).
		Where("inviter_app", "api").
		Where("inviter_account_id", accountId).
		Where("status", webInviteStatusActive).
		Where("expires_at IS NOT NULL").
		Where("expires_at < ?", gtime.Now()).
		Data(g.Map{"status": webInviteStatusExpired, "updated_at": gtime.Now()}).Update()
	return err
}

func (s *sSysPublish) refreshWebExpiredInviteCodesByTenant(ctx context.Context, tenantId int64) error {
	mod := g.DB().Model(webInviteTable).Safe().Ctx(ctx).
		Where("inviter_app", "api").
		Where("status", webInviteStatusActive).
		Where("expires_at IS NOT NULL").
		Where("expires_at < ?", gtime.Now())
	if tenantId > 0 {
		mod = mod.Where("inviter_tenant_id", tenantId)
	}
	_, err := mod.Data(g.Map{"status": webInviteStatusExpired, "updated_at": gtime.Now()}).Update()
	return err
}

func normalizeWebInviteSource(source string) string {
	source = strings.TrimSpace(strings.ToLower(source))
	switch source {
	case "", webInviteSourceWeb:
		if source == "" {
			return ""
		}
		return webInviteSourceWeb
	case webInviteSourceBot:
		return webInviteSourceBot
	default:
		return source
	}
}

type webInviteRow struct {
	Id                  int64       `json:"id"`
	Code                string      `json:"code"`
	Source              string      `json:"source"`
	InviterApp          string      `json:"inviter_app"`
	InviterTenantId     int64       `json:"inviter_tenant_id"`
	InviterTenantName   string      `json:"inviter_tenant_name"`
	InviterAccountId    int64       `json:"inviter_account_id"`
	InviterUsername     string      `json:"inviter_username"`
	InviterNickname     string      `json:"inviter_nickname"`
	UsedTenantId        int64       `json:"used_tenant_id"`
	UsedAccountId       int64       `json:"used_account_id"`
	UsedUsername        string      `json:"used_username"`
	UsedAccountUsername string      `json:"used_account_username"`
	UsedAccountNickname string      `json:"used_account_nickname"`
	TelegramUserId      string      `json:"registration_telegram_user_id"`
	TelegramUsername    string      `json:"registration_telegram_username"`
	Status              string      `json:"status"`
	ExpiresAt           *gtime.Time `json:"expires_at"`
	UsedAt              *gtime.Time `json:"used_at"`
	CreatedAt           *gtime.Time `json:"created_at"`
	UsedTenantName      string      `json:"used_tenant_name"`
}

func (r *webInviteRow) toModel() *sysin.InviteModel {
	if r == nil {
		return nil
	}
	return &sysin.InviteModel{
		Id:                r.Id,
		Code:              r.Code,
		Source:            r.Source,
		InviterApp:        r.InviterApp,
		InviterTenantId:   r.InviterTenantId,
		InviterTenantName: r.InviterTenantName,
		InviterAccountId:  r.InviterAccountId,
		InviterUsername:   r.InviterUsername,
		InviterNickname:   r.InviterNickname,
		UsedTenantId:      r.UsedTenantId,
		UsedTenantName:    r.UsedTenantName,
		UsedAccountId:     r.UsedAccountId,
		UsedAccountName:   firstNonEmpty(r.UsedUsername, r.UsedAccountUsername, r.UsedAccountNickname),
		TelegramUserId:    r.TelegramUserId,
		TelegramUsername:  r.TelegramUsername,
		Status:            r.Status,
		ExpiresAt:         r.ExpiresAt,
		UsedAt:            r.UsedAt,
		CreatedAt:         r.CreatedAt,
	}
}

func (s *sSysPublish) webInviteCreateModel(ctx context.Context, r *webInviteRow) *sysin.InviteCreateModel {
	if r == nil {
		return nil
	}
	return &sysin.InviteCreateModel{
		Code:      r.Code,
		Source:    r.Source,
		ExpiresAt: r.ExpiresAt,
		InviteUrl: s.webInviteUrl(ctx, r.Code),
	}
}
