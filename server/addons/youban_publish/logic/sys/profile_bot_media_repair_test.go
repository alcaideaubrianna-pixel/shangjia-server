package sys

import (
	"errors"
	"testing"
)

func TestParseBotMediaCopyReference(t *testing.T) {
	chatID, messageID, err := parseBotMediaCopyReference("copy:8974928761:7242")
	if err != nil || chatID != "8974928761" || messageID != 7242 {
		t.Fatalf("parseBotMediaCopyReference() = %q, %d, %v", chatID, messageID, err)
	}
	if _, _, err = parseBotMediaCopyReference("invalid"); err == nil {
		t.Fatal("invalid reference must fail")
	}
}

func TestIsBotMediaPermanentlyUnavailableError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{errors.New("下载远程媒体失败：HTTP 404"), true},
		{errors.New("Bad Request: wrong file_id or the file is temporarily unavailable"), true},
		{errors.New("context deadline exceeded"), false},
		{nil, false},
	}
	for _, test := range tests {
		if got := isBotMediaPermanentlyUnavailableError(test.err); got != test.want {
			t.Fatalf("isBotMediaPermanentlyUnavailableError(%v) = %t, want %t", test.err, got, test.want)
		}
	}
}

func TestBotMessageMediaFileID(t *testing.T) {
	photoRaw := `{"message_id":7242,"photo":[{"file_id":"small"},{"file_id":"large"}]}`
	fileID, name, err := botMessageMediaFileID(photoRaw, "image")
	if err != nil || fileID != "large" || name != "photo_7242.jpg" {
		t.Fatalf("photo = %q, %q, %v", fileID, name, err)
	}
	videoRaw := `{"message_id":7250,"video":{"file_id":"video-file","file_name":"verify.mp4"}}`
	fileID, name, err = botMessageMediaFileID(videoRaw, "video")
	if err != nil || fileID != "video-file" || name != "verify.mp4" {
		t.Fatalf("video = %q, %q, %v", fileID, name, err)
	}
}

func TestBotTokenFromFileURL(t *testing.T) {
	token, err := botTokenFromFileURL("https://api.telegram.org/file/bot123456:secret/photos/file_1.jpg")
	if err != nil || token != "123456:secret" {
		t.Fatalf("botTokenFromFileURL() = %q, %v", token, err)
	}
	for _, value := range []string{"", "https://example.com/file.jpg", "https://api.telegram.org/file/botinvalid/file.jpg"} {
		if _, err = botTokenFromFileURL(value); err == nil {
			t.Fatalf("botTokenFromFileURL(%q) must fail", value)
		}
	}
}
