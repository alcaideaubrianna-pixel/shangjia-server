package sys

import (
	"context"
	"encoding/json"
	"github.com/go-telegram/bot/models"
	publishService "hotgo/addons/youban_publish/service"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tgbot "github.com/go-telegram/bot"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
)

func TestProfilePreviewAlbumReusesReferences(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if !strings.HasSuffix(r.URL.Path, "/sendMediaGroup") {
			t.Errorf("unexpected API %s", r.URL.Path)
		}
		if strings.Contains(r.Header.Get("Content-Type"), "multipart/") {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
			}
			if r.MultipartForm != nil && len(r.MultipartForm.File) != 0 {
				t.Error("existing references were reuploaded")
			}
		} else if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		var media []struct {
			Media string `json:"media"`
		}
		if err := json.Unmarshal([]byte(r.FormValue("media")), &media); err != nil {
			t.Error(err)
		}
		if len(media) != 2 || media[0].Media != "existing-photo" || media[1].Media != "existing-video" {
			t.Errorf("references=%+v", media)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":[{"message_id":1,"date":1,"chat":{"id":123,"type":"private"}},{"message_id":2,"date":1,"chat":{"id":123,"type":"private"}}]}`))
	}))
	defer server.Close()
	bot, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	err = NewSysBot().sendProfileMediaPurpose(context.Background(), context.Background(), bot, "123", []*publishsysin.MediaModel{
		{Id: 1, MediaType: "image", Purpose: "display", TgFileId: "existing-photo"},
		{Id: 2, MediaType: "video", Purpose: "display", TgFileId: "existing-video"},
	}, "display", "preview", false)
	if err != nil || requests != 1 {
		t.Fatalf("requests=%d err=%v", requests, err)
	}
}

func TestProfilePreviewAlbumDoesNotRetryAmbiguousFailure(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":504,"description":"Gateway timeout"}`))
	}))
	defer server.Close()
	bot, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	err = NewSysBot().sendProfileMediaPurpose(context.Background(), context.Background(), bot, "123", []*publishsysin.MediaModel{
		{Id: 1, MediaType: "image", Purpose: "display", TgFileId: "existing-photo"},
		{Id: 2, MediaType: "image", Purpose: "display", TgFileId: "existing-photo-2"},
	}, "display", "preview", false)
	if err == nil || requests != 1 {
		t.Fatalf("ambiguous failure retried: requests=%d err=%v", requests, err)
	}
}

// Stale/cross-bot file IDs must retain the existing upload fallback.
type previewCacheStub struct {
	publishService.ISysPublish
	path  string
	calls int
}

func (s *previewCacheStub) BotMediaCacheFile(ctx context.Context, in *publishsysin.BotMediaCacheFileInp) (*publishsysin.BotMediaCacheFileModel, error) {
	s.calls++
	return &publishsysin.BotMediaCacheFileModel{Path: s.path}, nil
}

func TestProfilePreviewStaleReferenceFallsBackToUpload(t *testing.T) {
	// The optional publish addon is not initialized by this isolated package.
	var previous publishService.ISysPublish
	func() {
		defer func() { _ = recover() }()
		previous = publishService.SysPublish()
	}()
	cache := &previewCacheStub{ISysPublish: previous, path: filepath.Join(t.TempDir(), "image.jpg")}
	if err := os.WriteFile(cache.path, []byte("fixture image bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	publishService.RegisterSysPublish(cache)
	t.Cleanup(func() { publishService.RegisterSysPublish(cache.ISysPublish) })
	requests, uploads := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: wrong file identifier/HTTP URL specified"}`))
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		if r.MultipartForm != nil {
			uploads += len(r.MultipartForm.File)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":123,"type":"private"}}}`))
	}))
	defer server.Close()
	bot, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	err = NewSysBot().sendProfileMediaPurpose(context.Background(), context.Background(), bot, "123", []*publishsysin.MediaModel{
		{Id: 1, MediaType: "image", Purpose: "display", TgFileId: "stale-reference"},
	}, "display", "preview", false)
	if err != nil || requests != 2 || uploads != 1 || cache.calls != 1 {
		t.Fatalf("requests=%d uploads=%d cache=%d err=%v", requests, uploads, cache.calls, err)
	}
}

func TestProfilePreviewCopyReferenceUsesAvailableURL(t *testing.T) {
	input, file, err := NewSysBot().profilePreviewInputMedia(context.Background(), &publishsysin.MediaModel{
		Id: 1, MediaType: "image", TgFileId: "copy:123:456", FileUrl: "https://example.test/photo.jpg",
	}, "")
	if err != nil || file != nil {
		t.Fatalf("unnecessary upload: file=%v err=%v", file, err)
	}
	photo, ok := input.(*models.InputMediaPhoto)
	if !ok || photo.Media != "https://example.test/photo.jpg" {
		t.Fatalf("unexpected input: %#v", input)
	}
}

func TestProfilePreviewAlbumUsesRemoteURLsWithoutDownloading(t *testing.T) {
	downloads := 0
	mediaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads++
		t.Error("preview downloaded remote media before sending")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mediaServer.Close()
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Content-Type"), "multipart/") {
			_ = r.ParseMultipartForm(1 << 20)
		} else {
			_ = r.ParseForm()
		}
		var media []struct {
			Media string `json:"media"`
		}
		if err := json.Unmarshal([]byte(r.FormValue("media")), &media); err != nil {
			t.Error(err)
		}
		if len(media) != 2 || media[0].Media != mediaServer.URL+"/image.jpg" || media[1].Media != mediaServer.URL+"/video.mp4" {
			t.Errorf("remote references=%+v", media)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":[{"message_id":1,"date":1,"chat":{"id":123,"type":"private"}}]}`))
	}))
	defer apiServer.Close()
	bot, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(apiServer.URL))
	if err != nil {
		t.Fatal(err)
	}
	err = NewSysBot().sendProfileMediaPurpose(context.Background(), context.Background(), bot, "123", []*publishsysin.MediaModel{
		{Id: 4927174, MediaType: "image", Purpose: "display", FileUrl: mediaServer.URL + "/image.jpg"},
		{Id: 4927177, MediaType: "video", Purpose: "display", FileUrl: mediaServer.URL + "/video.mp4"},
	}, "display", "preview", false)
	if err != nil || downloads != 0 {
		t.Fatalf("downloads=%d err=%v", downloads, err)
	}
}
