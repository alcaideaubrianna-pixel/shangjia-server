package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/gotd/td/telegram"
	"hotgo/internal/library/hgrds/pubsub"
)

// AccountRemoteRequest is the serialized request exchanged between web/publish
// processes and the account process. The Telegram client remains owned by the
// account process.
type AccountRemoteRequest struct {
	RequestID string          `json:"requestId"`
	AccountID int64           `json:"accountId"`
	Operation string          `json:"operation"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type AccountRemoteResponse struct {
	RequestID string          `json:"requestId"`
	Success   bool            `json:"success"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     string          `json:"error,omitempty"`
}

const (
	AccountRemoteOperationVerifyChannelAdmin = "verify_channel_admin"
	AccountRemoteOperationJoinChannel        = "join_channel"
)

// AccountRemoteHandler executes one serialized operation on the account role.
// Implementations must return JSON that can be decoded by the caller.
type AccountRemoteHandler func(context.Context, *telegram.Client, json.RawMessage) (json.RawMessage, error)

var remoteHandlers = map[string]AccountRemoteHandler{}
var remoteWaiters sync.Map

const accountRemoteRequestChannel = "telegram_collector:account_remote:request"

func RegisterAccountRemoteHandler(operation string, handler AccountRemoteHandler) {
	if operation != "" && handler != nil {
		remoteHandlers[operation] = handler
	}
}

func AccountRemoteHandlerFor(operation string) AccountRemoteHandler {
	return remoteHandlers[operation]
}

func ExecuteRemoteAccount(ctx context.Context, accountID int64, operation string, payload any, timeout time.Duration) (json.RawMessage, error) {
	if accountID <= 0 || operation == "" {
		return nil, fmt.Errorf("invalid account remote request")
	}
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	requestID := guid.S()
	responseChannel := accountRemoteResponseChannel(requestID)
	waiter := make(chan AccountRemoteResponse, 1)
	remoteWaiters.Store(responseChannel, waiter)
	defer remoteWaiters.Delete(responseChannel)
	stopResponse, err := pubsub.SubscribeReady(ctx, responseChannel, func(_ context.Context, message *gredis.Message) {
		var response AccountRemoteResponse
		if err := json.Unmarshal([]byte(message.Payload), &response); err == nil {
			DeliverRemoteAccountResponse(response)
		}
	})
	if err != nil {
		return nil, err
	}
	defer stopResponse()
	body, err := json.Marshal(AccountRemoteRequest{RequestID: requestID, AccountID: accountID, Operation: operation, Payload: mustJSON(payload)})
	if err != nil {
		return nil, err
	}
	if _, err = pubsub.Publish(ctx, accountRemoteRequestChannel, string(body)); err != nil {
		return nil, err
	}
	g.Log().Info(ctx, "Telegram账号跨角色请求已发布", g.Map{"requestId": requestID, "accountId": accountID, "operation": operation})
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case response := <-waiter:
		g.Log().Info(ctx, "Telegram账号跨角色响应已收到", g.Map{"requestId": requestID, "accountId": accountID, "operation": operation, "success": response.Success})
		if !response.Success {
			return nil, fmt.Errorf("%s", response.Error)
		}
		return response.Payload, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("Telegram账号跨角色操作超时 accountId:%d", accountID)
	}
}

func DeliverRemoteAccountResponse(response AccountRemoteResponse) {
	if value, ok := remoteWaiters.Load(accountRemoteResponseChannel(response.RequestID)); ok {
		value.(chan AccountRemoteResponse) <- response
	}
}

func AccountRemoteRequestChannel() string { return accountRemoteRequestChannel }

func AccountRemoteResponseChannel(requestID string) string {
	return accountRemoteResponseChannel(requestID)
}

func accountRemoteResponseChannel(requestID string) string {
	return "telegram_collector:account_remote:response:" + requestID
}

func mustJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		g.Log().Warningf(context.Background(), "序列化Telegram跨角色请求参数失败 err:%+v", err)
		return nil
	}
	return data
}
