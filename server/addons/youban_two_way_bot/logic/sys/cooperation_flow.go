package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	botservice "hotgo/addons/youban_bot/service"
	twdao "hotgo/addons/youban_two_way_bot/internal/dao"
	"hotgo/addons/youban_two_way_bot/internal/model/entity"
	"hotgo/addons/youban_two_way_bot/model/input/sysin"
	"hotgo/internal/library/cache"
)

const cooperationSessionTTL = 10 * time.Minute

type cooperationConfigRuntime struct {
	Id               int64  `json:"id"`
	TenantId         int64  `json:"tenantId"`
	BotId            int64  `json:"botId"`
	TwoWayBotId      int64  `json:"twoWayBotId"`
	ReviewRequired   int    `json:"reviewRequired"`
	NotificationType string `json:"notificationType"`
}
type cooperationResolvedBot struct {
	UserId   int64
	Username string
	Name     string
}
type cooperationChannelRuntime struct {
	Id           int64  `json:"id"`
	TgAccountId  int64  `json:"tgAccountId"`
	ChannelTitle string `json:"channelTitle"`
	TargetChatId string `json:"targetChatId"`
	AccessHash   string `json:"accessHash"`
}

func (s *sSysTwoWayBot) handleCooperationPrivateMessage(ctx context.Context, bot *tgbot.Bot, row *entity.YoubanTwoWayBotBot, msg *models.Message) (bool, error) {
	config, err := cooperationConfigByToken(ctx, row.BotToken)
	if err != nil || config == nil {
		return false, err
	}
	text := strings.TrimSpace(msg.Text)
	sessionKey := cooperationSubmissionSessionKey(row.Id, msg.From.ID)
	if text == "平台合作" || strings.EqualFold(text, "/cooperation") {
		_ = cache.Instance().Set(ctx, sessionKey, 1, cooperationSessionTTL)
		_, err = bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: msg.Chat.ID, Text: "请发送需要合作的机器人用户名，例如：@example_bot"})
		return true, err
	}
	state, _ := cache.Instance().Get(ctx, sessionKey)
	if state == nil || state.IsNil() {
		return false, nil
	}
	username := strings.TrimPrefix(strings.TrimSpace(text), "@")
	if username == "" {
		return true, nil
	}
	resolved, err := s.resolveCooperationBot(ctx, config, username)
	if err != nil {
		_, _ = bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: msg.Chat.ID, Text: err.Error()})
		return true, nil
	}
	applicationId, reviewRequired, err := s.createCooperationApplication(ctx, config, msg.From, resolved)
	if err != nil {
		_, _ = bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: msg.Chat.ID, Text: err.Error()})
		return true, nil
	}
	_, _ = cache.Instance().Remove(ctx, sessionKey)
	if reviewRequired == 1 {
		_, _ = bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: msg.Chat.ID, Text: "合作申请已提交，等待平台审核。"})
		_ = s.notifyCooperationReview(ctx, row, bot, applicationId, config, resolved, msg.From)
	} else {
		_, _ = bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: msg.Chat.ID, Text: "合作申请已提交，正在自动加入频道。"})
		if processErr := s.processCooperationApplication(ctx, applicationId, config.TenantId); processErr != nil {
			g.Log().Warningf(ctx, "自动处理合作申请失败 id:%d err:%+v", applicationId, processErr)
		}
	}
	return true, nil
}

func cooperationSubmissionSessionKey(botId, userId int64) string {
	return fmt.Sprintf("ybtwb:cooperation:session:%d:%d", botId, userId)
}

func cooperationConfigByToken(ctx context.Context, token string) (*cooperationConfigRuntime, error) {
	var row *cooperationConfigRuntime
	err := g.DB().Model(twdao.YoubanTwoWayBotCooperationConfig.Table()).As("cc").Fields("cc.id,cc.tenant_id,cc.bot_id,cc.two_way_bot_id,cc.review_required,cc.notification_type").InnerJoin("hg_youban_publish_bot pb", "pb.id=cc.bot_id").Where("pb.bot_token", strings.TrimSpace(token)).Where("cc.status", 1).WhereNull("cc.deleted_at").WhereNull("pb.deleted_at").Scan(&row)
	return row, err
}

func (s *sSysTwoWayBot) resolveCooperationBot(ctx context.Context, config *cooperationConfigRuntime, username string) (*cooperationResolvedBot, error) {
	channels, err := cooperationChannels(ctx, config.Id)
	if err != nil || len(channels) == 0 {
		return nil, gerror.New("平台尚未配置可用频道")
	}
	account, err := tgAccountById(ctx, channels[0].TgAccountId, config.TenantId)
	if err != nil {
		return nil, err
	}
	conf, err := publishTelegramConfig(ctx)
	if err != nil {
		return nil, err
	}
	storage, err := telegramSessionStorage(account.SessionKey)
	if err != nil {
		return nil, err
	}
	options := telegram.Options{SessionStorage: storage}
	if resolver, resolveErr := telegramMTProtoResolver(conf.ProxyUrl); resolveErr != nil {
		return nil, resolveErr
	} else if resolver != nil {
		options.Resolver = resolver
	}
	client := telegram.NewClient(conf.AppId, conf.AppHash, options)
	runCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	var result *cooperationResolvedBot
	err = client.Run(runCtx, func(clientCtx context.Context) error {
		resolved, resolveErr := client.API().ContactsResolveUsername(clientCtx, &tg.ContactsResolveUsernameRequest{Username: username})
		if resolveErr != nil {
			return gerror.New("未找到该 Telegram 用户名")
		}
		for _, item := range resolved.Users {
			user, ok := item.(*tg.User)
			if !ok {
				continue
			}
			if !user.Bot {
				return gerror.New("该用户名不是 Telegram Bot，仅支持提交机器人")
			}
			name := strings.TrimSpace(user.FirstName + " " + user.LastName)
			if name == "" {
				name = user.Username
			}
			result = &cooperationResolvedBot{UserId: user.ID, Username: strings.TrimPrefix(user.Username, "@"), Name: name}
			return nil
		}
		return gerror.New("未找到该 Telegram Bot")
	})
	return result, err
}

func (s *sSysTwoWayBot) createCooperationApplication(ctx context.Context, config *cooperationConfigRuntime, applicant *models.User, submitted *cooperationResolvedBot) (int64, int, error) {
	columns := twdao.YoubanTwoWayBotCooperationApplication.Columns()
	blacklistColumns := twdao.YoubanTwoWayBotCooperationBlacklist.Columns()
	blocked, err := twdao.YoubanTwoWayBotCooperationBlacklist.Ctx(ctx).Where(blacklistColumns.ConfigId, config.Id).Where(blacklistColumns.ApplicantTgUserId, strconv.FormatInt(applicant.ID, 10)).Where(blacklistColumns.Status, 1).Count()
	if err != nil {
		return 0, 0, err
	}
	if blocked > 0 {
		return 0, 0, gerror.New("您已被平台限制提交合作申请，请联系平台管理员")
	}
	passed, err := twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(columns.ConfigId, config.Id).Where(columns.SubmittedBotUserId, strconv.FormatInt(submitted.UserId, 10)).WhereIn(columns.ReviewStatus, []string{sysin.CooperationReviewApproved, sysin.CooperationReviewNotRequired}).WhereIn(columns.JoinStatus, []string{sysin.CooperationJoinSuccess, sysin.CooperationJoinPartialFailed}).WhereNull(columns.DeletedAt).Count()
	if err != nil {
		return 0, 0, err
	}
	if passed > 0 {
		return 0, 0, gerror.New("该机器人已经通过合作申请")
	}
	count, err := twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(columns.ConfigId, config.Id).Where(columns.SubmittedBotUserId, strconv.FormatInt(submitted.UserId, 10)).WhereIn(columns.ReviewStatus, []string{sysin.CooperationReviewPending, sysin.CooperationReviewApproved, sysin.CooperationReviewNotRequired}).WhereIn(columns.JoinStatus, []string{sysin.CooperationJoinNotStarted, sysin.CooperationJoinProcessing, sysin.CooperationJoinSuccess, sysin.CooperationJoinPartialFailed}).WhereNull(columns.DeletedAt).Count()
	if err != nil {
		return 0, 0, err
	}
	if count > 0 {
		return 0, 0, gerror.New("该机器人已有合作申请，请等待处理")
	}
	reviewStatus := sysin.CooperationReviewPending
	if config.ReviewRequired == 0 {
		reviewStatus = sysin.CooperationReviewNotRequired
	}
	now := gtime.Now()
	id, err := twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Data(g.Map{columns.TenantId: config.TenantId, columns.ConfigId: config.Id, columns.ApplicantTgUserId: strconv.FormatInt(applicant.ID, 10), columns.ApplicantUsername: strings.TrimPrefix(applicant.Username, "@"), columns.ApplicantFirstName: applicant.FirstName, columns.ApplicantLastName: applicant.LastName, columns.SubmittedBotUserId: strconv.FormatInt(submitted.UserId, 10), columns.SubmittedBotUsername: submitted.Username, columns.SubmittedBotName: submitted.Name, columns.ReviewStatus: reviewStatus, columns.JoinStatus: sysin.CooperationJoinNotStarted, columns.SubmittedAt: now, columns.CreatedAt: now, columns.UpdatedAt: now}).InsertAndGetId()
	if err != nil {
		return 0, 0, err
	}
	channelColumns := twdao.YoubanTwoWayBotCooperationApplicationChannel.Columns()
	channels, _ := cooperationChannels(ctx, config.Id)
	for _, channel := range channels {
		_, _ = twdao.YoubanTwoWayBotCooperationApplicationChannel.Ctx(ctx).Data(g.Map{channelColumns.TenantId: config.TenantId, channelColumns.ApplicationId: id, channelColumns.ChannelId: channel.Id, channelColumns.Status: sysin.CooperationJoinNotStarted, channelColumns.CreatedAt: now, channelColumns.UpdatedAt: now}).Insert()
	}
	return id, config.ReviewRequired, nil
}

func (s *sSysTwoWayBot) notifyCooperationReview(ctx context.Context, row *entity.YoubanTwoWayBotBot, bot *tgbot.Bot, applicationId int64, config *cooperationConfigRuntime, submitted *cooperationResolvedBot, applicant *models.User) error {
	if config.NotificationType == "official" {
		return s.notifyCooperationOfficialAdmin(ctx, config, applicationId, submitted, applicant)
	}
	notifyRow := row
	notifyBot := bot
	if config.TwoWayBotId > 0 && (row == nil || config.TwoWayBotId != row.Id) {
		selected, err := s.botById(ctx, config.TwoWayBotId, config.TenantId)
		if err != nil {
			return err
		}
		notifyRow = selected
		notifyBot, err = s.telegramBot(ctx, selected.BotToken)
		if err != nil {
			return err
		}
	}
	if notifyRow == nil || strings.TrimSpace(notifyRow.SupergroupId) == "" {
		return nil
	}
	topicThreadId, err := s.cooperationReviewTopicThreadId(ctx, notifyBot, notifyRow, config.Id)
	if err != nil {
		return err
	}
	text := fmt.Sprintf("平台合作申请\n\n申请人：%s\n申请人ID：%d\n机器人：@%s\nBot ID：%d", cooperationApplicantDisplay(applicant), applicant.ID, submitted.Username, submitted.UserId)
	markup := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "通过", CallbackData: fmt.Sprintf("coop:approve:%d", applicationId)}, {Text: "拒绝", CallbackData: fmt.Sprintf("coop:reject:%d", applicationId)}, {Text: "拉黑", CallbackData: fmt.Sprintf("coop:blacklist:%d", applicationId)}},
		{cooperationApplicantButton(applicant)},
	}}
	_, err = notifyBot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: notifyRow.SupergroupId, MessageThreadID: topicThreadId, Text: text, ReplyMarkup: markup})
	if err != nil && cooperationReviewTopicUnavailable(err) {
		_, _ = cache.Instance().Remove(ctx, cooperationReviewTopicCacheKey(config.Id))
		topicThreadId, err = s.cooperationReviewTopicThreadId(ctx, notifyBot, notifyRow, config.Id)
		if err == nil {
			_, err = notifyBot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: notifyRow.SupergroupId, MessageThreadID: topicThreadId, Text: text, ReplyMarkup: markup})
		}
	}
	if err == nil {
		columns := twdao.YoubanTwoWayBotCooperationApplication.Columns()
		_, _ = twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(columns.Id, applicationId).Data(g.Map{columns.TopicThreadId: topicThreadId, columns.UpdatedAt: gtime.Now()}).Update()
	}
	return err
}

func (s *sSysTwoWayBot) cooperationReviewTopicThreadId(ctx context.Context, bot *tgbot.Bot, row *entity.YoubanTwoWayBotBot, configId int64) (int, error) {
	value, err := cache.Instance().GetOrSetFuncLock(ctx, cooperationReviewTopicCacheKey(configId), func(ctx context.Context) (interface{}, error) {
		topic, createErr := bot.CreateForumTopic(ctx, &tgbot.CreateForumTopicParams{ChatID: row.SupergroupId, Name: "合作申请"})
		if createErr != nil {
			return nil, createErr
		}
		return topic.MessageThreadID, nil
	}, 3650*24*time.Hour)
	if err != nil {
		return 0, err
	}
	threadId := value.Int()
	if threadId <= 0 {
		return 0, gerror.New("合作申请话题创建失败")
	}
	return threadId, nil
}

func cooperationReviewTopicCacheKey(configId int64) string {
	return fmt.Sprintf("ybtwb:cooperation:review-topic:%d", configId)
}

func cooperationReviewTopicUnavailable(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToUpper(err.Error())
	return strings.Contains(text, "MESSAGE_THREAD_INVALID") || strings.Contains(text, "TOPIC_CLOSED") || strings.Contains(text, "MESSAGE THREAD NOT FOUND")
}

func (s *sSysTwoWayBot) notifyCooperationOfficialAdmin(ctx context.Context, config *cooperationConfigRuntime, applicationId int64, submitted *cooperationResolvedBot, applicant *models.User) error {
	var accountId int64
	if err := g.DB().Model(twdao.YoubanTwoWayBotCooperationConfig.Table()).Fields("account_id").Where("id", config.Id).Scan(&accountId); err != nil || accountId <= 0 {
		return err
	}
	var chatId string
	if err := g.DB().Model("hg_youban_bot_account_bind").Fields("telegram_user_id").Where("app", "api").Where("account_id", accountId).Where("status", 1).WhereNull("deleted_at").Scan(&chatId); err != nil || chatId == "" {
		return err
	}
	token, err := botservice.SysBot().OfficialBotToken(ctx)
	if err != nil {
		return err
	}
	officialBot, err := s.telegramBot(ctx, token)
	if err != nil {
		return err
	}
	text := fmt.Sprintf("平台合作申请 #%d\n申请人：%s (%d)\n机器人：@%s (%d)\n请前往平台合作页面审核。", applicationId, cooperationApplicantDisplay(applicant), applicant.ID, submitted.Username, submitted.UserId)
	markup := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{cooperationApplicantButton(applicant)}}}
	_, err = officialBot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: chatId, Text: text, ReplyMarkup: markup})
	return err
}

func cooperationApplicantDisplay(applicant *models.User) string {
	if applicant == nil {
		return "未知用户"
	}
	if username := strings.TrimPrefix(strings.TrimSpace(applicant.Username), "@"); username != "" {
		return "@" + username
	}
	return cooperationApplicantNickname(applicant)
}

func cooperationApplicantNickname(applicant *models.User) string {
	if applicant == nil {
		return "未知用户"
	}
	name := strings.TrimSpace(strings.Join([]string{applicant.FirstName, applicant.LastName}, " "))
	if name == "" {
		name = strings.TrimPrefix(strings.TrimSpace(applicant.Username), "@")
	}
	if name == "" {
		name = fmt.Sprintf("用户 %d", applicant.ID)
	}
	runes := []rune(name)
	if len(runes) > 48 {
		name = string(runes[:48]) + "…"
	}
	return name
}

func cooperationApplicantButton(applicant *models.User) models.InlineKeyboardButton {
	if applicant == nil {
		return models.InlineKeyboardButton{Text: "未知用户", CallbackData: "coop:user:unknown"}
	}
	return models.InlineKeyboardButton{Text: cooperationApplicantNickname(applicant), URL: fmt.Sprintf("tg://user?id=%d", applicant.ID)}
}

func (s *sSysTwoWayBot) handleCooperationCallback(ctx context.Context, bot *tgbot.Bot, row *entity.YoubanTwoWayBotBot, query *models.CallbackQuery) (bool, error) {
	if query == nil || !strings.HasPrefix(query.Data, "coop:") {
		return false, nil
	}
	parts := strings.Split(query.Data, ":")
	if len(parts) != 3 {
		_, _ = bot.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "操作参数无效", ShowAlert: true})
		return true, nil
	}
	id, _ := strconv.ParseInt(parts[2], 10, 64)
	if parts[1] == "status" {
		_, _ = bot.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "该申请已处理"})
		return true, nil
	}
	ok, err := s.isGroupAdmin(ctx, bot, row, query.From.ID)
	if err != nil {
		_, _ = bot.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "验证管理员权限失败：" + err.Error(), ShowAlert: true})
		return true, nil
	}
	if !ok {
		_, _ = bot.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "仅管理群管理员可操作", ShowAlert: true})
		return true, nil
	}
	_, _ = bot.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "正在处理，请稍候…"})
	statusText := ""
	switch parts[1] {
	case "approve":
		err = s.approveCooperationApplication(ctx, id, row.TenantId, 0, "Telegram审核通过")
		statusText = "已通过"
	case "reject":
		err = s.rejectCooperationApplicationByTelegram(ctx, id, row.TenantId)
		statusText = "已拒绝"
	case "blacklist":
		err = s.blacklistCooperationApplication(ctx, id, row.TenantId, 0, "Telegram管理员拉黑")
		statusText = "已拉黑"
	default:
		err = gerror.New("不支持的操作")
	}
	if err != nil {
		_ = s.sendCooperationCallbackResult(ctx, bot, query, "操作失败："+err.Error())
		return true, nil
	}
	if editErr := s.editCooperationCallbackStatus(ctx, bot, query, id, statusText); editErr != nil {
		g.Log().Warningf(ctx, "更新合作审核按钮失败 applicationId:%d status:%s err:%+v", id, statusText, editErr)
	}
	return true, nil
}

func (s *sSysTwoWayBot) editCooperationCallbackStatus(ctx context.Context, bot *tgbot.Bot, query *models.CallbackQuery, applicationId int64, status string) error {
	chatId, messageId, _, ok := cooperationCallbackMessage(query)
	if !ok {
		return gerror.New("无法读取审核消息")
	}
	markup := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{
		Text:         status,
		CallbackData: fmt.Sprintf("coop:status:%d", applicationId),
	}}}}
	_, err := bot.EditMessageReplyMarkup(ctx, &tgbot.EditMessageReplyMarkupParams{ChatID: chatId, MessageID: messageId, ReplyMarkup: markup})
	return err
}

func (s *sSysTwoWayBot) sendCooperationCallbackResult(ctx context.Context, bot *tgbot.Bot, query *models.CallbackQuery, text string) error {
	chatId, _, threadId, ok := cooperationCallbackMessage(query)
	if !ok {
		return nil
	}
	_, err := bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: chatId, MessageThreadID: threadId, Text: text})
	return err
}

func cooperationCallbackMessage(query *models.CallbackQuery) (chatId int64, messageId int, threadId int, ok bool) {
	if query == nil {
		return 0, 0, 0, false
	}
	if query.Message.Message != nil {
		message := query.Message.Message
		return message.Chat.ID, message.ID, message.MessageThreadID, true
	}
	if query.Message.InaccessibleMessage != nil {
		message := query.Message.InaccessibleMessage
		return message.Chat.ID, message.MessageID, 0, true
	}
	return 0, 0, 0, false
}

func (s *sSysTwoWayBot) approveCooperationApplication(ctx context.Context, id, tenantId, reviewerId int64, remark string) error {
	columns := twdao.YoubanTwoWayBotCooperationApplication.Columns()
	result, err := twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(columns.Id, id).Where(columns.TenantId, tenantId).Where(columns.ReviewStatus, sysin.CooperationReviewPending).Data(g.Map{columns.ReviewStatus: sysin.CooperationReviewApproved, columns.ReviewedBy: reviewerId, columns.ReviewRemark: remark, columns.ReviewedAt: gtime.Now(), columns.UpdatedAt: gtime.Now()}).Update()
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return gerror.New("申请状态已变化")
	}
	return s.processCooperationApplication(ctx, id, tenantId)
}
func (s *sSysTwoWayBot) rejectCooperationApplicationByTelegram(ctx context.Context, id, tenantId int64) error {
	columns := twdao.YoubanTwoWayBotCooperationApplication.Columns()
	_, err := twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(columns.Id, id).Where(columns.TenantId, tenantId).Where(columns.ReviewStatus, sysin.CooperationReviewPending).Data(g.Map{columns.ReviewStatus: sysin.CooperationReviewRejected, columns.ReviewedAt: gtime.Now(), columns.UpdatedAt: gtime.Now()}).Update()
	return err
}

func (s *sSysTwoWayBot) processCooperationApplication(ctx context.Context, id, tenantId int64) error {
	appColumns := twdao.YoubanTwoWayBotCooperationApplication.Columns()
	var app *entity.YoubanTwoWayBotCooperationApplication
	if err := twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(appColumns.Id, id).Where(appColumns.TenantId, tenantId).WhereNull(appColumns.DeletedAt).Scan(&app); err != nil {
		return err
	}
	if app == nil {
		return gerror.New("合作申请不存在")
	}
	_, _ = twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(appColumns.Id, id).Data(g.Map{appColumns.JoinStatus: sysin.CooperationJoinProcessing, appColumns.ErrorMessage: "", appColumns.UpdatedAt: gtime.Now()}).Update()
	channels, err := cooperationChannels(ctx, app.ConfigId)
	if err != nil {
		return s.failCooperationApplication(ctx, app, "读取合作频道失败："+err.Error())
	}
	if len(channels) == 0 {
		return s.failCooperationApplication(ctx, app, "平台未配置可用的上架频道")
	}
	success, failed := 0, 0
	messages := []string{}
	channelColumns := twdao.YoubanTwoWayBotCooperationApplicationChannel.Columns()
	for _, channel := range channels {
		joinErr := s.addCooperationBotChannelAdmin(ctx, tenantId, channel, app.SubmittedBotUsername)
		status := sysin.CooperationJoinSuccess
		message := ""
		data := g.Map{channelColumns.Status: status, channelColumns.ErrorMessage: "", channelColumns.UpdatedAt: gtime.Now(), channelColumns.JoinedAt: gtime.Now()}
		if joinErr != nil {
			failed++
			status = sysin.CooperationJoinFailed
			message = cooperationTelegramError(joinErr)
			data[channelColumns.Status] = status
			data[channelColumns.ErrorMessage] = message
			data[channelColumns.JoinedAt] = nil
			messages = append(messages, channel.ChannelTitle+"："+message)
		} else {
			success++
		}
		_, _ = twdao.YoubanTwoWayBotCooperationApplicationChannel.Ctx(ctx).Where(channelColumns.ApplicationId, id).Where(channelColumns.ChannelId, channel.Id).Data(data).Update()
	}
	joinStatus := sysin.CooperationJoinSuccess
	if failed > 0 && success > 0 {
		joinStatus = sysin.CooperationJoinPartialFailed
	} else if failed > 0 {
		joinStatus = sysin.CooperationJoinFailed
	}
	errorMessage := strings.Join(messages, "；")
	_, err = twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(appColumns.Id, id).Data(g.Map{appColumns.JoinStatus: joinStatus, appColumns.ErrorMessage: errorMessage, appColumns.UpdatedAt: gtime.Now()}).Update()
	if err != nil {
		return err
	}
	if notifyErr := s.notifyCooperationApplicant(ctx, id, tenantId, joinStatus); notifyErr != nil {
		g.Log().Warningf(ctx, "通知合作申请人失败 applicationId:%d status:%s err:%+v", id, joinStatus, notifyErr)
		return gerror.Wrap(notifyErr, "合作已通过，但通知申请人失败")
	}
	return nil
}

func (s *sSysTwoWayBot) terminateCooperationApplication(ctx context.Context, id, tenantId, reviewerId int64, remark string) error {
	appColumns := twdao.YoubanTwoWayBotCooperationApplication.Columns()
	var app *entity.YoubanTwoWayBotCooperationApplication
	if err := twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(appColumns.Id, id).Where(appColumns.TenantId, tenantId).WhereNull(appColumns.DeletedAt).Scan(&app); err != nil {
		return err
	}
	if app == nil || app.Id <= 0 {
		return gerror.New("合作申请不存在")
	}
	if app.ReviewStatus != sysin.CooperationReviewApproved && app.ReviewStatus != sysin.CooperationReviewNotRequired && app.ReviewStatus != sysin.CooperationReviewCanceled {
		return gerror.New("只有已通过的合作才能取消")
	}
	channels, err := cooperationApplicationRemovalChannels(ctx, app.Id)
	if err != nil {
		return gerror.Wrap(err, "读取合作频道失败")
	}
	success, failed := 0, 0
	messages := make([]string, 0)
	channelColumns := twdao.YoubanTwoWayBotCooperationApplicationChannel.Columns()
	for _, channel := range channels {
		removeErr := s.removeCooperationBotFromChannel(ctx, tenantId, channel, app.SubmittedBotUsername)
		status := sysin.CooperationJoinRemoved
		errorMessage := ""
		if removeErr != nil {
			failed++
			status = sysin.CooperationJoinRemoveFailed
			errorMessage = cooperationTelegramError(removeErr)
			messages = append(messages, channel.ChannelTitle+"："+errorMessage)
		} else {
			success++
		}
		_, _ = twdao.YoubanTwoWayBotCooperationApplicationChannel.Ctx(ctx).
			Where(channelColumns.ApplicationId, app.Id).
			Where(channelColumns.ChannelId, channel.Id).
			Data(g.Map{channelColumns.Status: status, channelColumns.ErrorMessage: errorMessage, channelColumns.UpdatedAt: gtime.Now()}).
			Update()
	}
	joinStatus := sysin.CooperationJoinRemoved
	if failed > 0 && success > 0 {
		joinStatus = sysin.CooperationJoinPartialRemove
	} else if failed > 0 {
		joinStatus = sysin.CooperationJoinRemoveFailed
	}
	if remark == "" {
		remark = "后台取消合作"
	}
	_, err = twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(appColumns.Id, app.Id).Data(g.Map{
		appColumns.ReviewStatus: sysin.CooperationReviewCanceled,
		appColumns.JoinStatus:   joinStatus,
		appColumns.ErrorMessage: strings.Join(messages, "；"),
		appColumns.ReviewedBy:   reviewerId,
		appColumns.ReviewRemark: remark,
		appColumns.ReviewedAt:   gtime.Now(),
		appColumns.UpdatedAt:    gtime.Now(),
	}).Update()
	if err != nil {
		return err
	}
	return nil
}

func (s *sSysTwoWayBot) failCooperationApplication(ctx context.Context, app *entity.YoubanTwoWayBotCooperationApplication, message string) error {
	if app == nil {
		return gerror.New(message)
	}
	columns := twdao.YoubanTwoWayBotCooperationApplication.Columns()
	_, _ = twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(columns.Id, app.Id).Data(g.Map{columns.JoinStatus: sysin.CooperationJoinFailed, columns.ErrorMessage: message, columns.UpdatedAt: gtime.Now()}).Update()
	if notifyErr := s.notifyCooperationApplicant(ctx, app.Id, app.TenantId, sysin.CooperationJoinFailed); notifyErr != nil {
		g.Log().Warningf(ctx, "通知合作申请失败结果异常 applicationId:%d err:%+v", app.Id, notifyErr)
	}
	return gerror.New(message)
}

func cooperationChannels(ctx context.Context, configId int64) ([]*cooperationChannelRuntime, error) {
	var rows []*cooperationChannelRuntime
	err := g.DB().Model(twdao.YoubanTwoWayBotCooperationChannel.Table()).As("cc").Fields("c.id,c.tg_account_id,c.channel_title,c.target_chat_id,tc.access_hash").InnerJoin("hg_youban_publish_channel c", "c.id=cc.channel_id").LeftJoin("hg_youban_publish_tg_channel tc", "tc.tenant_id=c.tenant_id AND tc.tg_account_id=c.tg_account_id AND tc.channel_id=c.target_chat_id").Where("cc.config_id", configId).Where("cc.status", 1).WhereNull("cc.deleted_at").WhereNull("c.deleted_at").Scan(&rows)
	return rows, err
}

func cooperationApplicationRemovalChannels(ctx context.Context, applicationId int64) ([]*cooperationChannelRuntime, error) {
	var rows []*cooperationChannelRuntime
	err := g.DB().Model(twdao.YoubanTwoWayBotCooperationApplicationChannel.Table()).As("ac").
		Fields("c.id,c.tg_account_id,c.channel_title,c.target_chat_id,tc.access_hash").
		InnerJoin("hg_youban_publish_channel c", "c.id=ac.channel_id").
		LeftJoin("hg_youban_publish_tg_channel tc", "tc.tenant_id=c.tenant_id AND tc.tg_account_id=c.tg_account_id AND tc.channel_id=c.target_chat_id").
		Where("ac.application_id", applicationId).
		WhereIn("ac.status", []string{sysin.CooperationJoinSuccess, sysin.CooperationJoinRemoveFailed}).
		WhereNull("c.deleted_at").
		Scan(&rows)
	return rows, err
}

func (s *sSysTwoWayBot) addCooperationBotChannelAdmin(ctx context.Context, tenantId int64, channel *cooperationChannelRuntime, username string) error {
	account, err := tgAccountById(ctx, channel.TgAccountId, tenantId)
	if err != nil {
		return err
	}
	conf, err := publishTelegramConfig(ctx)
	if err != nil {
		return err
	}
	storage, err := telegramSessionStorage(account.SessionKey)
	if err != nil {
		return err
	}
	options := telegram.Options{SessionStorage: storage}
	if resolver, resolveErr := telegramMTProtoResolver(conf.ProxyUrl); resolveErr != nil {
		return resolveErr
	} else if resolver != nil {
		options.Resolver = resolver
	}
	channelId, err := strconv.ParseInt(channel.TargetChatId, 10, 64)
	if err != nil {
		return gerror.New("频道ID无效")
	}
	accessHash, err := strconv.ParseInt(channel.AccessHash, 10, 64)
	if err != nil {
		return gerror.New("频道缓存已失效，请刷新频道缓存")
	}
	client := telegram.NewClient(conf.AppId, conf.AppHash, options)
	runCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	return client.Run(runCtx, func(clientCtx context.Context) error {
		api := client.API()
		inputUser, resolveErr := resolveBotInputUser(clientCtx, api, username)
		if resolveErr != nil {
			return resolveErr
		}
		_, editErr := api.ChannelsEditAdmin(clientCtx, &tg.ChannelsEditAdminRequest{Channel: &tg.InputChannel{ChannelID: channelId, AccessHash: accessHash}, UserID: inputUser, AdminRights: cooperationChannelAdminRights(), Rank: "平台合作"})
		return editErr
	})
}

func (s *sSysTwoWayBot) removeCooperationBotFromChannel(ctx context.Context, tenantId int64, channel *cooperationChannelRuntime, username string) error {
	account, err := tgAccountById(ctx, channel.TgAccountId, tenantId)
	if err != nil {
		return err
	}
	conf, err := publishTelegramConfig(ctx)
	if err != nil {
		return err
	}
	storage, err := telegramSessionStorage(account.SessionKey)
	if err != nil {
		return err
	}
	options := telegram.Options{SessionStorage: storage}
	if resolver, resolveErr := telegramMTProtoResolver(conf.ProxyUrl); resolveErr != nil {
		return resolveErr
	} else if resolver != nil {
		options.Resolver = resolver
	}
	channelId, err := strconv.ParseInt(channel.TargetChatId, 10, 64)
	if err != nil {
		return gerror.New("频道ID无效")
	}
	accessHash, err := strconv.ParseInt(channel.AccessHash, 10, 64)
	if err != nil {
		return gerror.New("频道缓存已失效，请刷新频道缓存")
	}
	client := telegram.NewClient(conf.AppId, conf.AppHash, options)
	runCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	return client.Run(runCtx, func(clientCtx context.Context) error {
		api := client.API()
		inputUser, resolveErr := resolveBotInputUser(clientCtx, api, username)
		if resolveErr != nil {
			return resolveErr
		}
		channelInput := &tg.InputChannel{ChannelID: channelId, AccessHash: accessHash}
		_, demoteErr := api.ChannelsEditAdmin(clientCtx, &tg.ChannelsEditAdminRequest{Channel: channelInput, UserID: inputUser, AdminRights: tg.ChatAdminRights{}, Rank: ""})
		if demoteErr != nil && !cooperationBotAlreadyRemoved(demoteErr) {
			return demoteErr
		}
		peer := &tg.InputPeerUser{UserID: inputUser.UserID, AccessHash: inputUser.AccessHash}
		_, banErr := api.ChannelsEditBanned(clientCtx, &tg.ChannelsEditBannedRequest{Channel: channelInput, Participant: peer, BannedRights: tg.ChatBannedRights{ViewMessages: true}})
		if banErr != nil && !cooperationBotAlreadyRemoved(banErr) {
			return banErr
		}
		_, unbanErr := api.ChannelsEditBanned(clientCtx, &tg.ChannelsEditBannedRequest{Channel: channelInput, Participant: peer, BannedRights: tg.ChatBannedRights{}})
		if unbanErr != nil && !cooperationBotAlreadyRemoved(unbanErr) {
			return unbanErr
		}
		return nil
	})
}

func cooperationBotAlreadyRemoved(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToUpper(err.Error())
	return strings.Contains(text, "USER_NOT_PARTICIPANT") || strings.Contains(text, "PARTICIPANT_ID_INVALID")
}

func cooperationChannelAdminRights() tg.ChatAdminRights {
	return tg.ChatAdminRights{
		ChangeInfo:     true,
		PostMessages:   true,
		EditMessages:   true,
		DeleteMessages: true,
		InviteUsers:    true,
		PostStories:    true,
		EditStories:    true,
		DeleteStories:  true,
	}
}

func (s *sSysTwoWayBot) notifyCooperationApplicant(ctx context.Context, id, tenantId int64, status string) error {
	appColumns := twdao.YoubanTwoWayBotCooperationApplication.Columns()
	var app *entity.YoubanTwoWayBotCooperationApplication
	if err := twdao.YoubanTwoWayBotCooperationApplication.Ctx(ctx).Where(appColumns.Id, id).Where(appColumns.TenantId, tenantId).Scan(&app); err != nil || app == nil {
		return err
	}
	token, err := s.cooperationApplicantNotificationToken(ctx, app)
	if err != nil {
		return err
	}
	bot, err := s.telegramBot(ctx, token)
	if err != nil {
		return err
	}
	text := cooperationApplicantResultText(app, status)
	if text == "" {
		return nil
	}
	chatId, err := strconv.ParseInt(app.ApplicantTgUserId, 10, 64)
	if err != nil {
		return gerror.Wrap(err, "申请人Telegram ID无效")
	}
	_, err = bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: chatId, Text: text})
	return err
}

func (s *sSysTwoWayBot) cooperationApplicantNotificationToken(ctx context.Context, app *entity.YoubanTwoWayBotCooperationApplication) (string, error) {
	if app == nil || app.ConfigId <= 0 {
		return "", gerror.New("合作申请配置无效")
	}
	configColumns := twdao.YoubanTwoWayBotCooperationConfig.Columns()
	var config *entity.YoubanTwoWayBotCooperationConfig
	if err := twdao.YoubanTwoWayBotCooperationConfig.Ctx(ctx).Where(configColumns.Id, app.ConfigId).Where(configColumns.TenantId, app.TenantId).WhereNull(configColumns.DeletedAt).Scan(&config); err != nil {
		return "", err
	}
	if config == nil || config.Id <= 0 {
		return "", gerror.New("平台合作配置不存在")
	}
	if config.TwoWayBotId > 0 {
		row, err := s.botById(ctx, config.TwoWayBotId, app.TenantId)
		if err == nil && strings.TrimSpace(row.BotToken) != "" {
			return strings.TrimSpace(row.BotToken), nil
		}
	}
	var token string
	if err := g.DB().Model("hg_youban_publish_bot").Fields("bot_token").Where("id", config.BotId).Where("tenant_id", app.TenantId).WhereNull("deleted_at").Scan(&token); err != nil {
		return "", err
	}
	if token = strings.TrimSpace(token); token == "" {
		return "", gerror.New("未找到合作机器人的 Bot Token")
	}
	return token, nil
}

func cooperationApplicantResultText(app *entity.YoubanTwoWayBotCooperationApplication, status string) string {
	if app == nil {
		return ""
	}
	switch status {
	case sysin.CooperationJoinSuccess:
		return "合作申请已通过，机器人已加入采集频道。"
	case sysin.CooperationJoinPartialFailed:
		return "合作申请已通过，但部分频道添加失败：" + app.ErrorMessage + " ⚠️ 请联系客服"
	case sysin.CooperationJoinFailed:
		return "合作申请处理失败：" + app.ErrorMessage + " ⚠️ 请联系客服"
	default:
		return ""
	}
}
func cooperationTelegramError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToUpper(err.Error())
	switch {
	case strings.Contains(text, "ADMINS_TOO_MUCH"), strings.Contains(text, "BOTS_TOO_MUCH"):
		return "频道管理员或机器人数量已满"
	case strings.Contains(text, "CHAT_ADMIN_REQUIRED"):
		return "TG账号没有添加管理员权限"
	case strings.Contains(text, "USER_ALREADY_PARTICIPANT"):
		return "机器人已在频道中"
	case strings.Contains(text, "FLOOD_WAIT"):
		return "Telegram请求频繁，请稍后重试"
	default:
		return err.Error()
	}
}
