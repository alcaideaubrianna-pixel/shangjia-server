package sys

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/grand"

	"hotgo/addons/youban_bot/model/input/sysin"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
)

const inviteTable = "hg_youban_bot_invite_code"

const (
	inviteSourceWeb = "web"
	inviteSourceBot = "bot"

	inviteStatusActive  = "active"
	inviteStatusUsed    = "used"
	inviteStatusExpired = "expired"
)

func (s *sSysBot) MyInviteInfo(ctx context.Context) (res *sysin.InviteInfoModel, err error) {
	account, err := s.currentInviteAccount(ctx)
	if err != nil {
		return nil, err
	}
	res = &sysin.InviteInfoModel{
		ExpireDays:     s.inviteExpireDays(ctx),
		BotInviteHint:  "请先在个人中心绑定系统账号后，再使用机器人生成邀请码。",
		WebInviteHint:  "邀请码支持 7 天有效期自动轮换，可在个人中心复制使用。",
		CanGenerateBot: account.AccountType == publishsysin.PublishAccountTypeAdmin,
	}
	if account.AccountType != publishsysin.PublishAccountTypeAdmin {
		res.WebInviteHint = "仅上架端管理员可生成邀请码。"
		return res, nil
	}
	if !s.isInviteBoundAccount(ctx, account.Id) {
		res.CanGenerateBot = false
		return res, nil
	}
	item, err := s.ensureInviteCode(ctx, account, inviteSourceWeb, false)
	if err != nil {
		return nil, err
	}
	res.Code = item.Code
	res.Source = item.Source
	res.ExpiresAt = item.ExpiresAt
	res.InviteUrl = item.InviteUrl
	res.InviteCount, res.UsedCount = s.inviteCountStats(ctx, account.Id)
	res.WebInviteHint = "复制邀请码到上架系统注册页即可使用。"
	return res, nil
}

func (s *sSysBot) MyInviteList(ctx context.Context, in *sysin.InviteListInp) (list []*sysin.InviteModel, totalCount int, err error) {
	account, err := s.currentInviteAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if account.AccountType != publishsysin.PublishAccountTypeAdmin {
		return []*sysin.InviteModel{}, 0, nil
	}
	if in == nil {
		in = &sysin.InviteListInp{}
	}
	_ = s.refreshExpiredInviteCodes(ctx, account.Id)
	perPage := in.PerPage
	if perPage <= 0 && in.PerPageAlias > 0 {
		perPage = in.PerPageAlias
	}
	if perPage <= 0 {
		perPage = 10
	}
	mod := g.DB().Model(inviteTable).Safe().Ctx(ctx).WhereNull("deleted_at").Where("inviter_account_id", account.Id)
	if source := normalizeInviteSource(in.Source); source != "" {
		mod = mod.Where("source", source)
	}
	if status := strings.TrimSpace(in.Status); status != "" {
		mod = mod.Where("status", status)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(code LIKE ? OR inviter_username LIKE ? OR inviter_nickname LIKE ? OR used_username LIKE ? OR registration_telegram_user_id LIKE ? OR registration_telegram_username LIKE ?)", like, like, like, like, like, like)
	}
	var rows []*inviteRow
	if err = mod.Page(in.Page, perPage).OrderDesc("id").ScanAndCount(&rows, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "获取邀请码列表失败")
	}
	if len(rows) == 0 {
		return []*sysin.InviteModel{}, totalCount, nil
	}
	list = make([]*sysin.InviteModel, 0, len(rows))
	for _, row := range rows {
		item := row.toModel()
		list = append(list, item)
	}
	return
}

func (s *sSysBot) CreateInviteCode(ctx context.Context, in *sysin.InviteCreateInp) (res *sysin.InviteCreateModel, err error) {
	account, err := s.currentInviteAccount(ctx)
	if err != nil {
		return nil, err
	}
	if account.AccountType != publishsysin.PublishAccountTypeAdmin {
		return nil, gerror.New("仅上架端管理员可生成邀请码")
	}
	if !s.isInviteBoundAccount(ctx, account.Id) {
		return nil, gerror.New("请先在个人中心绑定系统账号才可以使用")
	}
	source := normalizeInviteSource(in.Source)
	if source == "" {
		source = inviteSourceWeb
	}
	item, err := s.ensureInviteCode(ctx, account, source, in.ForceNew == 1 || source == inviteSourceBot)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (s *sSysBot) ensureInviteCode(ctx context.Context, account *publishsysin.AccountModel, source string, forceNew bool) (*sysin.InviteCreateModel, error) {
	if account == nil || account.Id <= 0 {
		return nil, gerror.New("账号信息不存在")
	}
	source = normalizeInviteSource(source)
	if source == "" {
		source = inviteSourceWeb
	}
	if account.AccountType != publishsysin.PublishAccountTypeAdmin {
		return nil, gerror.New("仅上架端管理员可生成邀请码")
	}
	if !forceNew && source == inviteSourceWeb {
		if row, err := s.latestActiveInvite(ctx, account.Id, source); err == nil && row != nil {
			return s.inviteCreateModel(ctx, row), nil
		}
	}
	expireDays := s.inviteExpireDays(ctx)
	codeLength := s.inviteCodeLength(ctx)
	now := gtime.Now()
	expiresAt := now.Add(time.Duration(expireDays) * 24 * time.Hour)
	code, err := s.uniqueInviteCode(ctx, codeLength)
	if err != nil {
		return nil, err
	}
	if forceNew || source == inviteSourceBot {
		_, _ = g.DB().Model(inviteTable).Safe().Ctx(ctx).
			Where("inviter_app", sysin.BotAppApi).
			Where("inviter_account_id", account.Id).
			Where("source", source).
			Where("status", inviteStatusActive).
			WhereNull("deleted_at").
			Data(g.Map{"status": inviteStatusExpired, "updated_at": now}).Update()
	}
	data := g.Map{
		"code":               code,
		"source":             source,
		"inviter_app":        sysin.BotAppApi,
		"inviter_tenant_id":  account.TenantId,
		"inviter_account_id": account.Id,
		"inviter_username":   account.Username,
		"inviter_nickname":   account.Nickname,
		"status":             inviteStatusActive,
		"expires_at":         expiresAt,
		"created_at":         now,
		"updated_at":         now,
	}
	if _, err = g.DB().Model(inviteTable).Safe().Ctx(ctx).Data(data).Insert(); err != nil {
		return nil, gerror.Wrap(err, "创建邀请码失败")
	}
	return &sysin.InviteCreateModel{
		Code:      code,
		Source:    source,
		ExpiresAt: expiresAt,
		InviteUrl: s.inviteUrl(ctx, code),
	}, nil
}

func (s *sSysBot) uniqueInviteCode(ctx context.Context, length int) (string, error) {
	if length < 6 {
		length = 6
	}
	if length > 16 {
		length = 16
	}
	for i := 0; i < 30; i++ {
		code := strings.ToUpper(grand.S(length))
		count, err := g.DB().Model(inviteTable).Safe().Ctx(ctx).Where("code", code).WhereNull("deleted_at").Count()
		if err != nil {
			return "", gerror.Wrap(err, "生成邀请码失败")
		}
		if count == 0 {
			return code, nil
		}
	}
	return "", gerror.New("生成邀请码失败，请重试")
}

func (s *sSysBot) latestActiveInvite(ctx context.Context, accountId int64, source string) (*inviteRow, error) {
	var row *inviteRow
	mod := g.DB().Model(inviteTable).Safe().Ctx(ctx).
		Where("inviter_app", sysin.BotAppApi).
		Where("inviter_account_id", accountId).
		Where("source", normalizeInviteSource(source)).
		Where("status", inviteStatusActive).
		WhereNull("deleted_at").
		OrderDesc("id")
	if err := mod.Scan(&row); err != nil {
		return nil, gerror.Wrap(err, "读取邀请码失败")
	}
	if row == nil || row.Id <= 0 {
		return nil, nil
	}
	if row.ExpiresAt != nil && row.ExpiresAt.Before(gtime.Now()) {
		_, _ = g.DB().Model(inviteTable).Safe().Ctx(ctx).Where("id", row.Id).Data(g.Map{"status": inviteStatusExpired, "updated_at": gtime.Now()}).Update()
		row.Status = inviteStatusExpired
	}
	return row, nil
}

func (s *sSysBot) inviteCountStats(ctx context.Context, accountId int64) (inviteCount int, usedCount int) {
	mod := g.DB().Model(inviteTable).Safe().Ctx(ctx).Where("inviter_app", sysin.BotAppApi).Where("inviter_account_id", accountId).WhereNull("deleted_at")
	inviteCount, _ = mod.Clone().Count()
	usedCount, _ = mod.Clone().Where("status", inviteStatusUsed).Count()
	return
}

func (s *sSysBot) inviteExpireDays(ctx context.Context) int {
	value := strings.TrimSpace(s.featureConfigValue(ctx, inviteFeature{}.Key(), "expireDays"))
	if value == "" {
		return 7
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 7
	}
	if n > 365 {
		return 365
	}
	return n
}

func (s *sSysBot) inviteCodeLength(ctx context.Context) int {
	value := strings.TrimSpace(s.featureConfigValue(ctx, inviteFeature{}.Key(), "codeLength"))
	if value == "" {
		return 6
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 6
	}
	if n > 16 {
		return 16
	}
	return n
}

func (s *sSysBot) inviteUrl(ctx context.Context, code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	path := "/auth/register?inviteCode=" + url.QueryEscape(code)
	base := strings.TrimRight(strings.TrimSpace(s.featureConfigValue(ctx, inviteFeature{}.Key(), "publishDomain")), "/")
	if base == "" {
		return path
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "https://" + base
	}
	return base + path
}

func (s *sSysBot) isInviteBoundAccount(ctx context.Context, accountId int64) bool {
	if accountId <= 0 {
		return false
	}
	count, err := g.DB().Model(accountBindTbl).Safe().Ctx(ctx).Where("app", sysin.BotAppApi).Where("account_id", accountId).Where("status", 1).WhereNull("deleted_at").Count()
	return err == nil && count > 0
}

func (s *sSysBot) currentInviteAccount(ctx context.Context) (*publishsysin.AccountModel, error) {
	if userId := contexts.GetUserId(ctx); userId > 0 {
		return s.loadInviteAccountByID(ctx, userId)
	}
	telegramUserId := telegramUserIdFromCtx(ctx)
	if telegramUserId != "" {
		bind, err := s.bindingByTelegram(ctx, sysin.BotAppApi, telegramUserId)
		if err != nil {
			return nil, err
		}
		if bind == nil || bind.AccountId <= 0 {
			return nil, gerror.New("请先在个人中心绑定系统账号才可以使用")
		}
		return s.loadInviteAccountByID(ctx, bind.AccountId)
	}
	return nil, gerror.New("请先登录")
}

func (s *sSysBot) loadInviteAccountByID(ctx context.Context, accountId int64) (*publishsysin.AccountModel, error) {
	var account *publishsysin.AccountModel
	if err := g.DB().Model(publishAccountTable).Safe().Ctx(ctx).Where("id", accountId).WhereNull("deleted_at").Scan(&account); err != nil {
		return nil, gerror.Wrap(err, "读取账号失败")
	}
	if account == nil || account.Id <= 0 {
		return nil, gerror.New("账号不存在")
	}
	if account.Status != consts.StatusEnabled {
		return nil, gerror.New("账号已被停用")
	}
	return account, nil
}

type inviteRow struct {
	Id               int64       `json:"id"`
	Code             string      `json:"code"`
	Source           string      `json:"source"`
	InviterApp       string      `json:"inviter_app"`
	InviterTenantId  int64       `json:"inviter_tenant_id"`
	InviterAccountId int64       `json:"inviter_account_id"`
	InviterUsername  string      `json:"inviter_username"`
	InviterNickname  string      `json:"inviter_nickname"`
	UsedTenantId     int64       `json:"used_tenant_id"`
	UsedAccountId    int64       `json:"used_account_id"`
	UsedUsername     string      `json:"used_username"`
	TelegramUserId   string      `json:"registration_telegram_user_id"`
	TelegramUsername string      `json:"registration_telegram_username"`
	Status           string      `json:"status"`
	ExpiresAt        *gtime.Time `json:"expires_at"`
	UsedAt           *gtime.Time `json:"used_at"`
	CreatedAt        *gtime.Time `json:"created_at"`
	UsedTenantName   string      `json:"used_tenant_name"`
}

func (r *inviteRow) toModel() *sysin.InviteModel {
	if r == nil {
		return nil
	}
	return &sysin.InviteModel{
		Id:               r.Id,
		Code:             r.Code,
		Source:           r.Source,
		InviterApp:       r.InviterApp,
		InviterTenantId:  r.InviterTenantId,
		InviterAccountId: r.InviterAccountId,
		InviterUsername:  r.InviterUsername,
		InviterNickname:  r.InviterNickname,
		UsedTenantId:     r.UsedTenantId,
		UsedTenantName:   r.UsedTenantName,
		UsedAccountId:    r.UsedAccountId,
		UsedAccountName:  r.UsedUsername,
		TelegramUserId:   r.TelegramUserId,
		TelegramUsername: r.TelegramUsername,
		Status:           r.Status,
		ExpiresAt:        r.ExpiresAt,
		UsedAt:           r.UsedAt,
		CreatedAt:        r.CreatedAt,
	}
}

func (s *sSysBot) inviteCreateModel(ctx context.Context, r *inviteRow) *sysin.InviteCreateModel {
	if r == nil {
		return nil
	}
	return &sysin.InviteCreateModel{
		Code:      r.Code,
		Source:    r.Source,
		ExpiresAt: r.ExpiresAt,
		InviteUrl: s.inviteUrl(ctx, r.Code),
	}
}

func normalizeInviteSource(source string) string {
	source = strings.TrimSpace(strings.ToLower(source))
	switch source {
	case "", inviteSourceWeb:
		if source == "" {
			return ""
		}
		return inviteSourceWeb
	case inviteSourceBot:
		return inviteSourceBot
	default:
		return source
	}
}

func (s *sSysBot) refreshExpiredInviteCodes(ctx context.Context, accountId int64) error {
	_, err := g.DB().Model(inviteTable).Safe().Ctx(ctx).
		Where("inviter_app", sysin.BotAppApi).
		Where("inviter_account_id", accountId).
		Where("status", inviteStatusActive).
		Where("expires_at IS NOT NULL").
		Where("expires_at < ?", gtime.Now()).
		Data(g.Map{"status": inviteStatusExpired, "updated_at": gtime.Now()}).Update()
	return err
}
