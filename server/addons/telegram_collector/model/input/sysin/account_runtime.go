package sysin

type AccountRuntimeBinding struct {
	AccountID int64
	Signature string
	Sources   []AccountRuntimeSource
	Payload   any
}

type AccountRuntimeSource struct {
	TenantID  int64
	AccountID int64
	SourceID  int64
	ChatID    string
}
