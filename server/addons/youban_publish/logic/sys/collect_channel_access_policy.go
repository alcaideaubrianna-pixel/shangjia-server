package sys

import (
	"fmt"
	"os"
	"strings"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const collectChannelDefaultStatusEnv = "YOUBAN_COLLECT_CHANNEL_DEFAULT_STATUS"

// This setting applies to newly discovered channel relations only.
func defaultCollectChannelStatus() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(collectChannelDefaultStatusEnv))) {
	case "", sysin.CollectAccountChannelEnabled:
		return sysin.CollectAccountChannelEnabled
	default:
		return sysin.CollectAccountChannelDisabled
	}
}

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

func collectChannelStatusEnabled(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), sysin.CollectAccountChannelEnabled)
}

func decideCollectChannelAccess(in collectChannelAccessPolicyInput) collectChannelAccessPolicyDecision {
	key := collectChannelAccessRelationKey(in.TenantID, in.AccountID, in.SourceID, in.CanonicalChat)
	if !in.RelationExists {
		return collectChannelAccessPolicyDecision{
			Allowed:     botCollectChannelRelationEnabled(defaultCollectChannelStatus()),
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
