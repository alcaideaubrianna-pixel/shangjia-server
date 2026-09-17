package sys

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"

	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	publishService "hotgo/addons/youban_publish/service"
)

var forwardedProfileNoRegexp = regexp.MustCompile(`(?i)(?:资料|笔记)?编号\s*[:：]?\s*([A-Z][A-Z0-9]{4,})`)

func (s *sSysBot) lookupForwardedScanProfile(ctx context.Context, botId int64, account *botProfileAccount, msg *models.Message) (*publishsysin.NoteModel, string, error) {
	if account == nil || msg == nil || msg.ForwardOrigin == nil {
		return nil, "none", nil
	}
	chatId, messageId := forwardedChannelMessageRef(msg)
	profileNo := forwardedCaptionProfileNo(msg.Caption)
	source := "profile_no"
	if chatId != "" && messageId > 0 {
		source = "send_ledger"
	}
	if chatId == "" && profileNo == "" {
		observeScanDirectLookup(ctx, botId, source, false)
		return nil, source, nil
	}
	startedAt := time.Now()
	note, err := publishService.SysPublish().BotProfileForwardLookup(ctx, &publishsysin.BotProfileForwardLookupInp{
		TenantId: account.TenantId, AccountId: account.AccountId, AccountType: account.AccountType,
		TargetChatId: chatId, TgMessageId: int64(messageId), ProfileNo: profileNo,
	})
	observeScanStage(ctx, botId, "direct_lookup", startedAt, err)
	observeScanDirectLookup(ctx, botId, source, note != nil && note.Id > 0)
	if err != nil {
		return nil, source, err
	}
	if note != nil && note.Id > 0 {
		g.Log().Info(ctx, "Bot扫图命中发送记录快捷查询", g.Map{
			"botId": botId, "tenantId": account.TenantId, "accountId": account.AccountId,
			"profileId": note.Id, "profileNo": note.ProfileNo, "source": source,
		})
	}
	return note, source, nil
}

func forwardedChannelMessageRef(msg *models.Message) (string, int) {
	if msg == nil || msg.ForwardOrigin == nil || msg.ForwardOrigin.MessageOriginChannel == nil {
		return "", 0
	}
	origin := msg.ForwardOrigin.MessageOriginChannel
	if origin.Chat.ID == 0 || origin.MessageID <= 0 {
		return "", 0
	}
	return strconv.FormatInt(origin.Chat.ID, 10), origin.MessageID
}

func forwardedCaptionProfileNo(caption string) string {
	matches := forwardedProfileNoRegexp.FindStringSubmatch(strings.TrimSpace(caption))
	if len(matches) != 2 {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(matches[1]))
}
