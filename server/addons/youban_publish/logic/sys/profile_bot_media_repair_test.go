package sys

import "testing"

func TestParseBotMediaCopyReference(t *testing.T) {
	chatID, messageID, err := parseBotMediaCopyReference("copy:8974928761:7242")
	if err != nil || chatID != "8974928761" || messageID != 7242 {
		t.Fatalf("parseBotMediaCopyReference() = %q, %d, %v", chatID, messageID, err)
	}
	if _, _, err = parseBotMediaCopyReference("invalid"); err == nil {
		t.Fatal("invalid reference must fail")
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
