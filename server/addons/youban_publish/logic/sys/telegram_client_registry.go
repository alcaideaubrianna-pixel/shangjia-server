package sys

import (
	"context"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gotd/td/telegram"
	collectorservice "hotgo/addons/telegram_collector/service"
	"time"
)

func (s *sSysPublish) TelegramClient(ctx context.Context, accountId int64) (*telegram.Client, error) {
	if accountId <= 0 {
		return nil, gerror.New("Telegram账号无效")
	}
	s.accountClientsMu.RLock()
	client := s.accountClients[accountId]
	s.accountClientsMu.RUnlock()
	if client != nil {
		return client, nil
	}
	// The runtime is authoritative. The map is only a local compatibility
	// cache and may be empty after a restart or on another API replica.
	var runtimeClient *telegram.Client
	if err := s.ExecuteTelegram(ctx, accountId, func(_ context.Context, client *telegram.Client) error {
		runtimeClient = client
		return nil
	}); err != nil {
		return nil, err
	}
	return runtimeClient, nil
}

// ExecuteTelegram runs an operation through the account runtime. Callers must
// use this for Telegram RPCs instead of reading the process-local client map.
func (s *sSysPublish) ExecuteTelegram(ctx context.Context, accountID int64, operation collectorservice.AccountOperation) error {
	if accountID <= 0 || operation == nil {
		return gerror.New("Telegram账号无效")
	}
	runtime := collectorservice.AccountRuntime()
	if runtime == nil {
		return gerror.New("Telegram账号运行时未启动")
	}
	ok, err := runtime.Execute(ctx, accountID, 45*time.Second, operation)
	if err != nil {
		return err
	}
	if !ok {
		return gerror.New("Telegram账号运行连接不可用")
	}
	return nil
}
