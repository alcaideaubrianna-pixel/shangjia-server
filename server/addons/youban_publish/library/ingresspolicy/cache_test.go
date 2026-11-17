package ingresspolicy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/os/gcache"
	"hotgo/addons/telegram_collector/domain"
	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
)

func TestPolicyScopeAndConcurrentInvalidation(t *testing.T) {
	ctx := context.Background()
	c := gcache.New()
	defer c.Close(ctx)
	if v, err := EnsureVersion(ctx, c, "new-version"); err != nil || v == "" {
		t.Fatalf("version initialization: %q %v", v, err)
	}
	s := collectorin.BotIngressScope{TenantID: 78, SourceID: 351, ChatID: -1004411799859}
	if err := c.Set(ctx, "version", "v1", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := Remember(ctx, c, s, "v1", collectorin.IngressDeferralApproval); err != nil {
		t.Fatal(err)
	}
	if reason, err := Lookup(ctx, c, "version", s); err != nil || reason != collectorin.IngressDeferralApproval {
		t.Fatalf("reason=%s err=%v", reason, err)
	}
	short := s
	short.ChatID = domain.TelegramChatID(4411799859)
	if reason, err := Lookup(ctx, c, "version", short); err != nil || !reason.Valid() {
		t.Fatalf("short ID: %s %v", reason, err)
	}
	for _, other := range []collectorin.BotIngressScope{{TenantID: 79, SourceID: 351, ChatID: s.ChatID}, {TenantID: 78, SourceID: 352, ChatID: s.ChatID}, {TenantID: 78, SourceID: 351, ChatID: -1003736052878}} {
		if reason, err := Lookup(ctx, c, "version", other); err != nil || reason.Valid() {
			t.Fatalf("cross-scope decision: %s %v", reason, err)
		}
	}
	if err := c.Set(ctx, "version", "v2", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := Remember(ctx, c, s, "v1", collectorin.IngressDeferralSelfLoop); err != nil {
		t.Fatal(err)
	}
	if reason, err := Lookup(ctx, c, "version", s); err != nil || reason.Valid() {
		t.Fatalf("stale writer poisoned new generation: %s %v", reason, err)
	}
}

type failedStore struct{ err error }

func (s failedStore) Get(context.Context, any) (*gvar.Var, error) { return nil, s.err }
func (s failedStore) SetIfNotExist(context.Context, any, any, time.Duration) (bool, error) {
	return false, s.err
}

func TestPolicyFailureAndUnknownValuesAreNotDecisions(t *testing.T) {
	ctx := context.Background()
	s := collectorin.BotIngressScope{TenantID: 78, SourceID: 351, ChatID: -1004411799859}
	want := errors.New("cache unavailable")
	if reason, err := Lookup(ctx, failedStore{want}, "version", s); !errors.Is(err, want) || reason.Valid() {
		t.Fatalf("failure became a rejection: %s %v", reason, err)
	}
	c := gcache.New()
	defer c.Close(ctx)
	_ = c.Set(ctx, "version", "v1", time.Hour)
	_ = c.Set(ctx, key(s, "v1"), "unknown", time.Minute)
	if reason, err := Lookup(ctx, c, "version", s); err == nil || reason.Valid() {
		t.Fatalf("unknown value accepted: %s %v", reason, err)
	}
}
