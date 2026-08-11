package sysin

import "time"

const (
	SourceTypeBot     = "bot"
	SourceTypeAccount = "account"

	EventPriorityNormal = 0
	EventPriorityUrgent = 100

	EventStatusReceived    = "received"
	EventStatusProcessing  = "processing"
	EventStatusReady       = "ready"
	EventStatusFailedRetry = "failed_retry"
	EventStatusDead        = "dead"

	DeliveryStatusPending     = "pending"
	DeliveryStatusProcessing  = "processing"
	DeliveryStatusCompleted   = "completed"
	DeliveryStatusFailedRetry = "failed_retry"
	DeliveryStatusDead        = "dead"
)

type RawUpdateEvent struct {
	EventID    string    `json:"eventId"`
	TenantID   int64     `json:"tenantId"`
	SourceID   int64     `json:"sourceId"`
	SourceType string    `json:"sourceType"`
	BotKey     string    `json:"botKey,omitempty"`
	AccountID  int64     `json:"accountId,omitempty"`
	ChatID     int64     `json:"chatId,omitempty"`
	MessageID  int64     `json:"messageId,omitempty"`
	UpdateID   int64     `json:"updateId,omitempty"`
	Priority   int       `json:"priority"`
	RawUpdate  []byte    `json:"rawUpdate"`
	ReceivedAt time.Time `json:"receivedAt"`
	TraceID    string    `json:"traceId,omitempty"`
}

type EventTask struct {
	EventID  int64  `json:"eventId"`
	EventKey string `json:"eventKey"`
	Priority int    `json:"priority"`
}

type CollectorDelivery struct {
	ID              int64  `json:"id"`
	DeliveryKey     string `json:"deliveryKey"`
	TenantID        int64  `json:"tenantId"`
	EventID         int64  `json:"eventId"`
	SourceID        int64  `json:"sourceId"`
	SourceType      string `json:"sourceType"`
	SourceChatID    string `json:"sourceChatId"`
	SourceMessageID int64  `json:"sourceMessageId"`
	RawText         string `json:"rawText"`
	RawUpdate       []byte `json:"rawUpdate"`
}
