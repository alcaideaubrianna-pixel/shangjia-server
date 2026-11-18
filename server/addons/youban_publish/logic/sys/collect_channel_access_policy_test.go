package sys

import "testing"

func TestDecideCollectChannelAccessDefaultsToFirstAllow(t *testing.T) {
	for _, chat := range []string{"-1004", "4"} {
		decision := decideCollectChannelAccess(collectChannelAccessPolicyInput{
			TenantID: 1, AccountID: 2, SourceID: 3, CanonicalChat: chat,
		})
		if !decision.Allowed || !decision.Discover {
			t.Fatalf("missing relation decision = %#v, want allowed and discover", decision)
		}
		// Both Telegram representations must identify the same subscription.
		if decision.RelationKey != "1/2/3/4" {
			t.Fatalf("chat %q relation key = %q", chat, decision.RelationKey)
		}
	}
}

func TestDecideCollectChannelAccessPreservesExplicitBlocks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status string
		want   bool
	}{
		{"disabled", "disabled", false},
		{"enabled", "enabled", true},
		{"legacy pending", "pending", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decision := decideCollectChannelAccess(collectChannelAccessPolicyInput{
				TenantID: 1, AccountID: 2, SourceID: 3, CanonicalChat: "-1004",
				RelationExists: true,
				Status:         tc.status,
			})
			if decision.Allowed != tc.want || decision.Discover {
				t.Fatalf("blocked relation decision = %#v", decision)
			}
		})
	}
}

func TestBotCollectScopeDoesNotReplaceSpecificRelation(t *testing.T) {
	decision := decideCollectChannelAccess(collectChannelAccessPolicyInput{
		TenantID: 1, AccountID: 2, SourceID: 3, CanonicalChat: "-1004",
		BotCollectScope: "all", RelationExists: true,
		Status: "disabled",
	})
	if decision.Allowed {
		t.Fatal("bot_collect_scope=all bypassed a disabled relation")
	}
}
