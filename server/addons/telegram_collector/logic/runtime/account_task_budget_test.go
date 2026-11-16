package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestAccountMediaFallbackUsesSharedBudget(t *testing.T) {
	adapter, err := gcfg.NewAdapterContent(`{"youbanPublish":{"telegram":{"accountTimeout":"12m"}}}`)
	if err != nil {
		t.Fatal(err)
	}
	previous := g.Cfg().GetAdapter()
	g.Cfg().SetAdapter(adapter)
	t.Cleanup(func() { g.Cfg().SetAdapter(previous) })
	if got := accountTaskTimeoutContext(context.Background(), sysin.AccountTaskTypeMessageMediaFallback); got != 12*time.Minute {
		t.Errorf("media fallback timeout = %s, want 12m", got)
	}
	if got := accountTaskTimeoutContext(context.Background(), sysin.AccountTaskTypeMessagePushInline); got != 2*time.Minute {
		t.Errorf("inline timeout changed unexpectedly: %s", got)
	}
	// A publishing policy must not alter an unrelated history task.
	if got := accountTaskTimeoutContext(context.Background(), sysin.AccountTaskTypeHistoryPage); got != 25*time.Minute {
		t.Fatalf("history timeout changed unexpectedly: %s", got)
	}
}
