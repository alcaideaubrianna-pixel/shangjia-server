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

// ChannelID identifies a collector channel relation.
type ChannelID int64

// MessageID identifies a Telegram source message.
type MessageID int64

func (id EventID) Valid() bool    { return id > 0 }
func (id DeliveryID) Valid() bool { return id > 0 }
func (id TenantID) Valid() bool   { return id > 0 }
func (id AccountID) Valid() bool  { return id >= 0 }
func (id SourceID) Valid() bool   { return id > 0 }
func (id ChannelID) Valid() bool  { return id > 0 }
func (id MessageID) Valid() bool  { return id > 0 }
