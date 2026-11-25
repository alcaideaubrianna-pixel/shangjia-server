package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gotd/td/telegram"
	collectorservice "hotgo/addons/telegram_collector/service"
)

type remoteChannelAdminPayload struct {
	ChannelID string `json:"channelId"`
}

func init() {
	collectorservice.RegisterAccountRemoteHandler(collectorservice.AccountRemoteOperationVerifyChannelAdmin,
		func(ctx context.Context, client *telegram.Client, raw json.RawMessage) (json.RawMessage, error) {
			var in remoteChannelAdminPayload
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, err
			}
			verified := false
			err := scanTgAccountChannelCachesWithClient(ctx, client, func(channels []*tgDialogCache) error {
				for _, channel := range channels {
					if channel != nil && normalizeTelegramChannelChatID(channel.ChannelId) == normalizeTelegramChannelChatID(in.ChannelID) && channel.CanAddAdmins == 1 {
						verified = true
						return nil
					}
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			return json.Marshal(verified)
		})
}

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

func (s *sSysPublish) ExecuteRemoteTelegram(ctx context.Context, accountID int64, operation string, payload any, result any) error {
	raw, err := collectorservice.ExecuteRemoteAccount(ctx, accountID, operation, payload, 45*time.Second)
	if err != nil {
		return err
	}
	if result != nil {
		if err = json.Unmarshal(raw, result); err != nil {
			return fmt.Errorf("解析Telegram账号远程响应失败: %w", err)
		}
	}
	return nil
}
