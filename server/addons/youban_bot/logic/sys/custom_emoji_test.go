package sys

import (
	"bytes"
	"compress/gzip"
	"context"
	"testing"

	"github.com/go-telegram/bot/models"

	"hotgo/addons/youban_bot/model/input/sysin"
	"hotgo/internal/library/storager"
)

func TestCustomEmojiResolveInpFilter(t *testing.T) {
	in := &sysin.CustomEmojiResolveInp{EmojiIds: []string{" 5368324170671202286 ", "5368324170671202286", "123"}}
	if err := in.Filter(context.Background()); err != nil {
		t.Fatalf("Filter returned error: %v", err)
	}
	if len(in.EmojiIds) != 2 || in.EmojiIds[0] != "5368324170671202286" || in.EmojiIds[1] != "123" {
		t.Fatalf("unexpected normalized ids: %#v", in.EmojiIds)
	}
}

func TestCustomEmojiFormat(t *testing.T) {
	tests := []struct {
		name       string
		sticker    *models.Sticker
		filePath   string
		format     string
		renderType string
		uploadType string
	}{
		{name: "static", sticker: &models.Sticker{}, filePath: "stickers/a.webp", format: "webp", renderType: "image", uploadType: storager.KindImg},
		{name: "animated", sticker: &models.Sticker{IsAnimated: true}, filePath: "stickers/a.tgs", format: "tgs", renderType: "lottie", uploadType: storager.KindOther},
		{name: "video", sticker: &models.Sticker{IsVideo: true}, filePath: "stickers/a.webm", format: "webm", renderType: "video", uploadType: storager.KindVideo},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			format, renderType, uploadType := customEmojiFormat(test.sticker, test.filePath)
			if format != test.format || renderType != test.renderType || uploadType != test.uploadType {
				t.Fatalf("unexpected result: %s %s %s", format, renderType, uploadType)
			}
		})
	}
}

func TestDecompressTelegramTGS(t *testing.T) {
	want := []byte(`{"v":"5.7.4","fr":60}`)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(want); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := decompressTelegramTGS(compressed.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("unexpected content: %s", got)
	}
}
