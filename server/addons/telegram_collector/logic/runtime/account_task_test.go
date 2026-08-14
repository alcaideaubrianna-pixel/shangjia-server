package runtime

import (
	"testing"
	"time"

	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestAccountTaskTimeoutForMaterialImportHistoryPage(t *testing.T) {
	if got := accountTaskTimeout(sysin.AccountTaskTypeMaterialImportHistoryPage); got != 25*time.Minute {
		t.Fatalf("material import timeout = %s, want %s", got, 25*time.Minute)
	}
}
