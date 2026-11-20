package sysin

import "github.com/gogf/gf/v2/os/gtime"

const ProfileStatusReviewPending = 3

type BotProfileReviewSubmit struct {
	BotID, TenantID, AccountID   int64
	ProfileNo, ChatID, Applicant string
}

type BotProfileReview struct {
	Id, TenantId, BotId, OwnerAccountId, AccountId, ProfileId int64
	ProfileNo, Title, Applicant, ChatId, Status               string
	CreatedAt                                                 *gtime.Time
}
