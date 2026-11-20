package sysin

const (
	UploaderReviewRequestNotice = "uploader_review_request"
	UploaderReviewResultNotice  = "uploader_review_result"
	UploaderAccountStatusNotice = "uploader_account_status"
)

// UploaderReviewNotice carries the reference needed to resolve the recipient
// again at delivery time. It contains no Bot token or account credentials.
type UploaderReviewNotice struct {
	Kind      string
	NoticeID  int64
	TenantID  int64
	AccountID int64
	ReviewID  int64
	Status    int
}
