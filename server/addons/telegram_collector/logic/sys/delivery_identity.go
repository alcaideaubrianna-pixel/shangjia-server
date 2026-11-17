package sys

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/telegram_collector/internal/model/entity"
)

// collectorDeliveryKey identifies an individual raw update, never an album or a
// reusable database sequence value. Routing scope remains part of the identity.
func collectorDeliveryKey(event *entity.TgCollectorEvent, raw []byte) string {
	identity := struct {
		TenantID   int64
		SourceID   int64
		AccountID  int64
		SourceType string
		EventKey   string
		RawHash    string
	}{
		TenantID: event.TenantId, SourceID: event.SourceId,
		AccountID: event.AccountId, SourceType: event.SourceType,
		EventKey: strings.TrimSpace(event.EventKey),
	}
	// Older fixtures/imports can lack an event key. Their payload, rather than
	// the numeric row ID, supplies the update identity in that case.
	if identity.EventKey == "" {
		identity.RawHash = fmt.Sprintf("%x", sha256.Sum256(raw))
	}
	encoded, _ := json.Marshal(identity) // Only scalar fields; cannot fail.
	return fmt.Sprintf("event:v2:%x", sha256.Sum256(encoded))
}

func collectorDeliveryCollisionKey(baseKey string, eventID int64) string {
	return fmt.Sprintf("event:repair:%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d", baseKey, eventID))))
}

// A delayed queue entry must not deliver a different raw update after a restore
// or sequence reuse. Legacy deliveries are accepted only within their original
// event lifetime; they are never reset or overwritten to repair a collision.
func validateCollectorDeliveryEvent(delivery *entity.TgCollectorDelivery, event *entity.TgCollectorEvent, raw []byte) error {
	if delivery == nil || event == nil || event.Id <= 0 ||
		delivery.EventId != event.Id || delivery.TenantId != event.TenantId {
		return gerror.New("采集交付与原始事件归属不一致")
	}
	if strings.HasPrefix(delivery.DeliveryKey, "event:v2:") || strings.HasPrefix(delivery.DeliveryKey, "event:repair:") {
		baseKey := collectorDeliveryKey(event, raw)
		if delivery.DeliveryKey != baseKey && delivery.DeliveryKey != collectorDeliveryCollisionKey(baseKey, event.Id) {
			return gerror.New("采集交付原始事件身份已变化，禁止交付")
		}
		return nil
	}
	if delivery.DeliveryKey != fmt.Sprintf("event:%d", event.Id) ||
		delivery.CreatedAt == nil || event.CreatedAt == nil ||
		delivery.CreatedAt.Before(event.CreatedAt) {
		return gerror.New("历史采集交付身份冲突，禁止复用旧交付")
	}
	return nil
}
