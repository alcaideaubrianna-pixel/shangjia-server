// Package collectobserve carries collection observability identity without I/O.
// It must not be used for authorization, scheduling, or business decisions.
package collectobserve

import (
	"context"
	"time"
)

type contextKey struct{}

// Metadata is copied with the context, so concurrent rule branches cannot
// mutate each other's identity. ID fields belong in logs/traces, not metrics.
type Metadata struct {
	TenantID        int64
	AccountID       int64
	SourceID        int64
	BotID           int64
	TgAccountID     int64
	EventID         int64
	UpstreamEventID int64
	DeliveryID      int64
	RuleID          int64
	ProfileID       int64
	ChatID          string
	SourceType      string
	Mode            string
	// ReceivedAt is the local ingress instant, never Telegram's message date
	// or an unverified database wall-clock timestamp.
	ReceivedAt time.Time
}

func From(ctx context.Context) Metadata {
	if ctx != nil {
		if value, ok := ctx.Value(contextKey{}).(Metadata); ok {
			return value
		}
	}
	return Metadata{}
}

// With enriches identity using already loaded records. Empty values do not
// erase known metadata. Cross-event callers must bind the new event's fields.
func With(ctx context.Context, next Metadata) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	value := From(ctx)
	if next.EventID > 0 && value.EventID > 0 && next.EventID != value.EventID {
		value.RuleID, value.ProfileID = 0, 0
		value.UpstreamEventID, value.DeliveryID = 0, 0
		value.Mode, value.ReceivedAt = "unknown", time.Time{}
	}
	pairs := [][2]*int64{
		{&value.TenantID, &next.TenantID}, {&value.AccountID, &next.AccountID},
		{&value.SourceID, &next.SourceID}, {&value.BotID, &next.BotID},
		{&value.TgAccountID, &next.TgAccountID}, {&value.EventID, &next.EventID},
		{&value.UpstreamEventID, &next.UpstreamEventID}, {&value.DeliveryID, &next.DeliveryID},
		{&value.RuleID, &next.RuleID}, {&value.ProfileID, &next.ProfileID},
	}
	for _, pair := range pairs {
		if *pair[1] > 0 {
			*pair[0] = *pair[1]
		}
	}
	if next.ChatID != "" {
		value.ChatID = next.ChatID
	}
	if next.SourceType != "" {
		value.SourceType = SourceType(next.SourceType)
	}
	if next.Mode != "" {
		value.Mode = Mode(next.Mode)
	}
	if !next.ReceivedAt.IsZero() {
		value.ReceivedAt = next.ReceivedAt
	}
	return context.WithValue(ctx, contextKey{}, value)
}

func SourceType(value string) string {
	switch value {
	case "bot", "account":
		return value
	default:
		return "unknown"
	}
}

func Mode(value string) string {
	switch value {
	case "realtime", "history":
		return value
	default:
		return "unknown"
	}
}

func ModeForHistory(history bool) string {
	if history {
		return "history"
	}
	return "realtime"
}
