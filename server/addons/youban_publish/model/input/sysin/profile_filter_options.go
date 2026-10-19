package sysin

type ProfileFilterOptionsInp struct {
	AccountId int64 `json:"accountId" dc:"筛选账号ID"`
}

type ProfileFilterOptionsModel struct {
	Accounts       []*AccountOptionModel       `json:"accounts"`
	SourceChannels []*CollectSourceOptionModel `json:"sourceChannels"`
	PushChannels   []*AccountOptionModel       `json:"pushChannels"`
}
