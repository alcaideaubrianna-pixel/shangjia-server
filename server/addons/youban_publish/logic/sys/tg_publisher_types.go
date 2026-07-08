package sys

import "strings"

const (
	telegramPublishBizProfile = "profile"
	telegramPublishBizCollect = "collect"
)

type telegramPublishRequest struct {
	TaskId                 int64
	OperationNo            string
	OperationPrefix        string
	AllowCreateOperationNo bool
}

func (req telegramPublishRequest) normalizedOperationPrefix() string {
	prefix := strings.TrimSpace(req.OperationPrefix)
	if prefix != "" {
		return prefix
	}
	return telegramPublishBizProfile
}
