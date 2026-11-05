package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/internal/library/cache"
)

// profileOperationType identifies the user-visible lifecycle operation. The
// existing profile_state and tg_job tables remain the source of truth; this
// type only gives all entry points one vocabulary.
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

const profileOperationClaimTTL = 2 * time.Minute

func profileOperationClaimKey(tenantID, profileID int64, operation profileOperationType) string {
	_ = operation
	return fmt.Sprintf("youban_publish:profile_operation:lock:%d:%d", tenantID, profileID)
}

// claimProfileOperation prevents duplicate Bot/Web clicks from creating
// duplicate operations. It is deliberately short-lived: the durable state is
// still written to PostgreSQL and Telegram jobs remain idempotent by operation
// number.
func claimProfileOperation(ctx context.Context, tenantID, profileID int64, operation profileOperationType) (bool, error) {
	if tenantID <= 0 || profileID <= 0 || strings.TrimSpace(string(operation)) == "" {
		return false, gerror.New("资料操作参数不完整")
	}
	if !cache.Initialized() {
		return true, nil
	}
	return cache.Instance().SetIfNotExist(ctx, profileOperationClaimKey(tenantID, profileID, operation), 1, profileOperationClaimTTL)
}

func releaseProfileOperationClaim(ctx context.Context, tenantID, profileID int64) {
	if cache.Initialized() && tenantID > 0 && profileID > 0 {
		_, _ = cache.Instance().Remove(ctx, fmt.Sprintf("youban_publish:profile_operation:lock:%d:%d", tenantID, profileID))
	}
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
