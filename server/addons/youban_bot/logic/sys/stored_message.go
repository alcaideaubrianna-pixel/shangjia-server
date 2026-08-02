package sys

import (
	"context"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_bot/model/input/sysin"
)

type storedTelegramMessageRecord struct {
	Id        int64  `json:"id"`
	BotId     int64  `json:"botId"`
	ChatId    string `json:"chatId"`
	MessageId int    `json:"messageId"`
}

func (s *sSysBot) telegramMessageRecordId(ctx context.Context, botId int64, chatId string, messageId int) (int64, error) {
	if botId <= 0 || strings.TrimSpace(chatId) == "" || messageId <= 0 {
		return 0, nil
	}
	var row storedTelegramMessageRecord
	if err := g.DB().Model(messageTable).Safe().Ctx(ctx).
		Fields("id").
		Where("bot_id", botId).
		Where("chat_id", strings.TrimSpace(chatId)).
		Where("message_id", messageId).
		Scan(&row); err != nil {
		return 0, gerror.Wrap(err, "读取Telegram原消息记录失败")
	}
	return row.Id, nil
}

func (s *sSysBot) CopyStoredMessages(ctx context.Context, in *sysin.StoredMessageCopyInp) (*sysin.StoredMessageCopyModel, error) {
	res := &sysin.StoredMessageCopyModel{MessageIds: []int64{}}
	if in == nil || len(in.MessageRecordIds) == 0 {
		return res, nil
	}
	targetChatId := strings.TrimSpace(in.TargetChatId)
	if targetChatId == "" {
		return nil, gerror.New("目标Chat ID不能为空")
	}
	source, err := s.StoredMessageSource(ctx, in.MessageRecordIds)
	if err != nil {
		return nil, err
	}
	if source == nil || len(source.MessageIds) == 0 {
		return nil, gerror.New("Telegram原消息不存在")
	}
	row, err := s.botById(ctx, source.BotId)
	if err != nil {
		return nil, err
	}
	bot, err := s.telegramBot(ctx, row.BotToken)
	if err != nil {
		return nil, err
	}
	callCtx, cancel := telegramAPICtx()
	defer cancel()
	if len(source.MessageIds) == 1 {
		message, err := bot.CopyMessage(callCtx, &tgbot.CopyMessageParams{ChatID: targetChatId, FromChatID: source.ChatId, MessageID: source.MessageIds[0]})
		if err != nil {
			return nil, err
		}
		if message == nil || message.ID <= 0 {
			return nil, gerror.New("复制Telegram原消息未返回消息ID")
		}
		res.MessageIds = append(res.MessageIds, int64(message.ID))
		return res, nil
	}
	copied, err := bot.CopyMessages(callCtx, &tgbot.CopyMessagesParams{ChatID: targetChatId, FromChatID: source.ChatId, MessageIDs: source.MessageIds})
	if err != nil {
		return nil, err
	}
	for _, message := range copied {
		if message.ID > 0 {
			res.MessageIds = append(res.MessageIds, int64(message.ID))
		}
	}
	if len(res.MessageIds) != len(source.MessageIds) {
		return nil, gerror.Newf("复制Telegram原消息数量不完整，期望:%d 实际:%d", len(source.MessageIds), len(res.MessageIds))
	}
	return res, nil
}

func (s *sSysBot) StoredMessageSource(ctx context.Context, ids []int64) (*sysin.StoredMessageSourceModel, error) {
	ids = uniqueStoredMessageIds(ids)
	if len(ids) == 0 {
		return &sysin.StoredMessageSourceModel{MessageIds: []int{}}, nil
	}
	var records []*storedTelegramMessageRecord
	if err := g.DB().Model(messageTable).Safe().Ctx(ctx).WhereIn("id", ids).Scan(&records); err != nil {
		return nil, gerror.Wrap(err, "读取Telegram原消息失败")
	}
	if len(records) != len(ids) {
		return nil, gerror.New("部分Telegram原消息已不存在")
	}
	byId := make(map[int64]*storedTelegramMessageRecord, len(records))
	for _, record := range records {
		if record != nil {
			byId[record.Id] = record
		}
	}
	ordered := make([]*storedTelegramMessageRecord, 0, len(ids))
	for _, id := range ids {
		record := byId[id]
		if record == nil {
			return nil, gerror.New("Telegram原消息不存在")
		}
		ordered = append(ordered, record)
	}
	botId := ordered[0].BotId
	chatId := strings.TrimSpace(ordered[0].ChatId)
	messageIds := make([]int, 0, len(ordered))
	for _, record := range ordered {
		if record.BotId != botId || strings.TrimSpace(record.ChatId) != chatId {
			return nil, gerror.New("Telegram原消息不属于同一会话")
		}
		messageIds = append(messageIds, record.MessageId)
	}
	row, err := s.botById(ctx, botId)
	if err != nil {
		return nil, err
	}
	return &sysin.StoredMessageSourceModel{BotId: botId, BotUsername: strings.TrimPrefix(strings.TrimSpace(row.BotUsername), "@"), ChatId: chatId, MessageIds: messageIds}, nil
}

func uniqueStoredMessageIds(ids []int64) []int64 {
	result := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func (s *sSysBot) RetainStoredMessages(ctx context.Context, ids []int64) error {
	ids = uniqueStoredMessageIds(ids)
	if len(ids) == 0 {
		return nil
	}
	_, err := g.DB().Model(messageTable).Safe().Ctx(ctx).WhereIn("id", ids).Data(g.Map{"retained_at": gtime.Now()}).Update()
	return gerror.Wrap(err, "保留Telegram原消息失败")
}

func (s *sSysBot) ReleaseStoredMessages(ctx context.Context, ids []int64) error {
	ids = uniqueStoredMessageIds(ids)
	if len(ids) == 0 {
		return nil
	}
	_, err := g.DB().Model(messageTable).Safe().Ctx(ctx).WhereIn("id", ids).Data(g.Map{"retained_at": nil}).Update()
	return gerror.Wrap(err, "释放Telegram原消息失败")
}
