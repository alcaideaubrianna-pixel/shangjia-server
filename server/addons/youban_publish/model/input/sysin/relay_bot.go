package sysin

// RelayOwnedBotModel is internal cross-plugin data, not an HTTP response contract.
type RelayOwnedBotModel struct {
	BotModel
	AccountId int64 `json:"accountId"`
	CreatedBy int64 `json:"createdBy"`
}
