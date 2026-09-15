package sys

import (
	"errors"
	"strings"
	"testing"
)

func TestTerminalPublishRecoveryEligible(t *testing.T) {
	base := telegramJobRecord{Id: 11, ProfileId: 22, ChannelId: 33, OperationNo: "profile:22"}
	tests := []struct {
		name string
		job  telegramJobRecord
		err  error
		want bool
	}{
		{name: "recover exhausted temporary failure", job: base, err: errors.New("context deadline exceeded"), want: true},
		{name: "skip permanent channel failure", job: base, err: errors.New("Bad Request: chat not found"), want: false},
		{name: "skip message push", job: func() telegramJobRecord { job := base; job.OperationNo = "message_push:1"; return job }(), err: errors.New("timeout"), want: false},
		{name: "skip recursive recovery", job: func() telegramJobRecord { job := base; job.OperationNo = "auto-recover:11:22"; return job }(), err: errors.New("timeout"), want: false},
		{name: "skip incomplete job", job: telegramJobRecord{}, err: errors.New("timeout"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := terminalPublishRecoveryEligible(test.job, test.err); got != test.want {
				t.Fatalf("terminalPublishRecoveryEligible()=%v want=%v", got, test.want)
			}
		})
	}
}

func TestTerminalPublishRecoveryOperationNoIsStable(t *testing.T) {
	job := telegramJobRecord{Id: 11, ProfileId: 22}
	first := terminalPublishRecoveryOperationNo(job)
	second := terminalPublishRecoveryOperationNo(job)
	if first != second || !strings.HasPrefix(first, "auto-recover:11:22") {
		t.Fatalf("unexpected recovery operation number %q %q", first, second)
	}
}
