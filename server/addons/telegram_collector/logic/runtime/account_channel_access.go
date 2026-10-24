package runtime

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

func (w *accountWorker) channelAccessUpdateHandler(next telegram.UpdateHandler) telegram.UpdateHandler {
	return telegram.UpdateHandlerFunc(func(ctx context.Context, update tg.UpdatesClass) error {
		observer, ok := w.session.(collectorservice.AccountRuntimeChannelAccessObserver)
		if ok {
			for _, event := range accountChannelAccessEvents(w.selfUserID.Load(), update) {
				observeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				err := observer.HandleAccountRuntimeChannelAccess(observeCtx, event)
				cancel()
				if err != nil {
					// A notification failure must not suppress other messages in
					// this same Telegram update container.
					g.Log().Warningf(ctx, "TG账号频道状态通知入队失败 userId:%d channelId:%d reason:%s err:%+v", event.UserID, event.ChannelID, event.Reason, err)
				}
			}
		}
		return next.Handle(ctx, update)
	})
}

func accountChannelAccessEvents(selfID int64, update tg.UpdatesClass) []sysin.AccountChannelAccessEvent {
	if selfID <= 0 {
		return nil
	}
	var changes []tg.UpdateClass
	var chats []tg.ChatClass
	date := 0
	switch value := update.(type) {
	case *tg.Updates:
		changes, chats, date = value.Updates, value.Chats, value.Date
	case *tg.UpdatesCombined:
		changes, chats, date = value.Updates, value.Chats, value.Date
	case *tg.UpdateShort:
		changes, date = []tg.UpdateClass{value.Update}, value.Date
	}
	channels := make(map[int64]tg.ChatClass, len(chats))
	for _, chat := range chats {
		switch value := chat.(type) {
		case *tg.Channel:
			channels[value.ID] = value
		case *tg.ChannelForbidden:
			channels[value.ID] = value
		}
	}
	// Only interpret channel entities accompanying an actual channel-status
	// update; an unrelated response may contain partial/stale peer entities.
	events := make(map[int64]sysin.AccountChannelAccessEvent)
	for _, change := range changes {
		event := sysin.AccountChannelAccessEvent{UserID: selfID, Date: date}
		switch value := change.(type) {
		case *tg.UpdateChannelParticipant:
			if value.UserID != selfID {
				continue
			}
			event.ChannelID, event.Date = value.ChannelID, value.Date
			switch member := value.NewParticipant.(type) {
			case *tg.ChannelParticipantBanned:
				if !member.Left && !member.BannedRights.ViewMessages {
					continue // A send restriction does not prevent collection.
				}
				event.Reason = sysin.AccountChannelAccessRemoved
			case *tg.ChannelParticipantLeft:
				event.Reason = sysin.AccountChannelAccessLeft
			default:
				// Missing participant is ambiguous, not proof of removal.
				continue
			}
		case *tg.UpdateChannel:
			event.ChannelID = value.ChannelID
			switch channel := channels[value.ChannelID].(type) {
			case *tg.ChannelForbidden:
				// The container belongs to this connection/user.
				event.Reason = sysin.AccountChannelAccessForbidden
			case *tg.Channel:
				if channel.Min || (!channel.Left && !channel.BannedRights.ViewMessages) {
					continue
				}
				event.Reason = sysin.AccountChannelAccessLeft
				if channel.BannedRights.ViewMessages {
					event.Reason = sysin.AccountChannelAccessRemoved
				}
			default:
				continue
			}
		default:
			continue
		}
		if channel, ok := channels[event.ChannelID].(*tg.Channel); ok {
			event.Title = channel.Title
		} else if channel, ok := channels[event.ChannelID].(*tg.ChannelForbidden); ok {
			event.Title = channel.Title
		}
		if event.ChannelID <= 0 {
			continue
		}
		// Prefer explicit removal evidence over a generic inaccessible entity.
		if old, ok := events[event.ChannelID]; !ok || event.Reason == sysin.AccountChannelAccessRemoved || old.Reason != sysin.AccountChannelAccessRemoved {
			events[event.ChannelID] = event
		}
	}
	result := make([]sysin.AccountChannelAccessEvent, 0, len(events))
	for _, event := range events {
		result = append(result, event)
	}
	return result
}
