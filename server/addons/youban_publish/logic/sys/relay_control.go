package sys

import (
	"context"
	"errors"
	"fmt"
	relayconsts "hotgo/addons/youban_publish/consts"
	"regexp"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	"hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/internal/model/entity"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func relayTaskID(key, prefix string) (int64, error) {
	if !strings.HasPrefix(key, prefix) {
		return 0, gerror.New("中转账号任务参数无效")
	}
	parts := strings.Split(strings.TrimPrefix(key, prefix), ":")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, gerror.New("中转账号任务编号无效")
	}
	return id, nil
}

func (s *sSysPublish) submitRelayControl(ctx context.Context, id int64) (resultErr error) {
	var row entity.YoubanPublishRelayChannel
	if err := dao.YoubanPublishRelayChannel.Ctx(ctx).Master().WherePri(id).Scan(&row); err != nil {
		return err
	}
	defer func() {
		if resultErr == nil || row.Id == 0 {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		c := dao.YoubanPublishRelayChannel.Columns()
		_, _ = dao.YoubanPublishRelayChannel.Ctx(cleanupCtx).WherePri(id).Where(c.TaskId, row.TaskId).Where(c.OperationStatus, relayconsts.RelayStatusPending).Data(do.YoubanPublishRelayChannel{OperationStatus: relayconsts.RelayStatusFailed, LastError: resultErr.Error()}).Update()
	}()
	// The worker must never see a submitted task before its fencing ID is committed.
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		taskID, err := collectorservice.AccountTasks().Submit(ctx, &collectorin.AccountTaskSubmit{
			TenantID: row.TenantId, AccountID: row.TgAccountId, TaskType: collectorin.AccountTaskTypeRelayControl,
			TaskKey: fmt.Sprintf("relay-control:%d:%s", id, guid.S()), Priority: collectorin.EventPriorityUrgent, MaxAttempts: 1})
		if err != nil {
			return gerror.Wrap(err, "提交中转频道管理任务失败")
		}
		_, err = tx.Model(dao.YoubanPublishRelayChannel.Table()).WherePri(id).Data(do.YoubanPublishRelayChannel{TaskId: taskID}).Update()
		return err
	})
}

func (s *sSysPublish) handleRelayControlTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) (resultErr error) {
	id, err := relayTaskID(task.TaskKey, "relay-control:")
	if err != nil {
		return err
	}
	var row entity.YoubanPublishRelayChannel
	if err = dao.YoubanPublishRelayChannel.Ctx(ctx).Master().WherePri(id).Scan(&row); err != nil {
		return err
	}
	if row.Id == 0 || row.TenantId != task.TenantID || row.TgAccountId != task.AccountID {
		return gerror.New("中转频道管理任务归属不匹配")
	}
	if row.TaskId != task.ID {
		return nil
	}
	defer func() {
		if resultErr != nil {
			_, _ = dao.YoubanPublishRelayChannel.Ctx(context.WithoutCancel(ctx)).WherePri(id).Where(dao.YoubanPublishRelayChannel.Columns().TaskId, task.ID).Data(do.YoubanPublishRelayChannel{OperationStatus: relayconsts.RelayStatusFailed, LastError: resultErr.Error(), HealthStatus: relayChannelHealthError}).Update()
		}
	}()
	if row.ChannelId == "" {
		if row.OperationKind != "create" || row.OperationStatus != relayconsts.RelayStatusPending {
			return gerror.New("频道创建结果待核实，禁止自动重复创建")
		}
		// Persist uncertainty BEFORE the irreversible create RPC. A lost response must not create twice.
		result, err := dao.YoubanPublishRelayChannel.Ctx(ctx).WherePri(id).Where(dao.YoubanPublishRelayChannel.Columns().TaskId, task.ID).Where(dao.YoubanPublishRelayChannel.Columns().OperationStatus, relayconsts.RelayStatusPending).Data(do.YoubanPublishRelayChannel{OperationStatus: relayconsts.RelayStatusCreating}).Update()
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return nil
		}
		updates, createErr := client.API().ChannelsCreateChannel(ctx, &tg.ChannelsCreateChannelRequest{Broadcast: true, Title: row.Title})
		if createErr != nil {
			return gerror.Wrap(createErr, "创建中转频道失败，请核实协议账号频道后再操作")
		}
		var chats []tg.ChatClass
		switch v := updates.(type) {
		case *tg.Updates:
			chats = v.Chats
		case *tg.UpdatesCombined:
			chats = v.Chats
		}
		for _, chat := range chats {
			if ch, ok := chat.(*tg.Channel); ok {
				hash, ok := ch.GetAccessHash()
				if !ok {
					continue
				}
				row.ChannelId = strconv.FormatInt(ch.ID, 10)
				row.AccessHash = strconv.FormatInt(hash, 10)
				row.TargetChatId = "-100" + row.ChannelId
				break
			}
		}
		if row.ChannelId == "" {
			return gerror.New("中转频道已创建但未取得标识，请核实后导入，禁止重复创建")
		}
		if _, err = dao.YoubanPublishRelayChannel.Ctx(ctx).WherePri(id).Where(dao.YoubanPublishRelayChannel.Columns().TaskId, task.ID).Data(do.YoubanPublishRelayChannel{ChannelId: row.ChannelId, AccessHash: row.AccessHash, TargetChatId: row.TargetChatId, OperationStatus: relayconsts.RelayStatusChecking}).Update(); err != nil {
			return err
		}
	}
	if row.BotId > 0 {
		botItem, botErr := s.getBotById(ctx, row.BotId, row.TenantId)
		if botErr != nil {
			return botErr
		}
		if err = s.ensureRelayBotWithClient(ctx, client, &row, botItem); err != nil {
			return err
		}
	}
	status := row.Status
	if row.OperationKind == "enable" {
		status = relayChannelStatusEnabled
	}
	_, err = dao.YoubanPublishRelayChannel.Ctx(ctx).WherePri(id).Where(dao.YoubanPublishRelayChannel.Columns().TaskId, task.ID).Data(do.YoubanPublishRelayChannel{
		Status: status, HealthStatus: relayChannelHealthReady, BotPermissionStatus: relayBotPermissionReady,
		OperationStatus: relayconsts.RelayStatusReady, LastError: "",
	}).Update()
	return err
}

// The account task executor serializes operations for the owning account.
// Bot identity and permissions are checked for the actual publisher, not a channel-wide flag.
func (s *sSysPublish) ensureRelayBotWithClient(ctx context.Context, client *telegram.Client, channel *entity.YoubanPublishRelayChannel, item *sysin.BotModel) (resultErr error) {
	bot, err := s.telegramMediaBot(ctx, item.BotToken)
	if err != nil {
		return err
	}
	me, err := bot.GetMe(ctx)
	if err != nil {
		return err
	}
	c := dao.YoubanPublishRelayBot.Columns()
	defer func() {
		data := do.YoubanPublishRelayBot{TenantId: channel.TenantId, RelayChannelId: channel.Id, BotId: item.Id, TelegramUserId: me.ID, Status: relayconsts.RelayStatusReady, CheckedAt: gtime.Now(), LastError: ""}
		if resultErr != nil {
			data.Status = relayconsts.RelayStatusMissing
			data.LastError = resultErr.Error()
		}
		_, saveErr := dao.YoubanPublishRelayBot.Ctx(context.WithoutCancel(ctx)).Data(data).OnConflict(c.TenantId + "," + c.RelayChannelId + "," + c.TelegramUserId).Save()
		if resultErr == nil {
			resultErr = saveErr
		}
	}()
	member, memberErr := bot.GetChatMember(ctx, &tgbot.GetChatMemberParams{ChatID: channel.TargetChatId, UserID: me.ID})
	if memberErr != nil {
		if scope, _ := relayFailureScope(memberErr); scope != "channel" {
			return memberErr
		}
	}
	if memberErr == nil && telegramBotCanSendMessage(member) && telegramBotCanDeleteMessage(member) {
		return nil
	}
	channelID, err := strconv.ParseInt(channel.ChannelId, 10, 64)
	if err != nil {
		return err
	}
	hash, err := strconv.ParseInt(channel.AccessHash, 10, 64)
	if err != nil || hash == 0 {
		return gerror.New("中转频道缺少有效access hash，请重新导入频道缓存")
	}
	input := &tg.InputChannel{ChannelID: channelID, AccessHash: hash}
	user, err := resolveRelayBot(ctx, client.API(), me.Username)
	if err != nil {
		return err
	}
	if user.UserID != me.ID {
		return gerror.New("Bot用户名对应身份与发布Bot不一致")
	}
	if inviteErr := func() error {
		_, inviteErr := client.API().ChannelsInviteToChannel(ctx, &tg.ChannelsInviteToChannelRequest{Channel: input, Users: []tg.InputUserClass{user}})
		return inviteErr
	}(); inviteErr != nil && !tgerr.Is(inviteErr, "USER_ALREADY_PARTICIPANT") && !tgerr.Is(inviteErr, "USER_BOT") {
		return gerror.Wrap(inviteErr, "自动邀请发布Bot到中转频道失败")
	}
	if _, err = client.API().ChannelsEditAdmin(ctx, &tg.ChannelsEditAdminRequest{Channel: input, UserID: user, AdminRights: tg.ChatAdminRights{PostMessages: true, DeleteMessages: true}, Rank: "中转发布"}); err != nil && !tgerr.Is(err, "CHAT_NOT_MODIFIED") {
		return gerror.Wrap(err, "授予中转Bot发布和清理权限失败")
	}
	member, err = bot.GetChatMember(ctx, &tgbot.GetChatMemberParams{ChatID: channel.TargetChatId, UserID: me.ID})
	if err != nil {
		return err
	}
	if !telegramBotCanSendMessage(member) || !telegramBotCanDeleteMessage(member) {
		return gerror.New("中转Bot权限尚未生效，稍后重试")
	}
	return nil
}

var relaySlowModeWaitPattern = regexp.MustCompile(`(?i)SLOWMODE_WAIT[_ ]?\(?([0-9]+)\)?`)

// Only an explicitly scoped channel failure permits changing just the channel.
func relayFailureScope(err error) (string, time.Duration) {
	if err == nil {
		return "", 0
	}
	var retry *tgbot.TooManyRequestsError
	if errors.As(err, &retry) {
		return "bot", time.Duration(retry.RetryAfter) * time.Second
	}
	if matches := relaySlowModeWaitPattern.FindStringSubmatch(err.Error()); len(matches) == 2 {
		seconds, parseErr := strconv.Atoi(matches[1])
		if parseErr == nil && seconds > 0 {
			return "channel", time.Duration(seconds+2) * time.Second
		}
	}
	if delay, ok := collectMediaFloodWaitDelay(err); ok {
		return "account", delay
	}
	if tgerr.Is(err, "CHANNEL_PRIVATE", "CHANNEL_INVALID", "CHAT_WRITE_FORBIDDEN", "CHAT_ADMIN_REQUIRED", "USER_BANNED_IN_CHANNEL") {
		return "channel", time.Minute
	}
	if isTelegramCopySourceUnavailableError(err) {
		return "channel", time.Minute
	}
	message := strings.ToLower(err.Error())
	for _, part := range []string{"bot is not a member", "bot was kicked", "bad request: user not found", "not enough rights to send"} {
		if strings.Contains(message, part) {
			return "channel", time.Minute
		}
	}
	if isTelegramAmbiguousDeliveryError(err) {
		return "uncertain", time.Minute
	}
	return "temporary", 30 * time.Second
}
