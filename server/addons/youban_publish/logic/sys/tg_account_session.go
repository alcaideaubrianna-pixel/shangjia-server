package sys

import (
	"context"
	"errors"
	"fmt"
	"html"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
)

func (s *sSysPublish) refreshAdminTgAccountSession(ctx context.Context, id int64, tenantId int64, operatorId int64) (status string, message string) {
	if _, err := s.adminTgAccountById(ctx, id, tenantId); err != nil {
		return sysin.PublishTgAccountStatusFailed, err.Error()
	}
	runtime := collectorservice.AccountRuntime()
	if runtime == nil {
		return refreshTgAccountViaTask(ctx, id, tenantId, operatorId, collectorservice.AccountTasks().SubmitAndWait)
	}

	// Refresh is a user-triggered health probe. Execute it through the
	// priority lane so a backlog of media tasks cannot make the result stale.
	var user *tg.User
	used, err := runtime.ExecutePriority(ctx, id, 20*time.Second, func(probeCtx context.Context, client *telegram.Client) error {
		if client == nil {
			return gerror.New("Telegram客户端未就绪")
		}
		var probeErr error
		user, probeErr = client.Self(probeCtx)
		return probeErr
	})
	if !used {
		return refreshTgAccountViaTask(ctx, id, tenantId, operatorId, collectorservice.AccountTasks().SubmitAndWait)
	}
	if err != nil {
		if isTelegramPermanentAccountAuthError(err) {
			return s.expireTgAccountSession(ctx, id, tenantId, operatorId, telegramPermanentAccountAuthMessage(err))
		}
		return sysin.PublishTgAccountStatusFailed, err.Error()
	}

	username, displayName := "", ""
	if user != nil {
		username = user.Username
		displayName = strings.TrimSpace(user.FirstName + " " + user.LastName)
	}
	s.updateTgAccountRefreshResult(ctx, id, tenantId, operatorId, sysin.PublishTgAccountStatusAuthorized, "", user, username, displayName)
	return sysin.PublishTgAccountStatusAuthorized, "Telegram账号连接正常"
}

// Web replicas do not own Telegram connections. Route their probes to the
// account owner through the existing durable task executor.
func refreshTgAccountViaTask(ctx context.Context, id, tenantId, operatorId int64, submit func(context.Context, *collectorin.AccountTaskSubmit, time.Duration) (*collectorin.AccountTask, error)) (string, string) {
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := probeCtx.Err(); err != nil {
		return sysin.PublishTgAccountStatusFailed, "TG账号状态刷新已取消，请稍后重试"
	}
	task, err := submit(probeCtx, &collectorin.AccountTaskSubmit{
		TenantID: tenantId, AccountID: id, TaskType: collectorin.AccountTaskTypeTgAccountRefresh,
		// The suffix prevents an earlier completed probe from being reused.
		TaskKey:  fmt.Sprintf("tg-account-refresh:%d:%d:%d:%s", id, tenantId, operatorId, guid.S()),
		Priority: tgJobPriorityUrgent, MaxAttempts: 1,
	}, 250*time.Millisecond)
	if err != nil {
		if probeCtx.Err() != nil {
			return sysin.PublishTgAccountStatusFailed, "TG账号状态刷新等待超时，请稍后重试"
		}
		return sysin.PublishTgAccountStatusFailed, err.Error()
	}
	if task == nil {
		return sysin.PublishTgAccountStatusFailed, "TG账号状态刷新未返回结果"
	}
	if task.Status != collectorin.AccountTaskStatusCompleted {
		if isTelegramPermanentAccountAuthError(gerror.New(task.ErrorMessage)) {
			return sysin.PublishTgAccountStatusExpired, telegramPermanentAccountAuthMessage(gerror.New(task.ErrorMessage))
		}
		return sysin.PublishTgAccountStatusFailed, firstNonEmpty(task.ErrorMessage, "TG账号状态刷新失败，请稍后重试")
	}
	return sysin.PublishTgAccountStatusAuthorized, "Telegram账号连接正常"
}

func (s *sSysPublish) handleTgAccountRefreshTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) error {
	var id, tenantId, operatorId int64
	if task == nil || task.TaskKey == "" {
		return gerror.New("TG账号刷新任务参数无效")
	}
	if _, err := fmt.Sscanf(task.TaskKey, "tg-account-refresh:%d:%d:%d", &id, &tenantId, &operatorId); err != nil || id <= 0 || tenantId <= 0 {
		return gerror.New("TG账号刷新任务参数无效")
	}
	if id != task.AccountID || tenantId != task.TenantID {
		return gerror.New("TG账号刷新任务归属不匹配")
	}
	if client == nil {
		return gerror.New("Telegram客户端未就绪")
	}
	user, err := client.Self(ctx)
	if err != nil {
		if isTelegramPermanentAccountAuthError(err) {
			s.expireTgAccountSession(context.Background(), id, tenantId, operatorId, telegramPermanentAccountAuthMessage(err))
		} else {
			s.failTgAccountRefresh(context.Background(), id, tenantId, operatorId, err.Error())
		}
		return err
	}
	username, displayName := "", ""
	if user != nil {
		username = user.Username
		displayName = strings.TrimSpace(user.FirstName + " " + user.LastName)
	}
	s.updateTgAccountRefreshResult(ctx, id, tenantId, operatorId, sysin.PublishTgAccountStatusAuthorized, "", user, username, displayName)
	return nil
}

func (s *sSysPublish) failTgAccountRefresh(ctx context.Context, id int64, tenantId int64, operatorId int64, message string) (string, string) {
	s.updateTgAccountRefreshResult(ctx, id, tenantId, operatorId, sysin.PublishTgAccountStatusFailed, message, nil, "", "")
	return sysin.PublishTgAccountStatusFailed, message
}

const tgAccountSessionExpiredMessage = "TG账号登录态已失效，请重新扫码登录"

var errTelegramPublishAccountMissing = errors.New("推送账号配置缺失")

// telegramAccountCanPublish is the single gate used before a Telegram publish
// task enters Asynq. Bot delivery does not require a protocol session; the
// account fallback validates its own account when it is actually used.
func (s *sSysPublish) telegramJobAccountCanPublish(ctx context.Context, job telegramJobRecord) (int64, bool, error) {
	return checkTelegramJobDeliveryAvailability(ctx, g.DB(), job)
}

func checkTelegramJobDeliveryAvailability(ctx context.Context, db gdb.DB, job telegramJobRecord) (int64, bool, error) {
	if job.TenantId <= 0 || job.ChannelId <= 0 {
		return 0, false, nil
	}
	botAvailable, err := db.Model(publishTgJobTable+" availability_job").Safe().Ctx(ctx).
		Where("availability_job.id", job.Id).
		Where(telegramJobAvailableBotSQL("availability_job")).Exist()
	if err != nil {
		return 0, false, gerror.Wrap(err, "检查Bot发布状态失败")
	}
	if botAvailable {
		return 0, true, nil
	}
	channelTable, hasSoftDelete := telegramJobChannelTable(job.OperationNo)
	channelModel := db.Model(channelTable).Safe().Ctx(ctx).
		Fields("tg_account_id").Where("id", job.ChannelId).Where("tenant_id", job.TenantId)
	if hasSoftDelete {
		channelModel = channelModel.WhereNull("deleted_at")
	}
	channel, err := channelModel.One()
	if err != nil {
		return 0, false, gerror.Wrap(err, "读取TG任务发送账号失败")
	}
	if channel.IsEmpty() {
		return 0, false, gerror.Wrapf(errTelegramPublishAccountMissing, "推送频道不存在或已删除，channel:%d，请重新选择上架频道", job.ChannelId)
	}
	tgAccountId := channel["tg_account_id"].Int64()
	if tgAccountId <= 0 {
		return 0, false, gerror.Wrapf(errTelegramPublishAccountMissing, "频道未配置可用Bot或协议号，channel:%d", job.ChannelId)
	}
	available, err := checkTelegramProtocolAccount(ctx, db, job.TenantId, tgAccountId)
	return tgAccountId, available, err
}

func telegramProtocolAccountCanPublish(ctx context.Context, tenantID, tgAccountId int64) (bool, error) {
	return checkTelegramProtocolAccount(ctx, g.DB(), tenantID, tgAccountId)
}

func checkTelegramProtocolAccount(ctx context.Context, db gdb.DB, tenantID, tgAccountId int64) (bool, error) {
	account, err := db.Model(publishTgAccountTable).Safe().Ctx(ctx).
		Fields("status,session_key").
		Where("id", tgAccountId).
		Where("tenant_id", tenantID).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return false, gerror.Wrap(err, "检查TG账号发布状态失败")
	}
	if account.IsEmpty() {
		return false, gerror.Wrapf(errTelegramPublishAccountMissing, "TG账号不存在或已删除，account:%d，请更换频道绑定账号或配置可用Bot", tgAccountId)
	}
	return account["status"].String() == sysin.PublishTgAccountStatusAuthorized && strings.TrimSpace(account["session_key"].String()) != "", nil
}

// Only untouched jobs can be failed here. Ambiguous or partly delivered sends
// retain their reconciliation/cleanup ownership instead of being resent.
func (s *sSysPublish) failTelegramMissingAccountJob(ctx context.Context, job telegramJobRecord, cause error) error {
	if job.SendPhase != "" || (job.Status != telegramJobStatusPending && job.Status != telegramJobStatusRetryWait) {
		return cause
	}
	changed, err := s.transitionTelegramJob(ctx, telegramJobTransition{
		JobId: job.Id, From: []string{telegramJobStatusPending, telegramJobStatusRetryWait},
		To: telegramJobStatusFailed, ErrorMessage: cause.Error(),
		SendPhases: []string{""},
		Additional: g.Map{"last_dispatch_error": cause.Error()},
	})
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	s.appendTelegramJobLog(ctx, job, "dispatch", "failed", cause.Error())
	if err = s.upsertPublishJobRecord(ctx, job, "failed", cause.Error()); err != nil {
		return err
	}
	if err = s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed); err != nil {
		return err
	}
	// Let the dispatcher select the next head on its next tick. Synchronously
	// waking another broken job here could recurse through a large backlog.
	return nil
}

func telegramJobChannelTable(operationNo string) (table string, hasSoftDelete bool) {
	if isMessagePushOperationNo(operationNo) {
		return publishTgChannelTable, false
	}
	return publishChannelTable, true
}

func (s *sSysPublish) pauseTelegramJobsForUnavailableAccount(ctx context.Context, tgAccountId int64, reason string) error {
	if tgAccountId <= 0 {
		return nil
	}
	accountChannelCondition := `(
		(operation_no NOT LIKE 'message_push:%' AND operation_no NOT LIKE 'message_push_plan:%' AND EXISTS (
			SELECT 1 FROM ` + publishChannelTable + ` c WHERE c.id=` + publishTgJobTable + `.channel_id AND c.tenant_id=` + publishTgJobTable + `.tenant_id AND c.tg_account_id=? AND c.deleted_at IS NULL
		)) OR
		((operation_no LIKE 'message_push:%' OR operation_no LIKE 'message_push_plan:%') AND EXISTS (
			SELECT 1 FROM ` + publishTgChannelTable + ` c WHERE c.id=` + publishTgJobTable + `.channel_id AND c.tenant_id=` + publishTgJobTable + `.tenant_id AND c.tg_account_id=?
		))
	)`
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where(accountChannelCondition, tgAccountId, tgAccountId).
		WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
		WhereIn("dispatch_status", []string{"", tgDispatchStatusIdle, tgDispatchStatusQueued, tgDispatchStatusProcessing}).
		Data(g.Map{
			"dispatch_status":     tgDispatchStatusIdle,
			"next_retry_at":       gtime.Now().Add(15 * time.Minute),
			"last_dispatch_error": reason,
			"updated_at":          gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "暂停失效TG账号待发送任务失败")
	}
	g.Log().Warningf(ctx, "TG账号不可用，已暂停待发送任务 tgAccountId:%d reason:%s", tgAccountId, reason)
	return nil
}

type tgAccountAuthOwner struct {
	Id               int64  `json:"id"`
	TenantId         int64  `json:"tenant_id"`
	AccountId        int64  `json:"account_id"`
	DisplayName      string `json:"display_name"`
	TelegramUsername string `json:"telegram_username"`
	Status           string `json:"status"`
}

func (s *sSysPublish) expireTgAccountSession(ctx context.Context, id int64, tenantId int64, operatorId int64, message string) (string, string) {
	s.updateTgAccountRefreshResult(ctx, id, tenantId, operatorId, sysin.PublishTgAccountStatusExpired, message, nil, "", "")
	if err := s.pauseTelegramJobsForUnavailableAccount(ctx, id, message); err != nil {
		g.Log().Warningf(ctx, "暂停失效TG账号任务失败 tgAccountId:%d err:%+v", id, err)
	}
	s.cancelTelegramDeleteFallbackTasks(ctx, []int64{id}, message)
	return sysin.PublishTgAccountStatusExpired, message
}

func isTelegramAuthKeyUnregistered(err error) bool {
	return err != nil && strings.Contains(err.Error(), "AUTH_KEY_UNREGISTERED")
}

func isTelegramPermanentAccountAuthError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToUpper(err.Error())
	permanentParts := []string{
		"AUTH_KEY_DUPLICATED",
		"AUTH_KEY_UNREGISTERED",
		"SESSION_REVOKED",
		"USER_DEACTIVATED",
		"USER_DEACTIVATED_BAN",
	}
	for _, part := range permanentParts {
		if strings.Contains(message, part) {
			return true
		}
	}
	return false
}

func telegramPermanentAccountAuthMessage(err error) string {
	if err == nil {
		return tgAccountSessionExpiredMessage
	}
	message := strings.ToUpper(err.Error())
	switch {
	case strings.Contains(message, "AUTH_KEY_DUPLICATED"):
		return "TG账号授权密钥被重复使用，Telegram 已作废该登录态，请重新扫码登录"
	case strings.Contains(message, "USER_DEACTIVATED_BAN"):
		return "TG账号已被 Telegram 封禁，已自动停用，请更换账号"
	case strings.Contains(message, "USER_DEACTIVATED"):
		return "TG账号已被 Telegram 停用或注销，已自动停用，请更换账号"
	case strings.Contains(message, "SESSION_REVOKED"):
		return "TG账号会话已被撤销，已自动停用，请重新扫码登录"
	case strings.Contains(message, "AUTH_KEY_UNREGISTERED"):
		return tgAccountSessionExpiredMessage
	default:
		return tgAccountSessionExpiredMessage
	}
}

func (s *sSysPublish) handleTgAccountPermanentAuthError(ctx context.Context, tgAccountId int64, operatorId int64, message string, cause error) {
	account, err := s.notifyTgAccountOwner(ctx, tgAccountId, 0)
	if err != nil {
		g.Log().Warningf(ctx, "读取失效TG账号失败 tgAccountId:%d err:%+v", tgAccountId, err)
		return
	}
	if account.Id <= 0 {
		return
	}
	if account.Status != sysin.PublishTgAccountStatusAuthorized {
		return
	}
	if strings.TrimSpace(message) == "" {
		message = telegramPermanentAccountAuthMessage(cause)
	}
	s.expireTgAccountSession(ctx, account.Id, account.TenantId, operatorId, message)
	if account.AccountId <= 0 {
		return
	}
	text := fmt.Sprintf("TG账号已自动停用。\n\nTG账号：%s\n原因：%s", html.EscapeString(firstNonEmpty(account.DisplayName, account.TelegramUsername, fmt.Sprintf("ID:%d", account.Id))), html.EscapeString(message))
	if notifyErr := s.notifyAccountWithPreference(ctx, account.TenantId, "account", &botsysin.NotifyAccountInp{
		App:       consts.AppApi,
		AccountId: account.AccountId,
		Text:      text,
		ParseMode: "HTML",
	}); notifyErr != nil {
		g.Log().Warningf(ctx, "发送TG账号自动停用通知失败 tgAccountId:%d accountId:%d err:%+v", account.Id, account.AccountId, notifyErr)
	}
}

func (s *sSysPublish) notifyTgAccountOwner(ctx context.Context, tgAccountId int64, tenantId int64) (*tgAccountAuthOwner, error) {
	if tgAccountId <= 0 {
		return &tgAccountAuthOwner{}, nil
	}
	var account *tgAccountAuthOwner
	mod := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,account_id,display_name,telegram_username,status").
		Where("id", tgAccountId).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if err := mod.Scan(&account); err != nil {
		return nil, gerror.Wrap(err, "读取TG账号失败")
	}
	if account == nil {
		return &tgAccountAuthOwner{}, nil
	}
	return account, nil
}

func (s *sSysPublish) updateTgAccountRefreshResult(ctx context.Context, id int64, tenantId int64, operatorId int64, status string, message string, user *tg.User, username string, displayName string) {
	data := g.Map{
		"status":        status,
		"error_message": message,
		"updated_by":    operatorId,
		"updated_at":    gtime.Now(),
	}
	if status == sysin.PublishTgAccountStatusAuthorized {
		data["last_login_at"] = gtime.Now()
		if user != nil {
			data["telegram_user_id"] = strconv.FormatInt(user.ID, 10)
			data["telegram_username"] = username
			data["telegram_first_name"] = user.FirstName
			data["telegram_last_name"] = user.LastName
			data["telegram_phone"] = user.Phone
			data["telegram_is_bot"] = boolToInt(user.Bot)
			if displayName != "" {
				data["display_name"] = displayName
			}
		}
	}
	_, _ = g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Where("id", id).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Data(data).
		Update()
}

func (s *sSysPublish) telegramSessionPathByKey(sessionKey string) (string, error) {
	parts := strings.Split(strings.TrimSpace(sessionKey), "/")
	if len(parts) != 3 || !strings.HasPrefix(parts[0], "tenant_") || !strings.HasPrefix(parts[1], "account_") {
		return "", gerror.New("TG账号会话路径无效，请重新登录")
	}
	tenantPart := strings.TrimPrefix(parts[0], "tenant_")
	accountPart := strings.TrimPrefix(parts[1], "account_")
	token := strings.TrimSuffix(parts[2], ".json")
	if tenantPart == "" || accountPart == "" || token == "" {
		return "", gerror.New("TG账号会话路径无效，请重新登录")
	}
	return filepath.Join(gfile.Pwd(), "runtime", "youban_publish", "telegram_sessions", "tenant_"+tenantPart, fmt.Sprintf("account_%s_%s.json", accountPart, token)), nil
}

func (s *sSysPublish) ensureTgAccountsBelongTenant(ctx context.Context, ids []int64, tenantId int64) error {
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return gerror.New("请选择TG账号")
	}
	count, err := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查TG账号权限失败")
	}
	if count != len(ids) {
		return gerror.New("存在无权操作的TG账号")
	}
	return nil
}

func (s *sSysPublish) resolveTenantTgAccountId(ctx context.Context, id int64, tenantId int64) (int64, error) {
	if id <= 0 || tenantId <= 0 {
		return 0, gerror.New("请选择TG账号")
	}
	account, err := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).Unscoped().
		Where("id", id).
		Where("tenant_id", tenantId).
		One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取TG账号失败")
	}
	if account.IsEmpty() {
		return 0, gerror.New("TG账号不存在或不属于当前租户")
	}
	if account["deleted_at"].IsNil() && account["status"].String() == sysin.PublishTgAccountStatusAuthorized {
		return id, nil
	}
	telegramUserId := strings.TrimSpace(account["telegram_user_id"].String())
	if telegramUserId == "" {
		return 0, gerror.New("TG账号已失效，请重新选择账号")
	}
	replacement, err := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("telegram_user_id", telegramUserId).
		Where("status", sysin.PublishTgAccountStatusAuthorized).
		WhereNull("deleted_at").
		OrderDesc("id").
		One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取TG账号最新授权失败")
	}
	if replacement.IsEmpty() {
		return 0, gerror.New("TG账号已失效，请重新扫码登录")
	}
	return replacement["id"].Int64(), nil
}

func (s *sSysPublish) ensureTgAccountsBelongAccount(ctx context.Context, ids []int64, tenantId int64, accountId int64) error {
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return gerror.New("请选择TG账号")
	}
	count, err := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查TG账号权限失败")
	}
	if count != len(ids) {
		return gerror.New("存在无权操作的TG账号")
	}
	return nil
}

func (s *sSysPublish) ensureTgAccountBelongsAccount(ctx context.Context, id int64, tenantId int64, accountId int64) error {
	return s.ensureTgAccountsBelongAccount(ctx, []int64{id}, tenantId, accountId)
}

func uniqueIds(ids []int64) []int64 {
	if len(ids) == 0 {
		return []int64{}
	}
	seen := make(map[int64]struct{}, len(ids))
	list := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		list = append(list, id)
	}
	return list
}
