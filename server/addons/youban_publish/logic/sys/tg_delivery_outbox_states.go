package sys

import (
	"strings"

	"hotgo/addons/youban_publish/model"
)

// TelegramAttemptStatus is the persisted status of one delivery attempt.
// Values intentionally match the existing database strings.
type TelegramAttemptStatus = model.TGAttemptStatus

const (
	TelegramAttemptStatusSending    TelegramAttemptStatus = model.TGAttemptStatusSending
	TelegramAttemptStatusWaiting    TelegramAttemptStatus = model.TGAttemptStatusAwaiting
	TelegramAttemptStatusConfirmed  TelegramAttemptStatus = model.TGAttemptStatusConfirmed
	TelegramAttemptStatusRetryWait  TelegramAttemptStatus = model.TGAttemptStatusRetryWait
	TelegramAttemptStatusSuperseded TelegramAttemptStatus = model.TGAttemptStatusSuperseded
)

func ParseTelegramAttemptStatus(value string) (TelegramAttemptStatus, bool) {
	return model.ParseTGAttemptStatus(strings.TrimSpace(value))
}

func (a telegramDeliveryAttempt) StatusValue() (TelegramAttemptStatus, bool) {
	return ParseTelegramAttemptStatus(a.Status)
}

// TelegramOutboxStatus is kept as a compatibility alias for the shared model.
type TelegramOutboxStatus = model.TGOutboxStatus

const (
	TelegramOutboxStatusPending    TelegramOutboxStatus = model.TGOutboxStatusPending
	TelegramOutboxStatusProcessing TelegramOutboxStatus = model.TGOutboxStatusProcessing
	TelegramOutboxStatusDelivered  TelegramOutboxStatus = model.TGOutboxStatusDelivered
)

func ParseTelegramOutboxStatus(value string) (TelegramOutboxStatus, bool) {
	return model.ParseTGOutboxStatus(strings.TrimSpace(value))
}
