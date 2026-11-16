package sys

import (
	"fmt"
	"strings"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type collectChannelAccessPolicyInput struct {
	TenantID        int64
	AccountID       int64
	SourceID        int64
	CanonicalChat   string
	BotCollectScope string
	RelationExists  bool
	CollectEnabled  int
	Status          string
	ApprovalStatus  string
}

type collectChannelAccessPolicyDecision struct {
	Allowed     bool
	Discover    bool
	RelationKey string
}

func decideCollectChannelAccess(in collectChannelAccessPolicyInput) collectChannelAccessPolicyDecision {
	key := collectChannelAccessRelationKey(in.TenantID, in.AccountID, in.SourceID, in.CanonicalChat)
	if !in.RelationExists {
		return collectChannelAccessPolicyDecision{
			Allowed:     true,
			Discover:    true,
			RelationKey: key,
		}
	}
	status := strings.ToLower(strings.TrimSpace(in.Status))
	approval := strings.ToLower(strings.TrimSpace(in.ApprovalStatus))
	if status == sysin.CollectAccountChannelDisabled || approval == "rejected" ||
		status == sysin.CollectAccountChannelPending {
		return collectChannelAccessPolicyDecision{RelationKey: key}
	}
	return collectChannelAccessPolicyDecision{
		Allowed: in.CollectEnabled == 1 &&
			(status == sysin.CollectAccountChannelEnabled || status == "approved") &&
			approval != "rejected",
		RelationKey: key,
	}
}

func collectChannelAccessRelationKey(tenantID, accountID, sourceID int64, canonicalChat string) string {
	return fmt.Sprintf("%d/%d/%d/%s", tenantID, accountID, sourceID, canonicalCollectSourceChatID(canonicalChat))
}
