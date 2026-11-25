package runtime

import (
	"context"
	"encoding/json"

	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"
	"hotgo/addons/telegram_collector/service"
	"hotgo/internal/library/hgrds/pubsub"
)

func (r *accountRuntime) startRemoteExecutor(ctx context.Context) {
	_ = pubsub.Subscribe(service.AccountRemoteRequestChannel(), func(handlerCtx context.Context, message *gredis.Message) {
		var request service.AccountRemoteRequest
		if err := json.Unmarshal([]byte(message.Payload), &request); err != nil {
			return
		}
		g.Log().Info(handlerCtx, "Account角色收到Telegram跨角色请求", g.Map{"requestId": request.RequestID, "accountId": request.AccountID, "operation": request.Operation})
		handler := service.AccountRemoteHandlerFor(request.Operation)
		if handler == nil {
			publishRemoteResponse(handlerCtx, request, false, nil, "Telegram账号跨角色操作不支持")
			return
		}
		var payload json.RawMessage
		var operationErr error
		ok, err := r.Execute(handlerCtx, request.AccountID, 45_000_000_000, func(operationCtx context.Context, client *telegram.Client) error {
			payload, operationErr = handler(operationCtx, client, request.Payload)
			return operationErr
		})
		if err != nil {
			publishRemoteResponse(handlerCtx, request, false, nil, err.Error())
			return
		}
		if !ok {
			g.Log().Warning(handlerCtx, "Account角色未找到Telegram账号运行Worker", g.Map{"requestId": request.RequestID, "accountId": request.AccountID})
			publishRemoteResponse(handlerCtx, request, false, nil, "Telegram账号运行连接不可用")
			return
		}
		publishRemoteResponse(handlerCtx, request, true, payload, "")
	})
	_ = ctx
}

func publishRemoteResponse(ctx context.Context, request service.AccountRemoteRequest, success bool, payload json.RawMessage, message string) {
	data, _ := json.Marshal(service.AccountRemoteResponse{RequestID: request.RequestID, Success: success, Payload: payload, Error: message})
	if _, err := pubsub.Publish(ctx, service.AccountRemoteResponseChannel(request.RequestID), string(data)); err != nil {
		g.Log().Warningf(ctx, "发布Telegram跨角色响应失败 requestId:%s err:%+v", request.RequestID, err)
	}
}
