package telegrambudget

import (
	"context"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
)

func useConfig(t *testing.T, content string) {
	t.Helper()
	adapter, err := gcfg.NewAdapterContent(content)
	if err != nil {
		t.Fatal(err)
	}
	previous := g.Cfg().GetAdapter()
	g.Cfg().SetAdapter(adapter)
	t.Cleanup(func() { g.Cfg().SetAdapter(previous) })
}

func TestDefaultsAndTaskFinalizationMargin(t *testing.T) {
	useConfig(t, `{"youbanPublish":{"telegram":{}}}`)
	ctx := context.Background()
	for _, test := range []struct {
		name string
		read func(context.Context) time.Duration
		want time.Duration
	}{
		{"job", Job, 30 * time.Minute},
		{"display", Display, 10 * time.Minute},
		{"verify", Verify, 10 * time.Minute},
		{"bot", BotRequest, 10 * time.Minute},
		{"account", Account, 10 * time.Minute},
		{"accountGate", AccountGate, 30 * time.Second},
		{"finalize", Finalize, 10 * time.Second},
		{"response", Response, 5 * time.Second},
		{"shutdown", Shutdown, 30 * time.Second},
		{"enqueue", Enqueue, 5 * time.Second},
		{"preview", Preview, 30 * time.Second},
		{"poster", Poster, 5 * time.Second},
		{"sanitize", Sanitize, 10 * time.Minute},
		{"cleanup", Cleanup, 15 * time.Second},
		{"lease", LeaseOperation, 3 * time.Second},
		{"webhook", Webhook, 15 * time.Second},
	} {
		if got := test.read(ctx); got != test.want {
			t.Errorf("%s = %s, want %s", test.name, got, test.want)
		}
	}
	if got := TaskTimeout(ctx); got != 30*time.Minute+10*time.Second {
		t.Fatalf("task timeout must include finalization margin: %s", got)
	}
}

func TestConfiguredTaskMarginAndIndependentStageBudgets(t *testing.T) {
	useConfig(t, `{"youbanPublish":{"telegram":{"sendBudget":"20m","finalizeTimeout":"20s","displayTimeout":"8m","verifyTimeout":"9m","botRequestTimeout":"7m","accountTimeout":"6m","accountGateTimeout":"40s","sanitizeTimeout":"12m"}}}`)
	ctx := context.Background()
	if TaskTimeout(ctx) != 20*time.Minute+20*time.Second {
		t.Fatal("task deadline did not track configured job and finalization budgets")
	}
	if Display(ctx) != 8*time.Minute || Verify(ctx) != 9*time.Minute ||
		BotRequest(ctx) != 7*time.Minute || Account(ctx) != 6*time.Minute || AccountGate(ctx) != 40*time.Second || Sanitize(ctx) != 12*time.Minute {
		t.Fatal("independent stage configuration was ignored")
	}
}

func TestInvalidJobBudgetsFallBackAndValidBounds(t *testing.T) {
	for _, value := range []string{"", "garbage", "0s", "-1m", "1m", "31m"} {
		t.Run(value, func(t *testing.T) {
			useConfig(t, `{"youbanPublish":{"telegram":{"sendBudget":"`+value+`"}}}`)
			if got := Job(context.Background()); got != DefaultJob {
				t.Fatalf("invalid budget = %s, want %s", got, DefaultJob)
			}
		})
	}
	for _, value := range []string{"2m", "30m"} {
		t.Run(value, func(t *testing.T) {
			useConfig(t, `{"youbanPublish":{"telegram":{"sendBudget":"`+value+`"}}}`)
			want, _ := time.ParseDuration(value)
			if got := Job(context.Background()); got != want {
				t.Fatalf("boundary budget = %s, want %s", got, want)
			}
		})
	}
}
