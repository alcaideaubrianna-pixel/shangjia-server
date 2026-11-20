package sys

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcache"
)

func collectCooldownFixture() gdb.Record {
	return gdb.Record{"id": g.NewVar(21513466), "tenant_id": g.NewVar(73), "account_id": g.NewVar(608), "source_id": g.NewVar(1434),
		"status": g.NewVar("media_ready"), "error_message": g.NewVar("部分采集规则等待重试：规则1352: 存在无权操作或不可用的推送频道"),
		"text_hash": g.NewVar("hash-v1"), "raw_text": g.NewVar("正文"), "media_count": g.NewVar(8), "material_role": g.NewVar("display"), "material_group_status": g.NewVar("paired")}
}

func TestCollectEventCooldownSharedAcrossLanes(t *testing.T) {
	ctx := context.Background()
	event := collectCooldownFixture()
	now := time.Unix(1700000000, 0)
	store := gcache.New()
	defer store.Close(ctx)
	entry := collectEventCooldown{Version: collectEventCooldownVersion(event), Message: event["error_message"].String(), RetryAt: now.Add(5 * time.Minute).UnixMilli()}
	if err := store.Set(ctx, collectEventCooldownKey(event), entry, 5*time.Minute); err != nil {
		t.Fatal(err)
	}
	for _, lane := range []string{"realtime", "history", "legacy"} {
		err := checkCollectEventConfigurationCooldown(ctx, event, now.Add(2*time.Minute), func(ctx context.Context, key string) (*gvar.Var, error) { return store.Get(ctx, key) })
		var retry *collectProcessRetryError
		if !errors.As(err, &retry) || retry.delay != 3*time.Minute {
			t.Fatalf("lane %s escaped shared deadline: %v", lane, err)
		}
	}
	if err := checkCollectEventConfigurationCooldown(ctx, event, now.Add(5*time.Minute), func(ctx context.Context, key string) (*gvar.Var, error) { return store.Get(ctx, key) }); err != nil {
		t.Fatalf("deadline was extended by duplicate wake: %v", err)
	}
}

func TestCollectEventCooldownVersionAndScope(t *testing.T) {
	event := collectCooldownFixture()
	now := time.Now()
	entry := collectEventCooldown{Version: collectEventCooldownVersion(event), Message: event["error_message"].String(), RetryAt: now.Add(5 * time.Minute).UnixMilli()}
	event["updated_at"] = g.NewVar("recovery touched timestamp")
	if collectEventCooldownRemaining(event, entry, now) <= 0 {
		t.Fatal("recovery timestamp incorrectly bypassed cooldown")
	}
	for _, field := range []string{"raw_text", "text_hash", "media_count", "material_parent_event_id", "material_group_status", "error_message", "status"} {
		changed := collectCooldownFixture()
		changed[field] = g.NewVar("changed")
		if field == "media_count" || field == "material_parent_event_id" {
			changed[field] = g.NewVar(999)
		}
		if collectEventCooldownRemaining(changed, entry, now) != 0 {
			t.Fatalf("new input/state did not bypass old cooldown: %s", field)
		}
	}
	for _, field := range []string{"tenant_id", "account_id", "source_id", "id"} {
		changed := collectCooldownFixture()
		changed[field] = g.NewVar(999)
		if collectEventCooldownKey(changed) == collectEventCooldownKey(event) {
			t.Fatalf("cooldown crossed scope %s", field)
		}
	}
}

func TestCollectEventCooldownManualReplayAndCacheFailure(t *testing.T) {
	event := collectCooldownFixture()
	failure := errors.New("redis unavailable")
	calls := 0
	read := func(context.Context, string) (*gvar.Var, error) { calls++; return nil, failure }
	ctx := context.WithValue(context.Background(), collectManualReplayContextKey{}, event["id"].Int64())
	if err := checkCollectEventConfigurationCooldown(ctx, event, time.Now(), read); err != nil || calls != 0 {
		t.Fatalf("authorized replay did not bypass: %v", err)
	}
	other := context.WithValue(context.Background(), collectManualReplayContextKey{}, int64(999))
	if err := checkCollectEventConfigurationCooldown(other, event, time.Now(), read); !errors.Is(err, failure) {
		t.Fatalf("cache failure swallowed or other event bypassed: %v", err)
	}
	event["error_message"] = g.NewVar("")
	before := calls
	if err := checkCollectEventConfigurationCooldown(context.Background(), event, time.Now(), read); err != nil || calls != before {
		t.Fatal("new content/success queried stale cooldown")
	}
}

func TestCollectConfigurationCooldownDoesNotDelayOtherDependency(t *testing.T) {
	for _, sibling := range []error{nil, gerror.New("采集资料标题为空"), newCollectProcessRetryError(time.Second, "等待验证"), context.DeadlineExceeded} {
		r := executeCollectRuleBranches(context.Background(), []gdb.Record{{"id": g.NewVar(1)}, {"id": g.NewVar(2)}}, func(_ context.Context, row gdb.Record) (bool, string, error) {
			if row["id"].Int() == 1 {
				return false, "", gerror.New("采集规则未配置目标频道")
			}
			return sibling == nil, "", sibling
		})
		want := time.Duration(0)
		if sibling == nil || collectTerminalProcessingFailure(sibling) {
			want = 5 * time.Minute
		}
		if r.configurationCooldownDelay() != want {
			t.Fatalf("unexpected shared cooldown %s for %v", r.configurationCooldownDelay(), sibling)
		}
	}
}
