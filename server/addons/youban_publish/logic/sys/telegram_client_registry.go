package sys

import (
	"context"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gotd/td/telegram"
)

func (s *sSysPublish) TelegramClient(ctx context.Context, accountId int64) (*telegram.Client, error) {
	if accountId <= 0 {
		return nil, gerror.New("Telegram账号无效")
	}
	s.accountClientsMu.RLock()
	client := s.accountClients[accountId]
	s.accountClientsMu.RUnlock()
	if client == nil {
		return nil, gerror.New("Telegram账号运行连接不可用")
	}
	return client, nil
}
