package sys

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func TestTelegramVideoFileIDDoesNotPrepareSourceMetadata(t *testing.T) {
	// No attachment or source exists. Reuse must work without PG, downloading,
	// or ffprobe; an empty optional metadata result leaves Telegram's intact.
	media := &telegramMediaItem{MediaType: "video", TgFileId: "BAAC-cached", AttachmentId: -1}
	if got := (&sSysPublish{}).telegramVideoMeta(context.Background(), media); got != (telegramVideoMeta{}) {
		t.Fatalf("unexpected metadata: %+v", got)
	}
	media.VideoWidth, media.VideoHeight, media.VideoDuration = 1920, 1080, 12
	if got := (&sSysPublish{}).telegramVideoMeta(context.Background(), media); got != (telegramVideoMeta{1920, 1080, 12}) {
		t.Fatalf("known metadata lost: %+v", got)
	}
}

func TestTelegramVideoFileIDSingleSend(t *testing.T) {
	for _, cancelSend := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "unknown_no_resend"}[cancelSend], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var calls atomic.Int32
			releaseHandler := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
					return
				}
				defer r.MultipartForm.RemoveAll()
				if r.URL.Path != "/botlocal-test/sendVideo" || r.FormValue("video") != "BAAC-cached" || len(r.MultipartForm.File) != 0 {
					t.Errorf("unexpected request: %s %+v", r.URL.Path, r.MultipartForm)
				}
				if cancelSend {
					cancel()
					<-releaseHandler
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":123,"date":1,"chat":{"id":-1001,"type":"channel"},"video":{"file_id":"BAAC-cached","file_unique_id":"unique","width":640,"height":480,"duration":5}}}`)
			}))
			defer server.Close()
			bot, err := tgbot.New("local-test", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
			if err != nil {
				t.Fatal(err)
			}
			media := &telegramMediaItem{Id: 42, MediaType: "video", TgFileId: "BAAC-cached", Purpose: "verify"}
			messages, err := (&sSysPublish{}).sendTelegramSingleMedia(ctx, bot, "-1001", "verify", "verify", media)
			close(releaseHandler)
			if cancelSend {
				if err == nil {
					t.Fatal("canceled send must remain uncertain")
				}
			} else if err != nil || len(messages) != 1 || messages[0].MessageId != 123 {
				t.Fatalf("send response lost: %+v err:%v", messages, err)
			}
			if calls.Load() != 1 {
				t.Fatalf("requests=%d, want one", calls.Load())
			}
		})
	}
}

func TestTelegramVideoFileIDAlbumNeedsNoSource(t *testing.T) {
	media := &telegramMediaItem{MediaType: "video", TgFileId: "BAAC-cached", Purpose: "verify"}
	item, closers, err := (&sSysPublish{}).telegramInputMediaItem(context.Background(), media, "verify")
	defer closeTelegramMediaFiles(closers)
	if err != nil {
		t.Fatal(err)
	}
	video, ok := item.(*models.InputMediaVideo)
	if !ok || video.Media != media.TgFileId || video.Caption != "verify" || !video.SupportsStreaming || len(closers) != 0 {
		t.Fatalf("reuse changed album media: %#v", item)
	}
}
