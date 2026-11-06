package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
)

// profileOperationType identifies the user-visible lifecycle operation. The
// profile_state and Telegram job tables are the source of truth; this type
// gives every entry point one vocabulary without introducing a process lock.
type profileOperationType string

const (
	profileOperationCreate          profileOperationType = "create"
	profileOperationPublish         profileOperationType = "publish"
	profileOperationUnpublish       profileOperationType = "unpublish"
	profileOperationDelete          profileOperationType = "delete"
	profileOperationUnpublishDelete profileOperationType = "unpublish_delete"
	profileOperationRepublish       profileOperationType = "republish"
	profileOperationEdit            profileOperationType = "edit"
)

// claimProfileOperation is retained as a compatibility shim for existing
// callers. Profile lifecycle locking is database-owned now: operation_no plus
// the profile/channel uniqueness constraints provide idempotency. A Redis
// request lock caused stale claims after process crashes and made valid retry
// requests look permanently busy.
func claimProfileOperation(ctx context.Context, tenantID, profileID int64, operation profileOperationType) (bool, error) {
	_ = ctx
	_ = operation
	if tenantID <= 0 || profileID <= 0 || strings.TrimSpace(string(operation)) == "" {
		return false, gerror.New("资料操作参数不完整")
	}
	return true, nil
}

func releaseProfileOperationClaim(ctx context.Context, tenantID, profileID int64) {
	_ = ctx
	_ = tenantID
	_ = profileID
}

func profileOperationTypeFromNo(operationNo string) profileOperationType {
	value := strings.ToLower(strings.TrimSpace(operationNo))
	switch {
	case strings.HasPrefix(value, "edit:"):
		return profileOperationEdit
	case strings.HasPrefix(value, "collect:"):
		return profileOperationPublish
	case strings.HasPrefix(value, "republish:"), strings.HasPrefix(value, "ai-republish:"):
		return profileOperationRepublish
	case strings.HasPrefix(value, "down:"):
		return profileOperationUnpublish
	default:
		return profileOperationPublish
	}
}
