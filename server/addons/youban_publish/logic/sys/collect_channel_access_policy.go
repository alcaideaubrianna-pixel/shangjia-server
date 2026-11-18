package sys

import (
	"fmt"
)

type collectChannelAccessPolicyInput struct {
	TenantID        int64
	AccountID       int64
	SourceID        int64
	CanonicalChat   string
	BotCollectScope string
	RelationExists  bool
	Status          string
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
	return collectChannelAccessPolicyDecision{
		Allowed:     botCollectChannelRelationEnabled(in.Status),
		RelationKey: key,
	}
}

func collectChannelAccessRelationKey(tenantID, accountID, sourceID int64, canonicalChat string) string {
	return fmt.Sprintf("%d/%d/%d/%s", tenantID, accountID, sourceID, canonicalCollectSourceChatID(canonicalChat))
}
