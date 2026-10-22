package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-telegram/bot/models"
	publishService "hotgo/addons/youban_publish/service"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

func TestProfilePreviewMixedRejectedReferencesSkipIncompleteHTTPRetry(t *testing.T) {
	var previous publishService.ISysPublish
	func() { defer func() { _ = recover() }(); previous = publishService.SysPublish() }()
	cache := &previewCacheStub{ISysPublish: previous, path: filepath.Join(t.TempDir(), "preview.jpg")}
	if err := os.WriteFile(cache.path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	publishService.RegisterSysPublish(cache)
	t.Cleanup(func() { publishService.RegisterSysPublish(previous) })
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: wrong file identifier/HTTP URL specified"}`))
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		var media []struct {
			Media string `json:"media"`
		}
		if err := json.Unmarshal([]byte(r.FormValue("media")), &media); err != nil {
			t.Error(err)
		}
		if len(media) != 2 {
			t.Errorf("incomplete album: %v", media)
		}
		for _, item := range media {
			if !strings.HasPrefix(item.Media, "attach://") {
				t.Errorf("rejected reference retried: %s", item.Media)
			}
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":[{"message_id":1,"date":1,"chat":{"id":123,"type":"private"}}]}`))
	}))
	defer server.Close()
	bot, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	err = NewSysBot().sendProfileMediaPurpose(context.Background(), context.Background(), bot, "123", []*publishsysin.MediaModel{
		{Id: 1, MediaType: "image", Purpose: "display", TgFileId: "invalid-1", FileUrl: "https://example.test/1.jpg"},
		{Id: 2, MediaType: "image", Purpose: "display", TgFileId: "invalid-2"},
	}, "display", "preview", false)
	if err != nil || requests.Load() != 2 || cache.calls.Load() != 2 {
		t.Fatalf("requests=%d cache=%d err=%v", requests.Load(), cache.calls.Load(), err)
	}
}

func TestProfilePreviewExpiredFallbackDoesNotStartDownloads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := NewSysBot().sendProfileMediaGroupFallback(context.Background(), ctx, nil, "123",
		[]*publishsysin.MediaModel{{Id: 1, MediaType: "image", TgFileId: "bad"}}, []string{""}, []models.InputMedia{&models.InputMediaPhoto{Media: "bad"}}, "display")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
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
	path    string
	calls   atomic.Int32
	prepare func(context.Context, *publishsysin.BotMediaCacheFileInp) error
}

func (s *previewCacheStub) BotMediaCacheFile(ctx context.Context, in *publishsysin.BotMediaCacheFileInp) (*publishsysin.BotMediaCacheFileModel, error) {
	s.calls.Add(1)
	if s.prepare != nil {
		if err := s.prepare(ctx, in); err != nil {
			return nil, err
		}
	}
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
	if err != nil || requests != 2 || uploads != 1 || cache.calls.Load() != 1 {
		t.Fatalf("requests=%d uploads=%d cache=%d err=%v", requests, uploads, cache.calls.Load(), err)
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

func TestProfilePreviewRejectedAlbumRemainsGrouped(t *testing.T) {
	for _, fallbackFails := range []bool{false, true} {
		t.Run(fmt.Sprintf("fallbackFails=%t", fallbackFails), func(t *testing.T) {
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
			t.Cleanup(func() { publishService.RegisterSysPublish(previous) })
			requests, uploads := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if !strings.HasSuffix(r.URL.Path, "/sendMediaGroup") {
					t.Errorf("album split into individual API: %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				if requests == 1 {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: failed to send message #1 with the error message Wrong file identifier/HTTP URL specified"}`))
					return
				}
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
				}
				var media []struct {
					Media   string `json:"media"`
					Caption string `json:"caption"`
				}
				if err := json.Unmarshal([]byte(r.FormValue("media")), &media); err != nil {
					t.Error(err)
				}
				if len(media) != 4 {
					t.Errorf("album size=%d want=4", len(media))
				}
				for index, item := range media {
					if !strings.HasPrefix(item.Media, "attach://") {
						t.Errorf("fallback did not upload: %s", item.Media)
						continue
					}
					if r.MultipartForm == nil || len(r.MultipartForm.File[strings.TrimPrefix(item.Media, "attach://")]) != 1 {
						t.Error("missing album attachment")
					}
					if (index == 0 && item.Caption != "preview") || (index > 0 && item.Caption != "") {
						t.Errorf("caption order incorrect at %d", index)
					}
				}
				if r.MultipartForm != nil {
					uploads += len(r.MultipartForm.File)
				}
				if fallbackFails {
					w.WriteHeader(http.StatusGatewayTimeout)
					_, _ = w.Write([]byte(`{"ok":false,"error_code":504,"description":"Gateway timeout"}`))
					return
				}
				_, _ = w.Write([]byte(`{"ok":true,"result":[{"message_id":1,"date":1,"chat":{"id":123,"type":"private"}}]}`))
			}))
			defer server.Close()
			bot, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
			if err != nil {
				t.Fatal(err)
			}
			items := make([]*publishsysin.MediaModel, 0, 4)
			for id := int64(4146259); id <= 4146262; id++ {
				items = append(items, &publishsysin.MediaModel{Id: id, MediaType: "image", Purpose: "display", TgFileId: "stale-reference"})
			}
			err = NewSysBot().sendProfileMediaPurpose(context.Background(), context.Background(), bot, "123", items, "display", "preview", false)
			if (err != nil) != fallbackFails || requests != 2 || uploads != 4 || cache.calls.Load() != 4 {
				t.Fatalf("requests=%d uploads=%d cache=%d err=%v", requests, uploads, cache.calls.Load(), err)
			}
		})
	}
}

func TestProfilePreviewAlbumFallbackOrder(t *testing.T) {
	for _, mode := range []string{"http_success", "http_rejected", "http_timeout"} {
		t.Run(mode, func(t *testing.T) {
			var previous publishService.ISysPublish
			func() {
				defer func() { _ = recover() }()
				previous = publishService.SysPublish()
			}()
			cache := &previewCacheStub{ISysPublish: previous, path: filepath.Join(t.TempDir(), "media.jpg")}
			if err := os.WriteFile(cache.path, []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			publishService.RegisterSysPublish(cache)
			t.Cleanup(func() { publishService.RegisterSysPublish(previous) })
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if !strings.HasSuffix(r.URL.Path, "/sendMediaGroup") {
					t.Errorf("album split: %s", r.URL.Path)
				}
				if strings.Contains(r.Header.Get("Content-Type"), "multipart/") {
					if err := r.ParseMultipartForm(1 << 20); err != nil {
						t.Error(err)
					}
				} else if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				var media []struct {
					Media   string `json:"media"`
					Type    string `json:"type"`
					Caption string `json:"caption"`
				}
				if err := json.Unmarshal([]byte(r.FormValue("media")), &media); err != nil {
					t.Error(err)
				}
				if len(media) != 4 {
					t.Errorf("group size=%d", len(media))
					w.WriteHeader(500)
					return
				}
				for i, item := range media {
					want := fmt.Sprintf("file-%d", i)
					if requests == 2 {
						want = fmt.Sprintf("https://media.example.com/%d.jpg", i)
					}
					if requests <= 2 && item.Media != want {
						t.Errorf("stage=%d index=%d source=%s", requests, i, item.Media)
					}
					if requests == 3 && !strings.HasPrefix(item.Media, "attach://") {
						t.Error("upload stage missing attachment")
					}
					wantType := "photo"
					if i == 1 {
						wantType = "video"
					}
					if item.Type != wantType {
						t.Errorf("media type/order changed at %d", i)
					}
					if (i == 0 && item.Caption != "original caption") || (i > 0 && item.Caption != "") {
						t.Error("caption changed")
					}
				}
				if requests < 3 && cache.calls.Load() != 0 {
					t.Error("downloaded before URL rejection")
				}
				if requests == 3 && (r.MultipartForm == nil || len(r.MultipartForm.File) != 4) {
					t.Error("missing uploaded group files")
				}
				w.Header().Set("Content-Type", "application/json")
				if requests == 1 || (requests == 2 && mode == "http_rejected") {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: Wrong file identifier/HTTP URL specified"}`))
					return
				}
				if requests == 2 && mode == "http_timeout" {
					w.WriteHeader(http.StatusGatewayTimeout)
					_, _ = w.Write([]byte(`{"ok":false,"error_code":504,"description":"Gateway timeout"}`))
					return
				}
				_, _ = w.Write([]byte(`{"ok":true,"result":[{"message_id":1,"date":1,"chat":{"id":123,"type":"private"}}]}`))
			}))
			defer server.Close()
			bot, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
			if err != nil {
				t.Fatal(err)
			}
			items := make([]*publishsysin.MediaModel, 0, 4)
			for i := 0; i < 4; i++ {
				kind := "image"
				if i == 1 {
					kind = "video"
				}
				items = append(items, &publishsysin.MediaModel{Id: int64(4146259 + i), MediaType: kind, Purpose: "display", TgFileId: fmt.Sprintf("file-%d", i), FileUrl: fmt.Sprintf("https://media.example.com/%d.jpg", i)})
			}
			err = NewSysBot().sendProfileMediaPurpose(context.Background(), context.Background(), bot, "123", items, "display", "original caption", false)
			wantRequests, wantDownloads := 2, 0
			if mode == "http_rejected" {
				wantRequests, wantDownloads = 3, 4
			}
			if (err != nil) != (mode == "http_timeout") || requests != wantRequests || int(cache.calls.Load()) != wantDownloads {
				t.Fatalf("requests=%d downloads=%d err=%v", requests, cache.calls.Load(), err)
			}
			if items[0].TgFileId != "file-0" {
				t.Error("stored media mutated")
			}
		})
	}
}

// Reproduce the production fallback: all six URLs are rejected and the
// downloads must overlap rather than consume the request budget serially.
func TestProfilePreviewRejectedAlbumPreparesConcurrently(t *testing.T) {
	var previous publishService.ISysPublish
	func() { defer func() { _ = recover() }(); previous = publishService.SysPublish() }()
	cache := &previewCacheStub{ISysPublish: previous, path: filepath.Join(t.TempDir(), "image.jpg")}
	if err := os.WriteFile(cache.path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	var active, peak atomic.Int32
	ready := make(chan struct{})
	cache.prepare = func(ctx context.Context, _ *publishsysin.BotMediaCacheFileInp) error {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		if n == 3 {
			select {
			case <-ready:
			default:
				close(ready)
			}
		}
		select {
		case <-ready:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	publishService.RegisterSysPublish(cache)
	t.Cleanup(func() { publishService.RegisterSysPublish(previous) })
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: Wrong file identifier/HTTP URL specified"}`))
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		var media []struct {
			Media string `json:"media"`
			Type  string `json:"type"`
		}
		if err := json.Unmarshal([]byte(r.FormValue("media")), &media); err != nil {
			t.Error(err)
		}
		if len(media) != 6 {
			t.Errorf("incomplete album: %d", len(media))
		}
		for i, item := range media {
			if !strings.HasPrefix(item.Media, fmt.Sprintf("attach://preview_%d_", i+1)) {
				t.Errorf("order changed: %v", media)
			}
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":[{"message_id":1,"date":1,"chat":{"id":123,"type":"private"}}]}`))
	}))
	defer server.Close()
	bot, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	items := make([]*publishsysin.MediaModel, 6)
	for i := range items {
		items[i] = &publishsysin.MediaModel{Id: int64(i + 1), MediaType: "image", Purpose: "display", FileUrl: fmt.Sprintf("https://example.test/%d.jpg", i)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = NewSysBot().sendProfileMediaPurpose(ctx, ctx, bot, "123", items, "display", "preview", false)
	if err != nil || requests != 2 || peak.Load() != 3 || cache.calls.Load() != 6 {
		t.Fatalf("requests=%d peak=%d calls=%d err=%v", requests, peak.Load(), cache.calls.Load(), err)
	}
}

func TestProfilePreviewFallbackDownloadFailureDoesNotSendPartialAlbum(t *testing.T) {
	var previous publishService.ISysPublish
	func() { defer func() { _ = recover() }(); previous = publishService.SysPublish() }()
	wantErr := errors.New("source unavailable")
	cache := &previewCacheStub{ISysPublish: previous, path: filepath.Join(t.TempDir(), "image.jpg")}
	if err := os.WriteFile(cache.path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	cache.prepare = func(ctx context.Context, in *publishsysin.BotMediaCacheFileInp) error {
		if in.Media.Id == 2 {
			return wantErr
		}
		return nil
	}
	publishService.RegisterSysPublish(cache)
	t.Cleanup(func() { publishService.RegisterSysPublish(previous) })
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests > 1 {
			t.Error("sent incomplete fallback album")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: Wrong file identifier/HTTP URL specified"}`))
	}))
	defer server.Close()
	bot, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	err = NewSysBot().sendProfileMediaPurpose(context.Background(), context.Background(), bot, "123", []*publishsysin.MediaModel{
		{Id: 1, MediaType: "image", Purpose: "display", TgFileId: "stale-1"},
		{Id: 2, MediaType: "image", Purpose: "display", TgFileId: "stale-2"},
		{Id: 3, MediaType: "video", Purpose: "display", TgFileId: "stale-3"},
	}, "display", "caption", false)
	if !errors.Is(err, wantErr) || requests != 1 {
		t.Fatalf("requests=%d err=%v", requests, err)
	}
}

func TestProfilePreviewSingleReferenceFallsBackToHTTP(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if strings.Contains(r.Header.Get("Content-Type"), "multipart/") {
			_ = r.ParseMultipartForm(1 << 20)
		} else {
			_ = r.ParseForm()
		}
		want := "stale-file"
		if requests == 2 {
			want = "https://media.example.com/photo.jpg"
		}
		if r.FormValue("photo") != want {
			t.Errorf("photo=%s want=%s", r.FormValue("photo"), want)
		}
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: Wrong file identifier/HTTP URL specified"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":123,"type":"private"}}}`))
	}))
	defer server.Close()
	bot, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	err = NewSysBot().sendProfileMediaPurpose(context.Background(), context.Background(), bot, "123", []*publishsysin.MediaModel{{Id: 1, MediaType: "image", Purpose: "display", TgFileId: "stale-file", FileUrl: "https://media.example.com/photo.jpg"}}, "display", "caption", false)
	if err != nil || requests != 2 {
		t.Fatalf("requests=%d err=%v", requests, err)
	}
}

func TestProfilePreviewPreparationFailureDoesNotSendPartialAlbum(t *testing.T) {
	var previous publishService.ISysPublish
	func() { defer func() { _ = recover() }(); previous = publishService.SysPublish() }()
	cache := &previewCacheStub{ISysPublish: previous, path: filepath.Join(t.TempDir(), "missing.jpg")}
	publishService.RegisterSysPublish(cache)
	t.Cleanup(func() { publishService.RegisterSysPublish(previous) })
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; t.Error("sent incomplete album") }))
	defer server.Close()
	bot, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	err = NewSysBot().sendProfileMediaPurpose(context.Background(), context.Background(), bot, "123", []*publishsysin.MediaModel{
		{Id: 1, MediaType: "image", Purpose: "display", TgFileId: "good-file"},
		{Id: 2, MediaType: "image", Purpose: "display"},
	}, "display", "caption", false)
	if err == nil || requests != 0 {
		t.Fatalf("requests=%d err=%v", requests, err)
	}
}
