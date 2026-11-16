package sysin

import (
	"strings"

	"hotgo/addons/telegram_collector/domain"
)

// RawUpdateIDs exposes the persisted identity using strict domain types while
// keeping the JSON/DAO-facing event shape unchanged.
func (e RawUpdateEvent) RawUpdateIDs() (domain.TenantID, domain.AccountID, domain.SourceID, domain.ChannelID, domain.MessageID) {
	return domain.TenantID(e.TenantID), domain.AccountID(e.AccountID), domain.SourceID(e.SourceID), domain.ChannelID(e.ChatID), domain.MessageID(e.MessageID)
}

func (e RawUpdateEvent) ValidateBoundary() bool {
	tenantID, accountID, sourceID, chatID, messageID := e.RawUpdateIDs()
	if !tenantID.Valid() || !sourceID.Valid() || !accountID.Valid() {
		return false
	}
	if e.SourceType != SourceTypeBot && e.SourceType != SourceTypeAccount {
		return false
	}
	if e.SourceType == SourceTypeAccount && e.AccountID <= 0 {
		return false
	}
	if e.ChatID != 0 && !chatID.Valid() {
		return false
	}
	if e.MessageID != 0 && !messageID.Valid() {
		return false
	}
	return true
}

func (e RawUpdateEvent) Route() domain.Route {
	return domain.Route{
		Pool:      poolForPriority(e.Priority),
		Priority:  domain.EventPriority(e.Priority),
		AccountID: domain.AccountID(e.AccountID),
		SourceID:  domain.SourceID(e.SourceID),
	}
}

// AccountMessageIDs exposes account message identity using strict domain types.
func (e AccountMessageEvent) AccountMessageIDs() (domain.TenantID, domain.AccountID, domain.SourceID, domain.MessageID) {
	return domain.TenantID(e.TenantID), domain.AccountID(e.AccountID), domain.SourceID(e.SourceID), domain.MessageID(e.SourceMessageID)
}

func (e AccountMessageEvent) ValidateBoundary() bool {
	tenantID, accountID, sourceID, messageID := e.AccountMessageIDs()
	return tenantID.Valid() && accountID.Valid() && sourceID.Valid() && messageID.Valid() &&
		e.TgAccountID > 0 && strings.TrimSpace(e.SourceChatID) != "" && strings.TrimSpace(e.SourceUniqueKey) != ""
}

func (e AccountMessageEvent) Route() domain.Route {
	priority := domain.EventPriority(EventPriorityRealtime)
	if e.History {
		priority = domain.EventPriority(EventPriorityNormal)
	}
	return domain.Route{
		Pool:      poolForHistory(e.History),
		Priority:  priority,
		AccountID: domain.AccountID(e.AccountID),
		SourceID:  domain.SourceID(e.SourceID),
	}
}

// CollectorDeliveryIDs exposes delivery identity using strict domain types.
func (d CollectorDelivery) CollectorDeliveryIDs() (domain.DeliveryID, domain.EventID, domain.TenantID, domain.AccountID, domain.SourceID, domain.MessageID) {
	return domain.DeliveryID(d.ID), domain.EventID(d.EventID), domain.TenantID(d.TenantID), domain.AccountID(d.AccountID), domain.SourceID(d.SourceID), domain.MessageID(d.SourceMessageID)
}

func (d CollectorDelivery) ValidateBoundary() bool {
	deliveryID, eventID, tenantID, accountID, sourceID, messageID := d.CollectorDeliveryIDs()
	return deliveryID.Valid() && eventID.Valid() && tenantID.Valid() && accountID.Valid() && sourceID.Valid() &&
		messageID.Valid() && strings.TrimSpace(d.DeliveryKey) != ""
}

func poolForPriority(priority int) domain.SchedulerPool {
	if priority >= EventPriorityRealtime {
		return domain.PoolRealtime
	}
	return domain.PoolHistory
}

func poolForHistory(history bool) domain.SchedulerPool {
	if history {
		return domain.PoolHistory
	}
	return domain.PoolRealtime
}
