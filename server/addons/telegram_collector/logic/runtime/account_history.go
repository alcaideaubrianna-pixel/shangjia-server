package runtime

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

type accountHistory struct{}

type accountHistoryRetryError struct {
	delay time.Duration
	err   error
}

func (e *accountHistoryRetryError) Error() string { return e.err.Error() }
func (e *accountHistoryRetryError) Unwrap() error { return e.err }

func init() {
	collectorservice.RegisterAccountHistory(&accountHistory{})
}

func (h *accountHistory) FetchPage(ctx context.Context, client *telegram.Client, request *sysin.AccountHistoryPageRequest) ([]*tg.Message, error) {
	if client == nil || !validAccountHistoryPageRequest(request) {
		return nil, gerror.New("Telegram历史分页参数无效")
	}
	var result tg.MessagesMessagesClass
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		result, err = client.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:     request.Peer,
			OffsetID: request.OffsetID, Limit: request.Limit,
		})
		if err == nil {
			return accountHistoryMessages(result), nil
		}
		if delay, ok := tgerr.AsFloodWait(err); ok {
			return nil, &accountHistoryRetryError{delay: delay + 2*time.Second, err: err}
		}
		if !accountHistoryRetryable(err) || attempt == 2 {
			return nil, gerror.Wrap(err, "拉取Telegram历史消息失败")
		}
		delay := time.Duration(attempt+1) * time.Second
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return accountHistoryMessages(result), nil
}

func validAccountHistoryPageRequest(request *sysin.AccountHistoryPageRequest) bool {
	if request == nil || request.Peer == nil || request.Limit <= 0 {
		return false
	}
	switch peer := request.Peer.(type) {
	case *tg.InputPeerChat:
		return peer.ChatID > 0
	case *tg.InputPeerChannel:
		return peer.ChannelID > 0 && peer.AccessHash != 0
	default:
		return false
	}
}

func (h *accountHistory) RetryDelay(err error) (time.Duration, bool) {
	retry, ok := err.(*accountHistoryRetryError)
	if !ok || retry == nil {
		return 0, false
	}
	return retry.delay, true
}

func accountHistoryRetryable(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, keyword := range []string{"timeout", "eof", "connection reset", "broken pipe", "dc is closed"} {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}
