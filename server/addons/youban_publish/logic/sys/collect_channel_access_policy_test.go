package sys

import "testing"

func TestDecideCollectChannelAccessDefaultsToFirstAllow(t *testing.T) {
	decision := decideCollectChannelAccess(collectChannelAccessPolicyInput{
		TenantID: 1, AccountID: 2, SourceID: 3, CanonicalChat: "-1004",
	})
	if !decision.Allowed || !decision.Discover {
		t.Fatalf("missing relation decision = %#v, want allowed and discover", decision)
	}
	if decision.RelationKey != "1/2/3/-1004" {
		t.Fatalf("relation key = %q", decision.RelationKey)
	}
}

func TestDecideCollectChannelAccessPreservesExplicitBlocks(t *testing.T) {
	for _, tc := range []struct {
		name           string
		status         string
		approvalStatus string
	}{
		{"disabled", "disabled", "approved"},
		{"rejected", "enabled", "rejected"},
		{"pending", "pending", "pending"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decision := decideCollectChannelAccess(collectChannelAccessPolicyInput{
				TenantID: 1, AccountID: 2, SourceID: 3, CanonicalChat: "-1004",
				RelationExists: true, CollectEnabled: 1,
				Status: tc.status, ApprovalStatus: tc.approvalStatus,
			})
			if decision.Allowed || decision.Discover {
				t.Fatalf("blocked relation decision = %#v", decision)
			}
		})
	}
}

func TestBotCollectScopeDoesNotReplaceSpecificRelation(t *testing.T) {
	decision := decideCollectChannelAccess(collectChannelAccessPolicyInput{
		TenantID: 1, AccountID: 2, SourceID: 3, CanonicalChat: "-1004",
		BotCollectScope: "all", RelationExists: true,
		CollectEnabled: 1, Status: "disabled", ApprovalStatus: "approved",
	})
	if decision.Allowed {
		t.Fatal("bot_collect_scope=all bypassed a disabled relation")
	}
}
