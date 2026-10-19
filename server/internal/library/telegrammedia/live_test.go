package telegrammedia

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Opt-in, read-only verification against Telegram. The private fixture contains
// a token and message media only; it must never be committed or printed.
// No sendMessage, storage upload or database mutation is performed.
func TestLiveTelegramMediaDownload(t *testing.T) {
	fixturePath := os.Getenv("YOUBAN_TELEGRAM_MEDIA_TEST_FIXTURE")
	if fixturePath == "" {
		t.Skip("requires private read-only Telegram fixture")
	}
	var fixture struct {
		Token    string `json:"token"`
		Messages []struct {
			ID    int                `json:"id"`
			Photo []models.PhotoSize `json:"photo"`
			Video *models.Video      `json:"video"`
		} `json:"messages"`
	}
	contents, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal("private media fixture unavailable")
	}
	if json.Unmarshal(contents, &fixture) != nil || fixture.Token == "" || len(fixture.Messages) == 0 {
		t.Fatal("invalid private media fixture")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL := os.Getenv("YOUBAN_TELEGRAM_MEDIA_TEST_PROXY"); proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			t.Fatal("invalid test proxy")
		}
		transport.Proxy = http.ProxyURL(parsed)
	}
	client := &http.Client{Timeout: 20 * time.Second, Transport: transport}
	defer client.CloseIdleConnections()
	bot, err := tgbot.New(fixture.Token, tgbot.WithSkipGetMe(), tgbot.WithHTTPClient(20*time.Second, client))
	if err != nil {
		t.Fatal("unable to create official test client")
	}
	resolver := Resolver{Token: fixture.Token, Official: func(context.Context) (*tgbot.Bot, error) { return bot, nil }, Local: func(context.Context) (*tgbot.Bot, error) {
		t.Fatal("ordinary historical media was routed locally")
		return nil, nil
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	count := 0
	probe := func(message int, in Request) {
		count++
		started := time.Now()
		resolved, err := resolver.Resolve(ctx, in)
		if err != nil {
			t.Fatalf("message=%d resolve failed: %v", message, err)
		}
		getFileElapsed := time.Since(started)
		data, err := DownloadURL(ctx, client, resolved.URL, DefaultMaxBytes)
		if err != nil {
			t.Fatalf("message=%d transfer failed: %v", message, err)
		}
		if in.MediaType == "image" {
			if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
				t.Fatalf("message=%d returned invalid image", message)
			}
		}
		t.Logf("message=%d type=%s endpoint=%s bytes=%d get_file_ms=%d total_ms=%d", message, in.MediaType, resolved.Endpoint, len(data), getFileElapsed.Milliseconds(), time.Since(started).Milliseconds())
	}
	for _, message := range fixture.Messages {
		if len(message.Photo) > 0 {
			best := message.Photo[0]
			area := 0
			for _, photo := range message.Photo {
				if photo.Width >= 512 || photo.Height >= 512 {
					if next := photo.Width * photo.Height; area == 0 || next < area {
						best = photo
						area = next
					}
				}
			}
			probe(message.ID, Request{FileID: best.FileID, MediaType: "image", Size: int64(best.FileSize)})
		}
		if video := message.Video; video != nil {
			probe(message.ID, Request{FileID: video.FileID, MediaType: "video", Size: video.FileSize})
			if video.Thumbnail != nil {
				probe(message.ID, Request{FileID: video.Thumbnail.FileID, MediaType: "image", Size: int64(video.Thumbnail.FileSize)})
			}
		}
	}
	if count == 0 {
		t.Fatal("fixture contained no downloadable media")
	}
}
