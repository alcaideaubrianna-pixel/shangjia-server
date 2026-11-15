package sys

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestCollectProcessSourceQueueIsolation(t *testing.T) {
	first := collectProcessQueuePayload{
		TenantId: 78, AccountId: 0, SourceId: 351, SourceChatId: "-100123",
		EventId: 1, OutboxOwner: "owner-1",
	}
	sameSource := first
	sameSource.EventId = 999
	sameSource.OutboxOwner = "owner-2"
	if got, want := collectProcessQueueName(first), collectProcessQueueName(sameSource); got != want {
		t.Fatalf("同一 BOT 来源必须固定到同一队列: got=%s want=%s", got, want)
	}
	if got := collectProcessQueueName(first); got != collectProcessSourceQueueName(first) {
		t.Fatalf("BOT 来源没有使用来源分片队列: %s", got)
	}
	history := first
	history.History = true
	if got := collectProcessQueueName(history); got == collectProcessQueueName(first) {
		t.Fatalf("BOT 历史任务不能与实时任务共用队列: %s", got)
	}
}

func TestCollectProcessSourceQueuesRegistered(t *testing.T) {
	payload := collectProcessQueuePayload{TenantId: 56, AccountId: 561, SourceId: 1370, EventId: 1}
	realtime := collectProcessQueueName(payload)
	recovery := collectProcessQueueName(collectProcessQueuePayload{TenantId: 56, AccountId: 561, SourceId: 1370, EventId: 1, Recovery: true, History: true})
	if realtime == recovery || realtime == "" || recovery == "" {
		t.Fatalf("source realtime/recovery queues are not isolated: realtime=%s recovery=%s", realtime, recovery)
	}
}

func TestCollectProcessSourceWakeKeyIsolatedByMember(t *testing.T) {
	cases := []struct {
		name    string
		member  string
		wantKey string
	}{
		{name: "first source", member: "78:615:351", wantKey: "youban_publish:collect:source:wake:lock:78:615:351"},
		{name: "second source", member: "78:615:352", wantKey: "youban_publish:collect:source:wake:lock:78:615:352"},
		{name: "different account", member: "78:616:351", wantKey: "youban_publish:collect:source:wake:lock:78:616:351"},
	}

	seen := make(map[string]string, len(cases))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := collectProcessSourceWakeKey(tc.member)
			if got != tc.wantKey {
				t.Fatalf("source wake key = %q, want %q", got, tc.wantKey)
			}
			if previous, ok := seen[got]; ok && previous != tc.member {
				t.Fatalf("different source members share wake key: %q and %q", previous, tc.member)
			}
			seen[got] = tc.member
		})
	}
}

func TestCollectProcessSourceMemberShape(t *testing.T) {
	valid := []string{"78:615:351", "1:0:1", "900:123456:987654"}
	for _, member := range valid {
		parts := strings.Split(member, ":")
		if len(parts) != 3 {
			t.Fatalf("valid source member %q has %d parts", member, len(parts))
		}
		for _, part := range parts {
			if part == "" {
				t.Fatalf("valid source member %q contains an empty part", member)
			}
		}
	}

	invalid := []string{"", "78:615", "78:615:351:extra", "tenant:615:351", "78::351", "78:615:0"}
	for _, member := range invalid {
		parts := strings.Split(member, ":")
		if len(parts) != 3 {
			continue
		}
		tenantID, tenantErr := strconv.ParseInt(parts[0], 10, 64)
		accountID, accountErr := strconv.ParseInt(parts[1], 10, 64)
		sourceID, sourceErr := strconv.ParseInt(parts[2], 10, 64)
		if tenantErr == nil && accountErr == nil && sourceErr == nil && tenantID > 0 && accountID >= 0 && sourceID > 0 {
			t.Fatalf("invalid source member %q was accepted", member)
		}
	}
}

func TestCollectProcessDispatcherDoesNotUseGlobalWakeQueue(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	workerPath := filepath.Join(filepath.Dir(currentFile), "tg_queue_worker.go")
	content, err := os.ReadFile(workerPath)
	if err != nil {
		t.Fatalf("read dispatcher source: %v", err)
	}
	source := string(content)

	if strings.Contains(source, "collectProcessWakeQueueKey") {
		t.Fatalf("dispatcher must not reference the global wake queue key collectProcessWakeQueueKey")
	}
	if strings.Contains(source, `client.Do(ctx, "LPUSH", collectProcessSourceWakeKey(member)`) ||
		strings.Contains(source, `client.Do(ctx, "RPOP", collectProcessSourceWakeKey(member)`) {
		t.Fatalf("dispatcher must use source wake queues without reusing the source wake key as a Redis string and list")
	}
	if !strings.Contains(source, "collectProcessSourceWakeKey(member)") {
		t.Fatal("dispatcher must derive wake storage from the tenant/account/source member")
	}
}
