package sys

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"hotgo/addons/youban_tg_bot_gateway/service"
)

func mediaRoutingTestGateway(conf *service.RuntimeConfig) *sGateway {
	const token = "123456:routing-test"
	gateway := NewGateway()
	gateway.loadBindingsForClient = func(context.Context) (map[string][]service.BotBinding, error) {
		return map[string][]service.BotBinding{tokenKey(token): {{Owner: "test", Token: token}}}, nil
	}
	gateway.runtimeConfigForClient = func(context.Context) (*service.RuntimeConfig, error) { return conf, nil }
	return gateway
}

func TestMediaRoutingSeparatesSendingAndDownloading(t *testing.T) {
	t.Setenv("YOUBAN_TELEGRAM_RUNTIME_SEND_BOTS", "*")
	const token = "123456:routing-test"
	var sendCount, downloadCount atomic.Int32
	sendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendVideo") {
			t.Errorf("unexpected send method: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse sendVideo body: %v", err)
		}
		switch sendCount.Add(1) {
		case 1:
			if r.FormValue("video") != "https://cdn.example/verify.mp4" {
				t.Errorf("video URL was not retained: %q", r.FormValue("video"))
			}
		case 2:
			if r.FormValue("video") != "cached-video" {
				t.Errorf("file_id was not retained: %q", r.FormValue("video"))
			}
		case 3:
			file, header, err := r.FormFile("video")
			if err != nil {
				t.Errorf("video multipart missing: %v", err)
				break
			}
			defer file.Close()
			data, err := io.ReadAll(file)
			if err != nil || string(data) != "video-bytes" || header.Filename != "verify.mp4" {
				t.Errorf("invalid multipart video: filename=%s err=%v", header.Filename, err)
			}
		default:
			t.Error("unexpected duplicate sending request")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"result":{"message_id":42,"date":1,"chat":{"id":1,"type":"private"},"video":{"file_id":"video-id","file_unique_id":"unique","width":1,"height":1,"duration":1}}}`)
	}))
	defer sendServer.Close()
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/getFile") {
			t.Errorf("download endpoint received a sending request: %s", r.URL.Path)
		}
		downloadCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"result":{"file_id":"video-id","file_unique_id":"unique","file_size":30000000,"file_path":"/data/video.mp4"}}`)
	}))
	defer downloadServer.Close()
	gateway := mediaRoutingTestGateway(&service.RuntimeConfig{RuntimeServerURL: sendServer.URL, MediaServerURL: downloadServer.URL})
	gateway.newMediaClientForToken = func(token, proxyURL, endpoint string, handler tgbot.HandlerFunc) (*tgbot.Bot, error) {
		if endpoint == "" {
			endpoint = sendServer.URL // Stand in for the official API without external requests.
		} else if endpoint != downloadServer.URL {
			t.Fatalf("unexpected endpoint: %s", endpoint)
		}
		return newBotWithTimeout(token, proxyURL, endpoint, handler, telegramMediaSendRequestTimeout)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	sender, err := gateway.SendClient(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	downloader, err := gateway.MediaClient(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if sender == downloader {
		t.Fatal("sending and downloading share the same client")
	}
	for _, input := range []models.InputFile{
		&models.InputFileString{Data: "https://cdn.example/verify.mp4"},
		&models.InputFileString{Data: "cached-video"},
		&models.InputFileUpload{Filename: "verify.mp4", Data: strings.NewReader("video-bytes")},
	} {
		message, err := sender.SendVideo(ctx, &tgbot.SendVideoParams{ChatID: 1, Video: input})
		if err != nil || message == nil || message.Video == nil || message.ID != 42 {
			t.Fatalf("sendVideo did not return a confirmed video: message=%+v err=%v", message, err)
		}
	}
	if _, err := downloader.GetFile(ctx, &tgbot.GetFileParams{FileID: "video-id"}); err != nil {
		t.Fatal(err)
	}
	if sendCount.Load() != 3 || downloadCount.Load() != 1 {
		t.Fatalf("unexpected requests send=%d download=%d", sendCount.Load(), downloadCount.Load())
	}
	for _, sending := range []bool{false, true} {
		got, err := gateway.mediaClient(ctx, token, sending)
		want := downloader
		if sending {
			want = sender
		}
		if err != nil || got != want {
			t.Fatalf("dedicated cache not reused sending=%t err=%v", sending, err)
		}
	}
}

func TestMediaRoutingDefaultSenderUsesOfficialEndpoint(t *testing.T) {
	t.Setenv("YOUBAN_TELEGRAM_RUNTIME_SEND_BOTS", "*")
	gateway := mediaRoutingTestGateway(&service.RuntimeConfig{MediaServerURL: "http://local-download.internal"})
	var endpoints []string
	gateway.newMediaClientForToken = func(_, _, endpoint string, _ tgbot.HandlerFunc) (*tgbot.Bot, error) {
		endpoints = append(endpoints, endpoint)
		return new(tgbot.Bot), nil
	}
	if _, err := gateway.SendClient(context.Background(), "123456:routing-test"); err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.MediaClient(context.Background(), "123456:routing-test"); err != nil {
		t.Fatal(err)
	}
	if len(endpoints) != 2 || endpoints[0] != "" || endpoints[1] != "http://local-download.internal" {
		t.Fatalf("download endpoint redirected the default sender: %v", endpoints)
	}
}

func TestMediaRoutingSenderIgnoresLocalRuntimeEndpoint(t *testing.T) {
	t.Setenv("YOUBAN_TELEGRAM_RUNTIME_SEND_BOTS", "")
	gateway := mediaRoutingTestGateway(&service.RuntimeConfig{RuntimeServerURL: "http://new-runtime", MediaServerURL: "http://existing-media"})
	gateway.newMediaClientForToken = func(_, _, endpoint string, _ tgbot.HandlerFunc) (*tgbot.Bot, error) {
		if endpoint != "" {
			t.Fatalf("media sender must use official API: %s", endpoint)
		}
		return new(tgbot.Bot), nil
	}
	if _, err := gateway.SendClient(context.Background(), "123456:routing-test"); err != nil {
		t.Fatal(err)
	}
}
