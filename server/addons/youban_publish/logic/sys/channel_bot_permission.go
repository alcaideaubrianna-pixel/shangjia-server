package sys

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/contexts"
)

type channelBotPermissionState struct {
	BotId             int64  `json:"botId"`
	BotName           string `json:"botName"`
	BotUsername       string `json:"botUsername"`
	CanSendMessages   int    `json:"canSendMessages"`
	CanDeleteMessages int    `json:"canDeleteMessages"`
	InChannel         int    `json:"inChannel"`
	Status            string `json:"status"`
	Message           string `json:"message"`
	CheckedAt         string `json:"checkedAt"`
}

func encodeChannelBotPermissionStates(results []*sysin.ChannelCheckBotModel) string {
	states := make([]*channelBotPermissionState, 0, len(results))
	for _, result := range results {
		if result == nil || result.BotId <= 0 {
			continue
		}
		states = append(states, &channelBotPermissionState{
			BotId:             result.BotId,
			BotName:           result.BotName,
			BotUsername:       result.BotUsername,
			CanSendMessages:   result.CanSendMessage,
			CanDeleteMessages: result.CanDeleteMessages,
			InChannel:         result.InChannel,
			Status:            result.Status,
			Message:           result.Message,
		})
	}
	data, err := json.Marshal(states)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func mergeChannelBotPermissionState(raw string, result *sysin.ChannelCheckBotModel) string {
	if result == nil || result.BotId <= 0 {
		return raw
	}
	states := decodeChannelBotPermissionStates(raw)
	updated := false
	for _, state := range states {
		if state == nil || state.BotId != result.BotId {
			continue
		}
		state.BotName = result.BotName
		state.BotUsername = result.BotUsername
		state.CanSendMessages = result.CanSendMessage
		state.CanDeleteMessages = result.CanDeleteMessages
		state.InChannel = result.InChannel
		state.Status = result.Status
		state.Message = result.Message
		updated = true
		break
	}
	if !updated {
		states = append(states, &channelBotPermissionState{
			BotId:             result.BotId,
			BotName:           result.BotName,
			BotUsername:       result.BotUsername,
			CanSendMessages:   result.CanSendMessage,
			CanDeleteMessages: result.CanDeleteMessages,
			InChannel:         result.InChannel,
			Status:            result.Status,
			Message:           result.Message,
		})
	}
	data, err := json.Marshal(states)
	if err != nil {
		return raw
	}
	return string(data)
}

func decodeChannelBotPermissionStates(raw string) []*channelBotPermissionState {
	var states []*channelBotPermissionState
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &states); err != nil || states == nil {
		return []*channelBotPermissionState{}
	}
	return states
}

func channelBotPermissionStateForBot(raw string, botId int64) *channelBotPermissionState {
	for _, state := range decodeChannelBotPermissionStates(raw) {
		if state != nil && state.BotId == botId {
			return state
		}
	}
	return nil
}

func channelBotPermissionSummary(raw string) (status string, message string) {
	states := decodeChannelBotPermissionStates(raw)
	if len(states) == 0 {
		return "unknown", "尚未检测频道 Bot 权限"
	}
	for _, state := range states {
		if state == nil {
			continue
		}
		if state.CanSendMessages != 1 || state.CanDeleteMessages != 1 {
			name := strings.TrimSpace(state.BotName)
			if name == "" {
				name = fmt.Sprintf("Bot %d", state.BotId)
			}
			if strings.TrimSpace(state.Message) == "" {
				return "error", name + " 没有完整的发送或删除消息权限"
			}
			return "error", name + "：" + state.Message
		}
	}
	return "ok", "频道 Bot 发送和删除消息权限正常"
}

func applyChannelBotPermissionSummary(list []*sysin.ChannelModel) {
	for _, item := range list {
		if item == nil {
			continue
		}
		item.BotPermissionStatus, item.BotPermissionMessage = channelBotPermissionSummary(item.BotPermissionStatusJson)
	}
}

func (s *sSysPublish) persistChannelBotPermissionState(ctx context.Context, tenantId, channelId int64, results []*sysin.ChannelCheckBotModel) error {
	if tenantId <= 0 || channelId <= 0 {
		return nil
	}
	_, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("id", channelId).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Data(g.Map{
			"bot_permission_status_json": encodeChannelBotPermissionStates(results),
			"updated_by":                 contexts.GetUserId(ctx),
		}).Update()
	return err
}

func (s *sSysPublish) persistChannelBotPermission(ctx context.Context, tenantId, channelId, tgAccountId int64, targetChatId string, results []*sysin.ChannelCheckBotModel) error {
	if channelId > 0 {
		_, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
			Where("id", channelId).
			Where("tenant_id", tenantId).
			Where("tg_account_id", tgAccountId).
			WhereNull("deleted_at").
			Data(g.Map{
				"bot_permission_status_json": encodeChannelBotPermissionStates(results),
			}).Update()
		return err
	}
	var channel struct {
		Id int64 `json:"id"`
	}
	query := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id").
		Where("tenant_id", tenantId).
		Where("tg_account_id", tgAccountId).
		WhereNull("deleted_at")
	targetChatId = strings.TrimSpace(targetChatId)
	if targetChatId != "" {
		lookupIds := tgChannelCacheLookupIds(targetChatId)
		conditions := make([]string, 0, len(lookupIds)+1)
		args := make([]interface{}, 0, len(lookupIds)+1)
		for _, lookupId := range lookupIds {
			conditions = append(conditions, "target_chat_id = ?")
			args = append(args, lookupId)
		}
		if username := strings.TrimPrefix(targetChatId, "@"); username != "" {
			conditions = append(conditions, "channel_username = ?")
			args = append(args, username)
		}
		if len(conditions) > 0 {
			query = query.Where("("+strings.Join(conditions, " OR ")+")", args...)
		}
	}
	if err := query.OrderDesc("id").Scan(&channel); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if channel.Id <= 0 {
		return nil
	}
	return s.persistChannelBotPermissionState(ctx, tenantId, channel.Id, results)
}
