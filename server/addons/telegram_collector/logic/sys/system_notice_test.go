package sys

import (
	"encoding/json"
	"testing"

	"github.com/go-telegram/bot/models"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestCollectorSystemNoticeFromSDK(t *testing.T) {
	u := models.Update{ID: 1, ChannelPost: &models.Message{ID: 10, Chat: models.Chat{ID: -1004411799859}, Text: "✅提交成功"}}
	raw, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	if !collectorSystemNotice(sysin.SourceTypeBot, raw) {
		t.Fatalf("SDK receipt not recognized: %s", raw)
	}
	if collectorSystemNotice(sysin.SourceTypeAccount, raw) {
		t.Fatal("account flow changed")
	}
	u.ChannelPost.Text = ""
	u.ChannelPost.Caption = "验证视频"
	u.ChannelPost.Video = &models.Video{FileID: "local-only-video"}
	raw, err = json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	if collectorSystemNotice(sysin.SourceTypeBot, raw) {
		t.Fatal("verification media was ignored")
	}
}
