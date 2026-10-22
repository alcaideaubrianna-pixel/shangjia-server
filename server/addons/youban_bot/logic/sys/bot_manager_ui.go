package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/util/guid"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
	"hotgo/utility/sensitive"
)

var managerTokenPattern = regexp.MustCompile(`^[0-9]{5,}:[A-Za-z0-9_-]{20,}$`)
var managerEmbeddedTokenPattern = regexp.MustCompile(`[0-9]{5,}:[A-Za-z0-9_-]{20,}`)
var managerUsernamePattern = regexp.MustCompile(`@[A-Za-z0-9_]{5,32}`)
var managerUsernameLinkPattern = regexp.MustCompile(`(?i)(?:https?://)?t\.me/([A-Za-z0-9_]{5,32})`)

type managerInputCandidate struct {
	Value string
	Token bool
}

func managerCandidateLabel(c managerInputCandidate) string {
	if c.Token {
		return "Token（已隐藏）"
	}
	return "@" + strings.TrimPrefix(c.Value, "@")
}

// managerInputCandidates accepts a pasted message containing tokens and/or
// @usernames. It deliberately returns only bounded, de-duplicated candidates.
func managerInputCandidates(raw string) []managerInputCandidate {
	seen := map[string]bool{}
	result := make([]managerInputCandidate, 0, 20)
	for _, token := range managerEmbeddedTokenPattern.FindAllString(raw, -1) {
		if !managerTokenPattern.MatchString(token) || seen[token] {
			continue
		}
		seen[token] = true
		result = append(result, managerInputCandidate{Value: token, Token: true})
	}
	for _, username := range managerUsernamePattern.FindAllString(raw, -1) {
		username = strings.ToLower(strings.TrimPrefix(username, "@"))
		if seen[username] {
			continue
		}
		seen[username] = true
		result = append(result, managerInputCandidate{Value: username})
	}
	for _, match := range managerUsernameLinkPattern.FindAllStringSubmatch(raw, -1) {
		if len(match) < 2 {
			continue
		}
		username := strings.ToLower(match[1])
		if seen[username] {
			continue
		}
		seen[username] = true
		result = append(result, managerInputCandidate{Value: username})
	}
	if len(result) > 20 {
		return result[:20]
	}
	return result
}

type botManagerFeature struct{}

func (botManagerFeature) Key() string         { return "bot_management" }
func (botManagerFeature) Command() string     { return "bots" }
func (botManagerFeature) Description() string { return "机器人管理" }
func (botManagerFeature) Handle(ctx context.Context, s *sSysBot, fc *botFeatureContext) (bool, error) {
	if fc.Msg == nil || fc.Msg.From == nil || fc.Msg.Chat.Type != models.ChatTypePrivate {
		return true, nil
	}
	st, err := s.managerState(ctx, fc.BotId, fc.Msg.From.ID)
	if err != nil {
		return true, s.sendMessageOnly(ctx, fc.BotId, fmt.Sprint(fc.Msg.Chat.ID), err.Error())
	}
	if _, err := dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Data(do.YoubanBotManagerState{InputMode: "", Keyword: "", Revision: st.Revision + 1}).Update(); err != nil {
		return true, err
	}
	if st.SyncedAt == nil {
		if err := s.managerSyncFeedback(ctx, st, 0); err != nil {
			return true, s.managerError(ctx, st, 0, err)
		}
		return true, nil
	}
	return true, s.managerList(ctx, st, 0, 1)
}

type managerCommand struct {
	Action   string
	ID       int64
	Page     int
	Revision int64
}

func managerParseCommand(raw string) (managerCommand, error) {
	var c managerCommand
	p := strings.Split(raw, ":")
	if len(p) != 5 || p[0] != "bm" || len(raw) > 64 {
		return c, gerror.New("按钮已失效")
	}
	id, e1 := strconv.ParseInt(p[2], 10, 64)
	page, e2 := strconv.Atoi(p[3])
	rev, e3 := strconv.ParseInt(p[4], 10, 64)
	if e1 != nil || e2 != nil || e3 != nil || id < 0 || page < 1 || page > 100000 || rev < 0 {
		return c, gerror.New("按钮参数无效")
	}
	return managerCommand{p[1], id, page, rev}, nil
}
func managerButton(label, action string, id, revision int64) models.InlineKeyboardButton {
	return managerPageButton(label, action, id, 1, revision)
}

func managerSyncButtons() [][]models.InlineKeyboardButton {
	return [][]models.InlineKeyboardButton{{managerButton("刷新机器人列表", "list", 0, 0)}}
}
func managerPageButton(label, action string, id int64, page int, revision int64) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: label, CallbackData: fmt.Sprintf("bm:%s:%d:%d:%d", action, id, page, revision)}
}
func managerPager(action string, id int64, page, total int, rev int64) []models.InlineKeyboardButton {
	buttons := []models.InlineKeyboardButton{}
	if page > 1 {
		buttons = append(buttons, managerPageButton("上一页", action, id, page-1, rev))
	}
	if page*managerPageSize < total {
		buttons = append(buttons, managerPageButton("下一页", action, id, page+1, rev))
	}
	return buttons
}
func managerPage(page, total int) (int, int, int) {
	pages := (total + managerPageSize - 1) / managerPageSize
	if pages < 1 {
		pages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > pages {
		page = pages
	}
	start := (page - 1) * managerPageSize
	end := start + managerPageSize
	if end > total {
		end = total
	}
	return page, start, end
}

func (s *sSysBot) managerSend(ctx context.Context, entryBot, userID int64, messageID int, text string, buttons [][]models.InlineKeyboardButton) error {
	_, err := s.managerSendMessage(ctx, entryBot, userID, messageID, text, buttons)
	return err
}

func (s *sSysBot) managerSendMessage(ctx context.Context, entryBot, userID int64, messageID int, text string, buttons [][]models.InlineKeyboardButton) (int, error) {
	client, err := s.relayBot(ctx, s.replyBotToken(ctx, entryBot))
	if err != nil {
		return 0, err
	}
	call, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	markup := &models.InlineKeyboardMarkup{InlineKeyboard: buttons}
	if messageID > 0 {
		_, err = client.EditMessageText(call, &tgbot.EditMessageTextParams{ChatID: userID, MessageID: messageID, Text: sensitive.Tokens(text), ParseMode: models.ParseModeHTML, ReplyMarkup: markup})
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "message is not modified") {
			return messageID, nil
		}
		return messageID, err
	}
	message, err := client.SendMessage(call, &tgbot.SendMessageParams{ChatID: userID, Text: sensitive.Tokens(text), ParseMode: models.ParseModeHTML, ReplyMarkup: markup})
	if err != nil {
		return 0, err
	}
	return message.ID, nil
}
func (s *sSysBot) managerError(ctx context.Context, st *entity.YoubanBotManagerState, msgID int, err error) error {
	return s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, msgID, "操作未完成："+html.EscapeString(sensitive.Diagnostic(err.Error())), [][]models.InlineKeyboardButton{{managerButton("返回机器人列表", "list", 0, 0)}})
}

func (s *sSysBot) managerList(ctx context.Context, st *entity.YoubanBotManagerState, msgID, page int) error {
	c := dao.YoubanBotManaged.Columns()
	q := managerChannelBotsModel(ctx, st)
	if st.Keyword != "" {
		q = q.Where("("+c.BotName+" LIKE ? OR "+c.BotUsername+" LIKE ?)", "%"+st.Keyword+"%", "%"+strings.TrimPrefix(st.Keyword, "@")+"%")
	}
	total, err := q.Count()
	if err != nil {
		return err
	}
	page, _, _ = managerPage(page, total)
	var rows []*entity.YoubanBotManaged
	if err := q.OrderAsc(c.Id).Page(page, managerPageSize).Scan(&rows); err != nil {
		return err
	}
	text := fmt.Sprintf("<b>🤖 上架频道机器人管理</b> · 第 %d 页\n显示上架频道中的全部机器人，不限于自己创建的机器人。\n", page)
	if st.SyncedAt == nil {
		text += "尚未完成首次同步；同步完成会通知。\n"
	} else {
		text += "最近同步：" + st.SyncedAt.Format("m-d H:i") + "\n"
	}
	if st.Keyword != "" {
		text += "搜索：" + html.EscapeString(st.Keyword) + "\n"
	}
	buttons := [][]models.InlineKeyboardButton{}
	for _, b := range rows {
		label := firstNonEmpty(b.BotName, b.BotUsername)
		buttons = append(buttons, []models.InlineKeyboardButton{managerButton(label+" @"+b.BotUsername, "bot", b.Id, 0)})
	}
	if total == 0 {
		text += "暂无匹配机器人。可以添加上架 BOT，或同步频道中的机器人。\n"
	}
	if pager := managerPager("list", 0, page, total, 0); len(pager) > 0 {
		buttons = append(buttons, pager)
	}
	buttons = append(buttons, []models.InlineKeyboardButton{managerButton("➕ 添加上架 BOT", "add", 0, 0)}, []models.InlineKeyboardButton{managerButton("🔍 搜索机器人", "search", 0, 0), managerButton("🔄 同步机器人", "sync", 0, 0)}, []models.InlineKeyboardButton{managerButton("返回", "exit", 0, 0)})
	return s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, msgID, text, buttons)
}

func (s *sSysBot) handleBotManagerCallback(ctx context.Context, entryBot int64, q *models.CallbackQuery) (bool, error) {
	if q == nil || !strings.HasPrefix(q.Data, "bm:") {
		return false, nil
	}
	if q.Message.Message == nil || q.Message.Message.Chat.Type != models.ChatTypePrivate || q.Message.Message.Chat.ID != q.From.ID {
		return true, nil
	}
	client, err := s.relayBot(ctx, s.replyBotToken(ctx, entryBot))
	if err != nil {
		return true, err
	}
	_, _ = client.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: q.ID})
	st, err := s.managerState(ctx, entryBot, q.From.ID)
	if err != nil {
		return true, s.sendMessageOnly(ctx, entryBot, fmt.Sprint(q.From.ID), err.Error())
	}
	if !s.featureMenuVisible(ctx, (botManagerFeature{}).Key()) {
		return true, s.managerError(ctx, st, q.Message.Message.ID, gerror.New("机器人管理插件已停用"))
	}
	cmd, err := managerParseCommand(q.Data)
	if err != nil {
		return true, s.managerError(ctx, st, q.Message.Message.ID, err)
	}
	err = s.managerCallback(ctx, st, q.Message.Message.ID, cmd)
	if err != nil {
		return true, s.managerError(ctx, st, q.Message.Message.ID, err)
	}
	return true, nil
}

func (s *sSysBot) managerCallback(ctx context.Context, st *entity.YoubanBotManagerState, msgID int, c managerCommand) error {
	switch c.Action {
	case "list":
		return s.managerList(ctx, st, msgID, c.Page)
	case "exit":
		_, err := dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Data(do.YoubanBotManagerState{InputMode: "", DraftBotId: 0, Keyword: "", Revision: st.Revision + 1}).Update()
		if err != nil {
			return err
		}
		return s.replyWithMarkup(ctx, st.EntryBotId, fmt.Sprint(st.TelegramUserId), "已返回插件菜单。", s.replyKeyboard(context.WithValue(ctx, telegramUserIdCtxKey{}, fmt.Sprint(st.TelegramUserId))))
	case "sync":
		return s.managerSyncFeedback(ctx, st, msgID)
	case "tasks":
		// Old messages may still contain this removed entry point.
		return s.managerList(ctx, st, msgID, 1)
	case "retry":
		id, err := managerRetry(ctx, st, c.ID)
		if err != nil {
			return err
		}
		return s.managerSubmitted(ctx, st, msgID, id)
	case "search", "token", "username":
		text := "请输入机器人名称或 @用户名，搜索已同步的机器人。"
		if c.Action == "token" {
			text = "可以粘贴 Bot Token、@机器人用户名，或混合粘贴多条内容（例如：给 @foo_bot 添加 123456:AA...）。系统会自动提取、去重并逐个校验；Token 只用于验证和保存，收到后会尝试删除这条敏感消息。"
		}
		if c.Action == "username" {
			text = "请输入机器人用户名，支持 @foo_bot、t.me/foo_bot 或 https://t.me/foo_bot，也可以一次粘贴多个。系统会自动提取、去重并由协议号确认是否为真实机器人。"
		}
		if _, err := dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Data(do.YoubanBotManagerState{InputMode: c.Action, Revision: st.Revision + 1}).Update(); err != nil {
			return err
		}
		return s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, msgID, text, [][]models.InlineKeyboardButton{{managerButton("取消", "cancel", 0, 0)}})
	case "cancel":
		_, err := dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Data(do.YoubanBotManagerState{InputMode: "", DraftBotId: 0, Revision: st.Revision + 1}).Update()
		if err != nil {
			return err
		}
		return s.managerList(ctx, st, msgID, 1)
	case "add":
		return s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, msgID, "选择添加方式。批量任务会把每个机器人分配到一个候选上架频道。", [][]models.InlineKeyboardButton{{managerButton("添加机器人用户名", "username", 0, 0)}, {managerButton("添加机器人 Token", "token", 0, 0)}, {managerButton("选择已有机器人", "choose", 0, 0)}, {managerButton("返回", "list", 0, 0)}})
	case "choose":
		return s.managerChoose(ctx, st, msgID, c.Page)
	case "bot":
		b, err := managerBot(ctx, st, c.ID)
		if err != nil {
			return err
		}
		if _, err := dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Data(do.YoubanBotManagerState{DraftBotId: b.Id, InputMode: "", Revision: st.Revision + 1}).Update(); err != nil {
			return err
		}
		st.DraftBotId = b.Id
		st.Revision++
		return s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, msgID, "<b>🤖 "+html.EscapeString(b.BotName)+"</b>\n@"+html.EscapeString(b.BotUsername)+"\n可以查看所在频道、修改权限或移出频道，无需提供 Token。", [][]models.InlineKeyboardButton{{managerButton("所在频道", "members", b.Id, 0)}, {managerButton("添加到频道", "select", b.Id, st.Revision)}, {managerButton("返回机器人列表", "list", 0, 0)}})
	case "select", "toggle", "all", "none":
		return s.managerSelection(ctx, st, msgID, c)
	case "confirm":
		if c.Revision != st.Revision {
			return gerror.New("此确认按钮已过期，请重新选择")
		}
		channels, err := managerChannels(ctx, st)
		if err != nil {
			return err
		}
		selected := managerSelected(st, channels)
		if len(selected) == 0 {
			return gerror.New("请至少选择一个频道")
		}
		return s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, msgID, fmt.Sprintf("已选择 %d 个候选频道。\n机器人会公平分配到其中一个频道，优先机器人较少、管理员名额较多的频道。\n默认授予发布、编辑、删除权限。", len(selected)), [][]models.InlineKeyboardButton{{managerButton("确认添加", "allocate", st.DraftBotId, st.Revision)}, {managerButton("重新选择", "select", st.DraftBotId, st.Revision)}})
	case "allocate":
		id, err := s.managerAllocate(ctx, st, c.Revision)
		if err != nil {
			return err
		}
		return s.managerSubmitted(ctx, st, msgID, id)
	case "members":
		return s.managerMembers(ctx, st, msgID, c.ID, c.Page)
	case "member", "rights", "flip", "saveRights", "remove", "doRemove":
		return s.managerMemberAction(ctx, st, msgID, c)
	case "delete", "doDelete":
		return gerror.New("已移除删除上架配置入口；需要移出机器人，请从所在频道操作")
	default:
		return gerror.New("未识别的按钮，请重新打开机器人管理")
	}
}

func (s *sSysBot) managerChoose(ctx context.Context, st *entity.YoubanBotManagerState, msgID, page int) error {
	c := dao.YoubanBotManaged.Columns()
	q := dao.YoubanBotManaged.Ctx(ctx).Where(c.TenantId, st.TenantId).Where(c.AccountId, st.AccountId).WhereNot(c.BotToken, "")
	total, err := q.Count()
	if err != nil {
		return err
	}
	page, _, _ = managerPage(page, total)
	var rows []*entity.YoubanBotManaged
	if err := q.OrderAsc(c.Id).Page(page, managerPageSize).Scan(&rows); err != nil {
		return err
	}
	buttons := [][]models.InlineKeyboardButton{}
	for _, b := range rows {
		buttons = append(buttons, []models.InlineKeyboardButton{managerButton("@"+b.BotUsername, "bot", b.Id, 0)})
	}
	if p := managerPager("choose", 0, page, total, 0); len(p) > 0 {
		buttons = append(buttons, p)
	}
	buttons = append(buttons, []models.InlineKeyboardButton{managerButton("填写Token", "token", 0, 0), managerButton("返回", "list", 0, 0)})
	return s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, msgID, "请选择已配置的上架机器人：", buttons)
}

func (s *sSysBot) managerSelection(ctx context.Context, st *entity.YoubanBotManagerState, msgID int, c managerCommand) error {
	if c.Revision != st.Revision || st.DraftBotId <= 0 {
		return gerror.New("选择已失效，请重新打开机器人详情")
	}
	channels, err := managerChannels(ctx, st)
	if err != nil {
		return err
	}
	ids := managerSelected(st, channels)
	selected := map[int64]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	if c.Action == "toggle" {
		found := false
		for _, ch := range channels {
			if ch.Id == c.ID && ch.SyncStatus != "unavailable" {
				found = true
			}
		}
		if !found {
			return gerror.New("频道不可用")
		}
		selected[c.ID] = !selected[c.ID]
	}
	if c.Action == "all" || c.Action == "none" {
		for _, ch := range channels {
			selected[ch.Id] = c.Action == "all" && ch.SyncStatus != "unavailable"
		}
	}
	ids = []int64{}
	for _, ch := range channels {
		if selected[ch.Id] {
			ids = append(ids, ch.Id)
		}
	}
	if c.Action != "select" {
		res, err := dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Where(dao.YoubanBotManagerState.Columns().Revision, c.Revision).Data(do.YoubanBotManagerState{SelectedChannelIds: managerJSON(ids), SelectionSaved: 1, Revision: st.Revision + 1}).Update()
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return gerror.New("选择已更新，请使用最新按钮")
		}
		st.Revision++
	}
	page, start, end := managerPage(c.Page, len(channels))
	buttons := [][]models.InlineKeyboardButton{}
	for _, ch := range channels[start:end] {
		mark := "⬜"
		if selected[ch.Id] {
			mark = "✅"
		}
		limit := "未知"
		if ch.AdminLimit > 0 {
			limit = strconv.Itoa(ch.AdminLimit)
		}
		count := "未知"
		if ch.AdminCount >= 0 {
			count = strconv.Itoa(ch.AdminCount)
		}
		label := fmt.Sprintf("%s %s · 管理员 %s/%s", mark, ch.Title, count, limit)
		if ch.SyncStatus != "success" || ch.CanManage != 1 {
			label += " ⚠"
		}
		buttons = append(buttons, []models.InlineKeyboardButton{managerPageButton(label, "toggle", ch.Id, page, st.Revision)})
	}
	if p := managerPager("select", st.DraftBotId, page, len(channels), st.Revision); len(p) > 0 {
		buttons = append(buttons, p)
	}
	buttons = append(buttons, []models.InlineKeyboardButton{managerPageButton("全部选择", "all", 0, page, st.Revision), managerPageButton("全部取消", "none", 0, page, st.Revision)}, []models.InlineKeyboardButton{managerButton(fmt.Sprintf("确定（%d个频道）", len(ids)), "confirm", st.DraftBotId, st.Revision)}, []models.InlineKeyboardButton{managerButton("返回", "bot", st.DraftBotId, 0)})
	return s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, msgID, fmt.Sprintf("<b>选择候选上架频道</b> · 第 %d 页\n已选 %d 个，跨页保留。首次默认全选，以后沿用上次选择。\n每个机器人只分配一个频道；⚠ 表示需同步或权限异常，分配时会跳过。", page, len(ids)), buttons)
}

func (s *sSysBot) managerAddToken(ctx context.Context, st *entity.YoubanBotManagerState, token string) (*entity.YoubanBotManaged, error) {
	client, err := s.relayBot(ctx, token)
	if err != nil {
		return nil, gerror.New("Token格式或机器人连接异常")
	}
	call, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	identity, err := client.GetMe(call)
	if err != nil || identity.ID <= 0 || !identity.IsBot || !strings.HasPrefix(token, strconv.FormatInt(identity.ID, 10)+":") {
		return nil, gerror.New("机器人Token校验失败，请检查后重试")
	}
	if err := ps.SysPublish().BotManagerValidateToken(ctx, st.TenantId, st.AccountId, token); err != nil {
		return nil, err
	}
	mainCols := dao.YoubanBotBot.Columns()
	mainCount, err := dao.YoubanBotBot.Ctx(ctx).WhereLike(mainCols.BotToken, strconv.FormatInt(identity.ID, 10)+":%").Count()
	if err != nil {
		return nil, err
	}
	if mainCount > 0 {
		return nil, gerror.New("不能将平台主机器人添加为上架机器人")
	}
	bc := dao.YoubanBotManaged.Columns()
	n, err := dao.YoubanBotManaged.Ctx(ctx).Where(bc.TelegramBotId, identity.ID).WhereNot(bc.BotToken, "").Where("("+bc.TenantId+"<>? OR "+bc.AccountId+"<>?)", st.TenantId, st.AccountId).Count()
	if err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, gerror.New("该机器人已属于其他账户，请联系管理员核对")
	}
	err = dao.YoubanBotManaged.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		publishBotId, err := ps.SysPublish().BotManagerUpdateCredentials(ctx, &pi.BotManagerProjection{TenantId: st.TenantId, AccountId: st.AccountId, BotToken: token, BotName: telegramBotDisplayName(identity), BotUsername: identity.Username})
		if err != nil {
			return err
		}
		if _, err := dao.YoubanBotManaged.Ctx(ctx).Data(do.YoubanBotManaged{TenantId: st.TenantId, AccountId: st.AccountId, TelegramBotId: identity.ID, BotName: telegramBotDisplayName(identity), BotUsername: identity.Username, BotToken: token, PublishBotId: publishBotId, Status: 1}).OnConflict(bc.TenantId, bc.AccountId, bc.TelegramBotId).OnDuplicate(bc.BotName, bc.BotUsername, bc.BotToken, bc.PublishBotId).Save(); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE "+dao.YoubanBotManaged.Table()+" SET "+bc.DeletedAt+"=NULL WHERE "+bc.TenantId+"=? AND "+bc.AccountId+"=? AND "+bc.TelegramBotId+"=?", st.TenantId, st.AccountId, identity.ID)
		return err
	})
	if err != nil {
		return nil, gerror.New("保存机器人配置失败，请联系管理员检查数据库")
	}
	ps.SysPublish().BotManagerRefreshCredentials(ctx)
	var b entity.YoubanBotManaged
	if err := dao.YoubanBotManaged.Ctx(ctx).Where(bc.TenantId, st.TenantId).Where(bc.AccountId, st.AccountId).Where(bc.TelegramBotId, identity.ID).Scan(&b); err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *sSysBot) managerAddExisting(ctx context.Context, st *entity.YoubanBotManagerState, username string) (*entity.YoubanBotManaged, error) {
	c := dao.YoubanBotManaged.Columns()
	var b *entity.YoubanBotManaged
	if err := dao.YoubanBotManaged.Ctx(ctx).Where(c.TenantId, st.TenantId).Where(c.AccountId, st.AccountId).Where(c.BotUsername, strings.TrimPrefix(username, "@")).Scan(&b); err != nil {
		return nil, err
	}
	if b == nil {
		// Keep an identity-only row. The account worker resolves the username,
		// verifies User.Bot, and fills TelegramBotId before applying the task.
		id, err := dao.YoubanBotManaged.Ctx(ctx).Data(do.YoubanBotManaged{TenantId: st.TenantId, AccountId: st.AccountId, BotUsername: strings.TrimPrefix(username, "@"), BotName: strings.TrimPrefix(username, "@"), Status: 1}).InsertAndGetId()
		if err != nil {
			return nil, err
		}
		b = &entity.YoubanBotManaged{Id: id, TenantId: st.TenantId, AccountId: st.AccountId, BotUsername: strings.TrimPrefix(username, "@"), BotName: strings.TrimPrefix(username, "@"), Status: 1}
	}
	return b, nil
}

func (s *sSysBot) handleBotManagerInput(ctx context.Context, entryBot int64, msg *models.Message) (bool, error) {
	if msg == nil || msg.From == nil || msg.Chat.Type != models.ChatTypePrivate {
		return false, nil
	}
	text := strings.TrimSpace(firstNonEmpty(msg.Text, msg.Caption))
	token := managerEmbeddedTokenPattern.MatchString(text)
	if token {
		if client, err := s.relayBot(ctx, s.replyBotToken(ctx, entryBot)); err == nil {
			_, _ = client.DeleteMessage(ctx, &tgbot.DeleteMessageParams{ChatID: msg.Chat.ID, MessageID: msg.ID})
		}
	}
	// Tokens bypass all ordinary message logging, storage and feature handlers.
	var st *entity.YoubanBotManagerState
	c := dao.YoubanBotManagerState.Columns()
	if err := dao.YoubanBotManagerState.Ctx(ctx).Where(c.EntryBotId, entryBot).Where(c.TelegramUserId, msg.From.ID).Scan(&st); err != nil {
		if !token {
			return false, nil
		}
		return true, s.sendMessageOnly(ctx, entryBot, fmt.Sprint(msg.Chat.ID), "请先打开机器人管理，再添加 Token。")
	}
	if !token && st != nil && st.InputMode != "" {
		leaving := strings.HasPrefix(text, "/")
		for _, feature := range botFeatures {
			if feature != nil && featureLabelMatches(text, s.replyKeyboardLabel(ctx, feature), feature.Description(), feature.Command()) {
				leaving = true
				break
			}
		}
		if leaving {
			_, err := dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Data(do.YoubanBotManagerState{InputMode: ""}).Update()
			return false, err
		}
	}
	if !token && (st == nil || st.InputMode == "" || strings.HasPrefix(text, "/")) {
		return false, nil
	}
	if st == nil {
		return true, s.sendMessageOnly(ctx, entryBot, fmt.Sprint(msg.Chat.ID), "请先打开机器人管理，再添加 Token。")
	}
	if st.InputMode != "" && st.UpdatedAt != nil && time.Since(st.UpdatedAt.Time) > 10*time.Minute {
		_, _ = dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Data(do.YoubanBotManagerState{InputMode: ""}).Update()
		if !token {
			return false, nil
		}
		return true, s.managerError(ctx, st, 0, gerror.New("输入已超时，请重新点击添加机器人"))
	}
	current, err := s.managerState(ctx, entryBot, msg.From.ID)
	if err != nil {
		return true, s.sendMessageOnly(ctx, entryBot, fmt.Sprint(msg.Chat.ID), err.Error())
	}
	if current.AccountId != st.AccountId || current.TenantId != st.TenantId || !s.featureMenuVisible(ctx, (botManagerFeature{}).Key()) {
		return true, s.managerError(ctx, current, 0, gerror.New("绑定或插件状态已变化，请重新打开机器人管理"))
	}
	st = current
	if st.InputMode == "search" && !token {
		if len([]rune(text)) > 64 {
			return true, s.managerError(ctx, st, 0, gerror.New("搜索内容最多64个字"))
		}
		_, err = dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Data(do.YoubanBotManagerState{Keyword: text, InputMode: ""}).Update()
		if err != nil {
			return true, err
		}
		st.Keyword = text
		return true, s.managerList(ctx, st, 0, 1)
	}
	if st.InputMode != "token" && st.InputMode != "username" {
		return true, s.managerError(ctx, st, 0, gerror.New("请点击添加上架BOT后发送Token、@用户名或混合文本"))
	}
	candidates := managerInputCandidates(text)
	if st.InputMode == "username" {
		filtered := candidates[:0]
		for _, candidate := range candidates {
			if !candidate.Token {
				filtered = append(filtered, candidate)
			}
		}
		candidates = filtered
	}
	if len(candidates) == 0 {
		return true, s.managerError(ctx, st, 0, gerror.New("没有识别到有效的Token或@机器人用户名"))
	}
	results := make([]string, 0, len(candidates))
	batchKey := guid.S()
	for _, candidate := range candidates {
		var b *entity.YoubanBotManaged
		var addErr error
		if candidate.Token {
			b, addErr = s.managerAddToken(ctx, st, candidate.Value)
		} else {
			b, addErr = s.managerAddExisting(ctx, st, candidate.Value)
		}
		if addErr != nil {
			results = append(results, "❌ "+managerCandidateLabel(candidate)+"："+sensitive.Diagnostic(addErr.Error()))
			continue
		}
		// Reuse the existing transactional allocator. Each candidate gets one
		// durable attach task and reservations account for earlier candidates.
		if _, err := dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Data(do.YoubanBotManagerState{DraftBotId: b.Id, InputMode: "", Revision: st.Revision + 1}).Update(); err != nil {
			return true, err
		}
		st.DraftBotId, st.Revision = b.Id, st.Revision+1
		taskID, allocErr := s.managerAllocate(ctx, st, st.Revision, batchKey)
		if allocErr != nil {
			results = append(results, "❌ @"+b.BotUsername+"："+sensitive.Diagnostic(allocErr.Error()))
			continue
		}
		results = append(results, fmt.Sprintf("✅ @%s：任务 #%d 已提交，完成后显示分配频道", b.BotUsername, taskID))
	}
	_, _ = dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Data(do.YoubanBotManagerState{InputMode: "", DraftBotId: 0}).Update()
	return true, s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, 0, "<b>批量添加任务已提交</b>\n"+strings.Join(results, "\n")+"\n\n任务完成后会逐项通知机器人最终分配的频道。", managerSyncButtons())
}

func (s *sSysBot) managerSubmitted(ctx context.Context, st *entity.YoubanBotManagerState, msgID int, taskID int64) error {
	return s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, msgID, fmt.Sprintf("任务 #%d 已提交，完成后通知你。可以返回继续操作。", taskID), managerSyncButtons())
}

func (s *sSysBot) managerTasks(ctx context.Context, st *entity.YoubanBotManagerState, msgID, page int) error {
	c := dao.YoubanBotManagerTask.Columns()
	q := dao.YoubanBotManagerTask.Ctx(ctx).Where(c.TenantId, st.TenantId).Where(c.AccountId, st.AccountId).Where(c.EntryBotId, st.EntryBotId).Where(c.TelegramUserId, st.TelegramUserId)
	total, err := q.Count()
	if err != nil {
		return err
	}
	page, _, _ = managerPage(page, total)
	var rows []*entity.YoubanBotManagerTask
	if err := q.OrderDesc(c.Id).Page(page, managerPageSize).Scan(&rows); err != nil {
		return err
	}
	text := "<b>机器人管理任务</b>\n"
	buttons := [][]models.InlineKeyboardButton{}
	for _, t := range rows {
		status := map[string]string{"pending": "待调度", "submitted": "处理中", "success": "成功", "failed": "失败"}[t.Status]
		text += fmt.Sprintf("#%d · %s · 频道#%d · %s\n", t.Id, html.EscapeString(t.Action), t.ChannelId, status)
		if t.ErrorMessage != "" {
			text += html.EscapeString(sensitive.Diagnostic(t.ErrorMessage)) + "\n"
		}
		if t.Status == "failed" && t.Action != "sync" {
			buttons = append(buttons, []models.InlineKeyboardButton{managerButton(fmt.Sprintf("重试任务 #%d", t.Id), "retry", t.Id, 0)})
		}
	}
	if p := managerPager("tasks", 0, page, total, 0); len(p) > 0 {
		buttons = append(buttons, p)
	}
	buttons = append(buttons, []models.InlineKeyboardButton{managerPageButton("刷新", "tasks", 0, page, 0), managerButton("返回列表", "list", 0, 0)})
	return s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, msgID, text, buttons)
}

func (s *sSysBot) managerMembers(ctx context.Context, st *entity.YoubanBotManagerState, msgID int, botID int64, page int) error {
	b, err := managerBot(ctx, st, botID)
	if err != nil {
		return err
	}
	c := dao.YoubanBotManagedMember.Columns()
	q := dao.YoubanBotManagedMember.Ctx(ctx).Where(c.ManagedBotId, b.Id).Where(c.TenantId, st.TenantId).Where(c.AccountId, st.AccountId).WhereIn(c.MemberStatus, []string{"admin", "member"})
	total, err := q.Count()
	if err != nil {
		return err
	}
	if total == 0 {
		channels, err := managerChannels(ctx, st)
		if err != nil {
			return err
		}
		needsSync := len(channels) == 0
		for _, ch := range channels {
			if ch.SyncStatus == "unavailable" {
				continue
			}
			if ch.LastSyncedAt == nil || ch.SyncStatus != "success" || (b.CreatedAt != nil && ch.LastSyncedAt.Before(b.CreatedAt)) {
				needsSync = true
			}
		}
		if needsSync {
			return s.managerSyncFeedback(ctx, st, msgID)
		}
	}
	page, _, _ = managerPage(page, total)
	var rows []*entity.YoubanBotManagedMember
	if err := q.OrderAsc(c.Id).Page(page, managerPageSize).Scan(&rows); err != nil {
		return err
	}
	buttons := [][]models.InlineKeyboardButton{}
	for _, m := range rows {
		var ch entity.YoubanBotManagedChannel
		if err := dao.YoubanBotManagedChannel.Ctx(ctx).WherePri(m.ChannelId).Scan(&ch); err != nil {
			return err
		}
		status := map[string]string{"admin": "管理员", "member": "普通成员", "removed": "已移出", "unknown": "未知"}[m.MemberStatus]
		buttons = append(buttons, []models.InlineKeyboardButton{managerButton(ch.Title+" · "+status, "member", m.Id, 0)})
	}
	if p := managerPager("members", botID, page, total, 0); len(p) > 0 {
		buttons = append(buttons, p)
	}
	buttons = append(buttons, []models.InlineKeyboardButton{managerButton("重新同步频道", "sync", 0, 0)}, []models.InlineKeyboardButton{managerButton("返回机器人详情", "bot", botID, 0)})
	return s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, msgID, "@"+html.EscapeString(b.BotUsername)+" 的频道关系（以最近成功同步为准）", buttons)
}

func (s *sSysBot) managerMemberAction(ctx context.Context, st *entity.YoubanBotManagerState, msgID int, c managerCommand) error {
	mc := dao.YoubanBotManagedMember.Columns()
	var m *entity.YoubanBotManagedMember
	if err := dao.YoubanBotManagedMember.Ctx(ctx).WherePri(c.ID).Where(mc.TenantId, st.TenantId).Where(mc.AccountId, st.AccountId).Scan(&m); err != nil {
		return err
	}
	if m == nil {
		return gerror.New("频道关系不存在或无权管理")
	}
	var ch entity.YoubanBotManagedChannel
	if err := dao.YoubanBotManagedChannel.Ctx(ctx).WherePri(m.ChannelId).Scan(&ch); err != nil {
		return err
	}
	if _, err := managerBot(ctx, st, m.ManagedBotId); err != nil {
		return err
	}
	if c.Action == "doRemove" || c.Action == "saveRights" {
		if c.Revision != st.Revision || st.DraftBotId != m.ManagedBotId {
			return gerror.New("操作确认已失效，请重新打开频道详情")
		}
		action := "rights"
		r := st.DraftRightsJson
		if c.Action == "doRemove" {
			action = "remove"
			r = "{}"
		}
		if action == "rights" {
			var rights managerRights
			if err := json.Unmarshal([]byte(r), &rights); err != nil {
				return gerror.New("权限编辑已失效")
			}
		}
		var id int64
		err := dao.YoubanBotManagerState.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			res, err := dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Where(dao.YoubanBotManagerState.Columns().Revision, c.Revision).Data(do.YoubanBotManagerState{Revision: st.Revision + 1, DraftRightsJson: "{}"}).Update()
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				return gerror.New("操作已提交，请勿重复点击")
			}
			id, err = managerNewTask(ctx, st, guid.S(), action, m.ManagedBotId, ch.Id, r)
			return err
		})
		if err != nil {
			return err
		}
		return s.managerSubmitted(ctx, st, msgID, id)
	}
	if c.Action == "remove" {
		_, err := dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Data(do.YoubanBotManagerState{DraftBotId: m.ManagedBotId, Revision: st.Revision + 1}).Update()
		if err != nil {
			return err
		}
		return s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, msgID, "确认将机器人移出「"+html.EscapeString(ch.Title)+"」？\n会取消它的上架绑定。有未完成发布任务时将拒绝移除。", [][]models.InlineKeyboardButton{{managerButton("确认移出", "doRemove", m.Id, st.Revision+1), managerButton("取消", "member", m.Id, 0)}})
	}
	var r managerRights
	_ = json.Unmarshal([]byte(m.RightsJson), &r)
	if c.Action == "rights" || c.Action == "flip" {
		if c.Action == "flip" {
			if c.Revision != st.Revision {
				return gerror.New("按钮已过期")
			}
			_ = json.Unmarshal([]byte(st.DraftRightsJson), &r)
			switch c.Page {
			case 1:
				r.Post = !r.Post
			case 2:
				r.Edit = !r.Edit
			case 3:
				r.Delete = !r.Delete
			}
		}
		res, err := dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Where(dao.YoubanBotManagerState.Columns().Revision, st.Revision).Data(do.YoubanBotManagerState{DraftBotId: m.ManagedBotId, DraftRightsJson: managerJSON(r), Revision: st.Revision + 1}).Update()
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return gerror.New("权限已更新，请使用最新按钮")
		}
		rev := st.Revision + 1
		mark := func(v bool) string {
			if v {
				return "✅ "
			}
			return "⬜ "
		}
		return s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, msgID, "修改「"+html.EscapeString(ch.Title)+"」的机器人权限。取消发布/删除可能影响上架和清理，请确认。", [][]models.InlineKeyboardButton{{managerPageButton(mark(r.Post)+"发布消息", "flip", m.Id, 1, rev)}, {managerPageButton(mark(r.Edit)+"编辑消息", "flip", m.Id, 2, rev)}, {managerPageButton(mark(r.Delete)+"删除消息", "flip", m.Id, 3, rev)}, {managerButton("保存修改", "saveRights", m.Id, rev), managerButton("取消", "member", m.Id, 0)}})
	}
	buttons := [][]models.InlineKeyboardButton{}
	if strings.HasPrefix(ch.VisitUrl, "https://t.me/") {
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: "打开频道", URL: ch.VisitUrl}})
	}
	buttons = append(buttons, []models.InlineKeyboardButton{managerButton("修改权限", "rights", m.Id, 0), managerButton("移出频道", "remove", m.Id, 0)}, []models.InlineKeyboardButton{managerButton("返回所在频道", "members", m.ManagedBotId, 0)})
	return s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, msgID, fmt.Sprintf("<b>%s</b>\n成员状态：%s\n发布：%t · 编辑：%t · 删除：%t\n检测状态：%s", html.EscapeString(ch.Title), html.EscapeString(m.MemberStatus), r.Post, r.Edit, r.Delete, html.EscapeString(ch.SyncStatus)), buttons)
}

func (s *sSysBot) managerDelete(ctx context.Context, st *entity.YoubanBotManagerState, msgID int, c managerCommand) error {
	b, err := managerBot(ctx, st, c.ID)
	if err != nil {
		return err
	}
	if c.Action == "delete" {
		return s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, msgID, "删除上架配置不会自动移出频道。存在频道关系或旧上架配置时，请先解除引用。", [][]models.InlineKeyboardButton{{managerButton("确认删除", "doDelete", b.Id, st.Revision), managerButton("取消", "bot", b.Id, 0)}})
	}
	if c.Revision != st.Revision {
		return gerror.New("确认按钮已过期")
	}
	err = dao.YoubanBotManaged.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		mc := dao.YoubanBotManagedMember.Columns()
		n, err := dao.YoubanBotManagedMember.Ctx(ctx).Where(mc.ManagedBotId, b.Id).WhereNot(mc.MemberStatus, "removed").Count()
		if err != nil {
			return err
		}
		if n > 0 {
			return gerror.New("机器人仍在频道中，请先移出频道")
		}
		tc := dao.YoubanBotManagerTask.Columns()
		active, err := dao.YoubanBotManagerTask.Ctx(ctx).Where(tc.ManagedBotId, b.Id).WhereIn(tc.Status, []string{"pending", "submitted"}).Count()
		if err != nil {
			return err
		}
		if active > 0 {
			return gerror.New("机器人仍有管理任务，请等待完成")
		}
		if err := ps.SysPublish().BotManagerDelete(ctx, st.TenantId, st.AccountId, b.PublishBotId); err != nil {
			return err
		}
		if _, err := dao.YoubanBotManaged.Ctx(ctx).WherePri(b.Id).Data(do.YoubanBotManaged{BotToken: "", PublishBotId: 0}).Update(); err != nil {
			return err
		}
		_, err = dao.YoubanBotManaged.Ctx(ctx).WherePri(b.Id).Delete()
		return err
	})
	if err != nil {
		return err
	}
	return s.managerList(ctx, st, msgID, 1)
}
