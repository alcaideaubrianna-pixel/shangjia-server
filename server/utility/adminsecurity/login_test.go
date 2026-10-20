package adminsecurity

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"hotgo/internal/library/cache"
)

func TestSharedLoginLimitAndCaptcha(t *testing.T) {
	redis := miniredis.RunT(t)
	adapter, err := gcfg.NewAdapterContent(fmt.Sprintf(`{"cache":{"adapter":"redis"},"redis":{"default":{"address":%q}},"database":{"default":{"link":"pgsql:test:@tcp(127.0.0.1:1)/unused"}}}`, redis.Addr()))
	if err != nil {
		t.Fatal(err)
	}
	old := g.Cfg().GetAdapter()
	g.Cfg().SetAdapter(adapter)
	t.Cleanup(func() { g.Cfg().SetAdapter(old) })
	ctx := context.Background()
	cache.SetAdapter(ctx)
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := reserveAttempt(ctx, "concurrent", 10, time.Minute)
			if err != nil {
				t.Error(err)
			}
			if ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	// Lock contention may conservatively reject attempts; it must never permit
	// more than the shared budget. New limiter instances use that same budget.
	if allowed.Load() > 10 || allowed.Load() == 0 {
		t.Fatal("budget bypass", allowed.Load())
	}
	for i := 0; i < 10; i++ {
		_, err := reserveAttempt(ctx, "concurrent", 10, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := reserveAttempt(ctx, "concurrent", 10, time.Minute); ok || err != nil {
		t.Fatal("limit reset across calls", ok, err)
	}
	redis.FastForward(2 * time.Minute)
	if ok, err := reserveAttempt(ctx, "concurrent", 10, time.Minute); !ok || err != nil {
		t.Fatal("expired budget did not recover", ok, err)
	}
	first := captchaStore{ctx}
	if err := first.Set("replica-test", "aB23"); err != nil {
		t.Fatal(err)
	}
	second := captchaStore{ctx}
	if !second.Verify("replica-test", "ab23", true) || first.Verify("replica-test", "ab23", true) {
		t.Fatal("captcha replica/replay protection failed")
	}
	if err := first.Set("expires-test", "abcd"); err != nil {
		t.Fatal(err)
	}
	redis.FastForward(6 * time.Minute)
	if second.Verify("expires-test", "abcd", true) {
		t.Fatal("expired captcha accepted")
	}
	id, image, err := GenerateCaptcha(ctx, 0)
	if err != nil || id == "" || image == "" {
		t.Fatal("captcha generation", id, err)
	}
	redis.Close()
	if ok, err := reserveAttempt(ctx, "outage", 10, time.Minute); ok || err == nil {
		t.Fatal("Redis failure allowed login", ok, err)
	}
}
