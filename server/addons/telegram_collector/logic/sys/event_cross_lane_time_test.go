package sys

import (
	"fmt"
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestBotCrossLanePreservesSourceMessageTime(t *testing.T) {
	displayAt := time.Now().Add(-6 * time.Hour).Truncate(time.Second)
	verifyAt := displayAt.Add(10 * time.Second)
	// The old display and fresh verification can reach raw persistence six
	// hours apart. Pairing time must still derive from Telegram message.date.
	var got []time.Time
	for i, sourceAt := range []time.Time{displayAt, verifyAt} {
		receivedAt := displayAt
		if i == 1 {
			receivedAt = time.Now()
		}
		row := &entity.TgCollectorEvent{Id: int64(i + 1), TenantId: 78, SourceId: 351,
			SourceType: sysin.SourceTypeBot, ChatId: "-1004411799859", MessageId: int64(100 + i), ReceivedAt: gtime.New(receivedAt)}
		raw := []byte(fmt.Sprintf(`{"update_id":%d,"channel_post":{"message_id":%d,"date":%d,"chat":{"id":-1004411799859,"type":"channel"},"text":"本地跨通道时间测试"}}`, i+1, 100+i, sourceAt.Unix()))
		delivery, err := collectorDeliveryFromEvent(row, raw)
		if err != nil {
			t.Fatal(err)
		}
		if !delivery.ReceivedAt.Equal(sourceAt) {
			t.Fatalf("message time replaced by persistence/worker time: got=%v want=%v", delivery.ReceivedAt, sourceAt)
		}
		if delivery.SourceID != 351 || delivery.TenantID != 78 {
			t.Fatal("cross-lane ownership changed")
		}
		got = append(got, delivery.ReceivedAt)
	}
	if got[1].Sub(got[0]) != 10*time.Second {
		t.Fatal("queue delay expanded pairing interval")
	}
}
