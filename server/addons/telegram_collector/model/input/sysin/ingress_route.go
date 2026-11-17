package sysin

import "hotgo/addons/telegram_collector/domain"

type IngressLane uint8

const (
	IngressLaneRealtime IngressLane = iota + 1
	IngressLaneBackground
)

type IngressDeferralReason string

const (
	IngressDeferralNone     IngressDeferralReason = ""
	IngressDeferralApproval IngressDeferralReason = "approval_blocked"
	IngressDeferralSelfLoop IngressDeferralReason = "self_loop_publish_target"
)

func (r IngressDeferralReason) Valid() bool {
	return r == IngressDeferralApproval || r == IngressDeferralSelfLoop
}

type BotIngressScope struct {
	TenantID domain.TenantID
	SourceID domain.SourceID
	ChatID   domain.TelegramChatID
}

func (s BotIngressScope) Valid() bool {
	return s.TenantID.Valid() && s.SourceID.Valid() && s.ChatID.Valid()
}

type BotIngressDecision struct {
	Lane   IngressLane
	Reason IngressDeferralReason
}

func (d BotIngressDecision) Defer() bool { return d.Lane == IngressLaneBackground && d.Reason.Valid() }
