package sys

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSanitizeTelegramBotError(t *testing.T) {
	raw := `Post "https://api.telegram.org/bot1234567890:ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijk/sendMediaGroup": timeout`
	err := sanitizeTelegramBotError(errors.New(raw))
	if err == nil {
		t.Fatal("sanitizeTelegramBotError() returned nil")
	}
	if strings.Contains(err.Error(), "ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijk") {
		t.Fatalf("sanitizeTelegramBotError() leaked token: %s", err)
	}
	if !strings.Contains(err.Error(), "bot1234567890:***/sendMediaGroup") {
		t.Fatalf("sanitizeTelegramBotError() missing bot id: %s", err)
	}
}

func TestConfigureTelegramMediaUploadHTTPClient(t *testing.T) {
	transport := &http.Transport{ResponseHeaderTimeout: 20 * time.Second}
	client := &http.Client{Timeout: telegramHTTPTimeout, Transport: transport}
	configured := configureTelegramMediaUploadHTTPClient(client)
	if configured.Timeout != telegramMediaUploadHTTPTimeout {
		t.Fatalf("client timeout = %s, want %s", configured.Timeout, telegramMediaUploadHTTPTimeout)
	}
	configuredTransport, ok := configured.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport type = %T", configured.Transport)
	}
	if configuredTransport.ResponseHeaderTimeout != telegramMediaUploadResponseHeaderTimeout {
		t.Fatalf("response header timeout = %s, want %s", configuredTransport.ResponseHeaderTimeout, telegramMediaUploadResponseHeaderTimeout)
	}
	if transport.ResponseHeaderTimeout != 20*time.Second {
		t.Fatalf("original transport was mutated: %s", transport.ResponseHeaderTimeout)
	}
}

func TestTelegramBotIDFromToken(t *testing.T) {
	if got := telegramBotIDFromToken(" 1234567890:secret "); got != "1234567890" {
		t.Fatalf("telegramBotIDFromToken() = %q", got)
	}
}
