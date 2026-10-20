package sys

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hibiken/asynq"
)

func TestCollectChannelPurgeProgressTruthful(t *testing.T) {
	snapshot := collectChannelPurgeNotification{Title: "A<频道>", ChannelID: "-100123", Suffix: "代理", ProfileCount: 3, MessageCount: 5, Link: "https://t.me/example"}
	for _, tc := range []struct {
		name             string
		pending, blocked int
		success          bool
	}{
		{"all removed", 0, 0, true}, {"pending", 2, 0, false}, {"bot cannot delete", 0, 2, false}, {"both", 1, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := collectChannelPurgeProgressText(snapshot, tc.pending, tc.blocked)
			if strings.Contains(text, "删除成功") != tc.success {
				t.Fatalf("incorrect result: %s", text)
			}
			for _, want := range []string{"A&lt;频道&gt;", "https://t.me/example", "3 条"} {
				if !strings.Contains(text, want) {
					t.Fatalf("missing %q: %s", want, text)
				}
			}
			if !tc.success && !strings.Contains(text, "消息清理未完成") {
				t.Fatal(text)
			}
			snapshot.RuleOnly = true
			ruleText := collectChannelPurgeProgressText(snapshot, tc.pending, tc.blocked)
			if !strings.Contains(ruleText, "其他规则不受影响") {
				t.Fatalf("rule-only purge claims to disable the entire channel: %s", ruleText)
			}
			snapshot.RuleOnly = false
		})
	}
}

func TestCollectChannelPurgeRetrySnapshot(t *testing.T) {
	original := collectChannelPurgeQueuePayload{TenantID: 1, AccountID: 2, RuleID: 3, ChannelID: "-100123", OperationID: "operation", NotifyChatID: "123", ProfileIDs: []int64{7, 8}, JobIDs: []int64{9}, MessageCount: 4}
	body, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeCollectChannelPurgeQueuePayload(asynq.NewTask(tgTaskTypeCollectChannelPurge, body))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.OperationID != original.OperationID || decoded.NotifyChatID != original.NotifyChatID || len(decoded.ProfileIDs) != 2 || len(decoded.JobIDs) != 1 || decoded.MessageCount != 4 {
		t.Fatalf("snapshot lost: %+v", decoded)
	}
	if decoded.AllRules {
		t.Fatal("legacy/default payload widened to all rules")
	}
	original.AllRules = true
	body, err = json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err = decodeCollectChannelPurgeQueuePayload(asynq.NewTask(tgTaskTypeCollectChannelPurge, body))
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.AllRules {
		t.Fatal("explicit all-rule scope was lost on retry")
	}
}
