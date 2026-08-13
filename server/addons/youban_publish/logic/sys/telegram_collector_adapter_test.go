package sys

import (
	"testing"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

func TestPublishAccountTaskHandlersRegistered(t *testing.T) {
	for _, taskType := range []string{
		collectorin.AccountTaskTypeHistoryPage,
		collectorin.AccountTaskTypeDialogCacheRefresh,
	} {
		if collectorservice.AccountTaskHandlerFor(taskType) == nil {
			t.Fatalf("Telegram account task handler is not registered: %s", taskType)
		}
	}
}
