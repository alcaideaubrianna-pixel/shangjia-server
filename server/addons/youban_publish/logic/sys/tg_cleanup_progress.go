package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
)

// Cleanup is also shipped without the operation-progress extension. Keep that
// extension behind its capability boundary instead of referencing an optional
// concrete method: local untracked files must not be required by a release.
type telegramCleanupProgressSettler interface {
	settleProfileCleanupProgress(context.Context, telegramJobRecord, tgQueuePayload) error
}

func settleTelegramCleanupProgress(ctx context.Context, target interface{}, job telegramJobRecord, payload tgQueuePayload) error {
	if settler, ok := target.(telegramCleanupProgressSettler); ok {
		return settler.settleProfileCleanupProgress(ctx, job, payload)
	}
	// Legacy jobs continue through finishTelegramCleanupProfile. A job that
	// explicitly asks for the extension must not acknowledge its receipt when
	// that extension is absent (e.g. an incomplete or rolled-back release).
	if strings.TrimSpace(payload.ProgressOperationNo) != "" {
		return gerror.New("资料操作进度模块不可用，无法确认清理完成，请部署完整版本后重试")
	}
	return nil
}
