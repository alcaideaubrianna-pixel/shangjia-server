package sys

import (
	"context"
	"testing"
)

// TestStreamMediaFailureProtectsPartialSends 验证媒体失败不会终止已发送部分或待对账任务。
func TestStreamMediaFailureDoesNotBlockProfilePublish(t *testing.T) {
	publish := NewSysPublish()
	for _, test := range []struct {
		status string
		phase  string
	}{
		{telegramJobStatusPending, ""},
		{telegramJobStatusRetryWait, ""},
		{telegramJobStatusSending, ""},
		{telegramJobStatusConfirming, ""},
		{telegramJobStatusSent, ""},
		{telegramJobStatusPending, telegramSendPhaseDisplayConfirmed},
		{telegramJobStatusRetryWait, telegramSendPhaseVerifySending},
	} {
		t.Run(test.status+"/"+test.phase, func(t *testing.T) {
			settled, err := publish.settleStreamFailedMediaJob(context.Background(), telegramJobRecord{Status: JobStatus(test.status), SendPhase: SendPhase(test.phase)})
			if err != nil || settled {
				t.Fatalf("settled=%v err=%v", settled, err)
			}
		})
	}
}
