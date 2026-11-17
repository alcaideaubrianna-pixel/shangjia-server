package sys

import (
	"testing"

	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/telegram_collector/internal/model/entity"
)

func TestCollectorDeliveryIdentitySequenceReuse(t *testing.T) {
	old := entity.TgCollectorEvent{Id: 11223044, TenantId: 25, SourceId: 78, SourceType: "bot", EventKey: "old-update"}
	current := old
	current.EventKey = "new-update-922491"
	if collectorDeliveryKey(&old, nil) == collectorDeliveryKey(&current, nil) {
		t.Fatal("reused row ID collapsed distinct updates")
	}
	key := collectorDeliveryKey(&current, nil)
	current.Id++
	if key != collectorDeliveryKey(&current, nil) {
		t.Fatal("same update identity changed after row renumbering")
	}
	current.SourceId++
	if key == collectorDeliveryKey(&current, nil) {
		t.Fatal("different source reused delivery identity")
	}
}

func TestCollectorDeliveryIdentityAlbumMembersAndEdits(t *testing.T) {
	event := entity.TgCollectorEvent{TenantId: 25, SourceId: 78, SourceType: "bot"}
	// Legacy records without event_key must still distinguish members/edits.
	first := collectorDeliveryKey(&event, []byte(`{"message_id":922491,"media_group_id":"album"}`))
	second := collectorDeliveryKey(&event, []byte(`{"message_id":922492,"media_group_id":"album"}`))
	edit := collectorDeliveryKey(&event, []byte(`{"message_id":922491,"media_group_id":"album","caption":"edited"}`))
	if first == second || first == edit {
		t.Fatal("distinct album members or edits collapsed")
	}
}

func TestCollectorDeliveryIdentityLegacyCollisionFence(t *testing.T) {
	event := entity.TgCollectorEvent{Id: 11223044, TenantId: 25, SourceId: 78, EventKey: "current", CreatedAt: gtime.New("2026-11-16 12:01:35")}
	delivery := entity.TgCollectorDelivery{EventId: event.Id, TenantId: 25, DeliveryKey: "event:11223044", CreatedAt: gtime.New("2026-10-15 10:31:02")}
	if validateCollectorDeliveryEvent(&delivery, &event, nil) == nil {
		t.Fatal("old delivery accepted a newer reused event ID")
	}
	delivery.CreatedAt = gtime.New("2026-11-16 12:01:36")
	if err := validateCollectorDeliveryEvent(&delivery, &event, nil); err != nil {
		t.Fatal(err)
	}
	delivery.DeliveryKey = collectorDeliveryKey(&event, nil)
	if err := validateCollectorDeliveryEvent(&delivery, &event, nil); err != nil {
		t.Fatal(err)
	}
	delivery.DeliveryKey = collectorDeliveryCollisionKey(collectorDeliveryKey(&event, nil), event.Id)
	if err := validateCollectorDeliveryEvent(&delivery, &event, nil); err != nil {
		t.Fatalf("replacement delivery rejected: %v", err)
	}
	if delivery.DeliveryKey != collectorDeliveryCollisionKey(collectorDeliveryKey(&event, nil), event.Id) {
		t.Fatal("retry changed replacement identity")
	}
	event.EventKey = "replacement"
	if validateCollectorDeliveryEvent(&delivery, &event, nil) == nil {
		t.Fatal("v2 delivery accepted replacement event")
	}
	if validateCollectorDeliveryEvent(nil, &event, nil) == nil {
		t.Fatal("nil delivery accepted")
	}
}
