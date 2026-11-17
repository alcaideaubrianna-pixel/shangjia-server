package domain

// EventID identifies a persisted collector event.
type EventID int64

// DeliveryID identifies a persisted collector delivery.
type DeliveryID int64

// TenantID identifies a tenant.
type TenantID int64

// AccountID identifies a Telegram account. Zero means a bot-scoped delivery.
type AccountID int64

// SourceID identifies a collector source.
type SourceID int64

// ChannelID identifies a collector channel relation, never a Telegram chat.
type ChannelID int64

// TelegramChatID identifies a remote Telegram chat. Channel/supergroup IDs are
// negative; user/private chat IDs are positive. Zero means no chat is present.
type TelegramChatID int64

// MessageID identifies a Telegram source message.
type MessageID int64

// ProfileID identifies a collected content profile.
type ProfileID int64

// DispatchID identifies a collector dispatch record.
type DispatchID int64

// TGJobID identifies a Telegram publish job created for a collected profile.
type TGJobID int64

func (id EventID) Valid() bool        { return id > 0 }
func (id DeliveryID) Valid() bool     { return id > 0 }
func (id TenantID) Valid() bool       { return id > 0 }
func (id AccountID) Valid() bool      { return id >= 0 }
func (id SourceID) Valid() bool       { return id > 0 }
func (id ChannelID) Valid() bool      { return id > 0 }
func (id TelegramChatID) Valid() bool { return id != 0 }
func (id MessageID) Valid() bool      { return id > 0 }
