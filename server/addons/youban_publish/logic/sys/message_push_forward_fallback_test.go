package sys

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

func TestMessageTemplateForwardFallback(t *testing.T) {
	for _, tt := range []struct {
		name     string
		err      error
		messages bool
		upload   bool
	}{
		{"source private", fmt.Errorf("rpcDoRequest: %w", tgerr.New(400, "CHANNEL_PRIVATE")), false, true},
		{"source message deleted", tgerr.New(400, "MESSAGE_ID_INVALID"), false, true},
		{"forward restricted", tgerr.New(400, "CHAT_FORWARDS_RESTRICTED"), false, true},
		{"success", nil, true, false},
		{"empty response", nil, false, false},
		{"target forbidden", tgerr.New(403, "CHAT_WRITE_FORBIDDEN"), false, false},
		{"rate limit", tgerr.New(420, "FLOOD_WAIT_60"), false, false},
		{"timeout", context.DeadlineExceeded, false, false},
		{"canceled", context.Canceled, false, false},
		{"transport lost", errors.New("connection reset by peer"), false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			forwardCalls, uploadCalls := 0, 0
			want := []*telegramSentMessage{{}, {}}
			got, err := sendMessageTemplateWithForwardFallback(context.Background(), &messageTemplateForwardSource{MessageIds: []int{51142, 51143}}, func() ([]*telegramSentMessage, error) {
				forwardCalls++
				if tt.messages {
					return want, tt.err
				}
				return nil, tt.err
			}, func() ([]*telegramSentMessage, error) { uploadCalls++; return want, nil }, 18, "test", &tg.InputPeerChannel{ChannelID: 4471486687})
			if forwardCalls != 1 || (uploadCalls == 1) != tt.upload || uploadCalls > 1 {
				t.Fatalf("forward=%d upload=%d", forwardCalls, uploadCalls)
			}
			if tt.messages || tt.upload {
				if err != nil || len(got) != 2 {
					t.Fatalf("messages=%v err=%v", got, err)
				}
			} else if err == nil {
				t.Fatal("expected error without a second send")
			}
		})
	}
}

func TestMessageTemplateForwardFallbackUploadFailure(t *testing.T) {
	want := errors.New("media download failed")
	_, err := sendMessageTemplateWithForwardFallback(context.Background(), &messageTemplateForwardSource{MessageIds: []int{1}}, func() ([]*telegramSentMessage, error) { return nil, tgerr.New(400, "CHANNEL_PRIVATE") }, func() ([]*telegramSentMessage, error) { return nil, want }, 18, "test", nil)
	if !errors.Is(err, want) {
		t.Fatalf("got %v", err)
	}
}
