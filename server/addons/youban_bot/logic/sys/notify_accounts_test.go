package sys

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func TestUniqueNotifyAccountIds(t *testing.T) {
	got := uniqueNotifyAccountIds([]int64{3, 0, 2, 3, -1, 2, 5})
	want := []int64{3, 2, 5}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("uniqueNotifyAccountIds() = %v, want %v", got, want)
	}
}

func TestNotifyAccountChatIdIntegration(t *testing.T) {
	if os.Getenv("YOUBAN_BOT_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_BOT_INTEGRATION=1 to run database integration test")
	}
	ctx := context.Background()
	seed := time.Now().UnixNano()%1_000_000_000 + 9_000_000_000
	telegramUserId := fmt.Sprintf("notify_integration_%d", seed)
	now := gtime.Now()
	_, err := g.DB().Model(userTable).Safe().Ctx(ctx).Data(g.Map{
		"bot_id":              seed,
		"telegram_user_id":    telegramUserId,
		"telegram_username":   telegramUserId,
		"telegram_first_name": "integration",
		"chat_id":             telegramUserId,
		"chat_type":           "private",
		"status":              1,
		"created_at":          now,
		"updated_at":          now,
	}).Insert()
	if err != nil {
		t.Fatalf("insert Telegram user fixture: %+v", err)
	}
	defer func() {
		_, _ = g.DB().Exec(ctx, "DELETE FROM "+userTable+" WHERE bot_id=? AND telegram_user_id=?", seed, telegramUserId)
	}()

	chatId, err := (&sSysBot{}).notifyAccountChatId(ctx, seed, telegramUserId)
	if err != nil {
		t.Fatalf("read Telegram notification chat: %+v", err)
	}
	if chatId != telegramUserId {
		t.Fatalf("chatId=%s, want %s", chatId, telegramUserId)
	}
}
