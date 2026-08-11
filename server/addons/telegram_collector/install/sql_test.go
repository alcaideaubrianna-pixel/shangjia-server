package install

import (
	"context"
	"os"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
)

func TestUpgradeIntegration(t *testing.T) {
	if os.Getenv("YOUBAN_TELEGRAM_COLLECTOR_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_TELEGRAM_COLLECTOR_INTEGRATION=1 to run database integration test")
	}
	if err := Upgrade(context.Background()); err != nil {
		t.Fatalf("upgrade telegram collector: %v", err)
	}
}
