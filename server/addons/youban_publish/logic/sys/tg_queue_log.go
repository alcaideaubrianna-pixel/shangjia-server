package sys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"

	"hotgo/utility/ownedlease"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/gtrace"
)

var telegramQueueLogInstance, _ = os.Hostname()

// Redact before returning to callers: Redis driver errors can include EVAL arguments.
func telegramQueueRedactError(err error, secrets ...string) error {
	return ownedlease.RedactError(err, secrets...)
}

// Diagnostic identity only. Never log the token that authorizes queue writes.
func telegramQueueClaimID(token string) string {
	if token == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:8])
}

func telegramQueueLogFieldAllowed(key string) bool {
	switch key {
	case "channelId", "jobId", "profileId", "tenantId", "accountId", "operationNo", "executionId",
		"claimId", "queue", "stage", "reason", "action", "result", "durationMs",
		"leaseMs", "score", "priority", "availableAtMs", "retryAtMs", "scanned",
		"recovered", "failed", "pending", "cursor", "nextCursor", "limit", "step",
		"visited", "nextStep", "cancelled", "dueCount", "activeRestored":
		return true
	}
	return false
}

// Copy approved scalar fields only; callers cannot override envelope metadata
// or accidentally include tokens, complete queue payloads or media content.
func telegramQueueLogFields(ctx context.Context, event string, fields g.Map, err error) g.Map {
	data := g.Map{}
	copyFields := func(source g.Map) {
		for key, value := range source {
			if !telegramQueueLogFieldAllowed(key) {
				continue
			}
			switch value.(type) {
			case string, bool, int, int32, int64, uint, uint32, uint64, float64:
				data[key] = value
			}
		}
	}
	if base, ok := ctx.Value(telegramPublishTraceContextKey{}).(g.Map); ok {
		copyFields(base)
	}
	copyFields(fields)
	data["schemaVersion"] = 1
	data["log_type"] = "telegram_queue"
	data["event"] = event
	data["traceId"] = gtrace.GetTraceID(ctx)
	data["instance"] = telegramQueueLogInstance
	data["pid"] = os.Getpid()
	if _, exists := data["result"]; !exists {
		data["result"] = "ok"
	}
	if err != nil {
		data["result"] = "error"
		data["error"] = telegramPublishLogError(err)
	}
	return data
}

func logTelegramQueueEvent(ctx context.Context, event string, fields g.Map, err error) {
	data := telegramQueueLogFields(ctx, event, fields, err)
	payload, marshalErr := json.Marshal(data)
	if marshalErr != nil {
		// Scalar fields are normally JSON-safe; discard invalid numeric input.
		data = telegramQueueLogFields(ctx, event, nil, err)
		payload, _ = json.Marshal(data)
	}
	if err != nil || data["result"] == "lost" || data["result"] == "rejected" {
		g.Log().Warning(ctx, string(payload))
		return
	}
	g.Log().Info(ctx, string(payload))
}
