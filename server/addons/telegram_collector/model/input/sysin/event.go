package sysin

import "time"

const (
	SourceTypeBot     = "bot"
	SourceTypeAccount = "account"

	EventPriorityNormal   = 0
	EventPriorityRealtime = 50
	EventPriorityUrgent   = 100

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

type AccountMessageEvent struct {
	TenantID        int64                `json:"tenantId"`
	AccountID       int64                `json:"accountId"`
	SourceID        int64                `json:"sourceId"`
	TgAccountID     int64                `json:"tgAccountId"`
	SourceChatID    string               `json:"sourceChatId"`
	SourceMessageID int64                `json:"sourceMessageId"`
	SourceGroupedID string               `json:"sourceGroupedId,omitempty"`
	SourceUniqueKey string               `json:"sourceUniqueKey"`
	RawText         string               `json:"rawText"`
	Media           []CollectorMediaItem `json:"media,omitempty"`
	ReceivedAt      time.Time            `json:"receivedAt"`
	TraceID         string               `json:"traceId,omitempty"`
}

type CollectorMediaItem struct {
	Type                string `json:"type"`
	Purpose             string `json:"purpose,omitempty"`
	FileID              string `json:"fileId"`
	FileURL             string `json:"fileUrl,omitempty"`
	StoragePath         string `json:"storagePath,omitempty"`
	PosterURL           string `json:"posterUrl,omitempty"`
	FileMD5             string `json:"fileMd5,omitempty"`
	FilePHash           string `json:"filePhash,omitempty"`
	SourceKind          string `json:"sourceKind,omitempty"`
	SourceMediaID       int64  `json:"sourceMediaId,omitempty"`
	SourceAccessHash    int64  `json:"sourceAccessHash,omitempty"`
	SourceFileReference []byte `json:"sourceFileReference,omitempty"`
	SourceThumbSize     string `json:"sourceThumbSize,omitempty"`
	SourceMimeType      string `json:"sourceMimeType,omitempty"`
	SourceDCID          int    `json:"sourceDcId,omitempty"`
	SourceSize          int64  `json:"sourceSize,omitempty"`
	DebugMetaJSON       string `json:"debugMetaJson,omitempty"`
}

type CollectorDelivery struct {
	ID              int64                `json:"id"`
	DeliveryKey     string               `json:"deliveryKey"`
	TenantID        int64                `json:"tenantId"`
	AccountID       int64                `json:"accountId"`
	EventID         int64                `json:"eventId"`
	SourceID        int64                `json:"sourceId"`
	SourceType      string               `json:"sourceType"`
	TgAccountID     int64                `json:"tgAccountId,omitempty"`
	SourceChatID    string               `json:"sourceChatId"`
	SourceMessageID int64                `json:"sourceMessageId"`
	SourceGroupedID string               `json:"sourceGroupedId,omitempty"`
	SourceUniqueKey string               `json:"sourceUniqueKey,omitempty"`
	RawText         string               `json:"rawText"`
	Media           []CollectorMediaItem `json:"media,omitempty"`
	ReceivedAt      time.Time            `json:"receivedAt"`
	RawUpdate       []byte               `json:"rawUpdate"`
}
