package sys

import (
	"testing"

	"hotgo/addons/youban_bot/internal/dao"
)

func TestManagerInputCandidatesMixedAndDeduplicated(t *testing.T) {
	got := managerInputCandidates("给 daw @Foo_Bot，123456:ABCDEFGHIJKLMNOPQRSTUV @foo_bot https://t.me/other_bot")
	if len(got) != 3 || !got[0].Token || got[0].Value != "123456:ABCDEFGHIJKLMNOPQRSTUV" {
		t.Fatalf("unexpected candidates: %#v", got)
	}
	if got[1].Token || got[1].Value != "foo_bot" || got[2].Value != "other_bot" {
		t.Fatalf("unexpected usernames: %#v", got)
	}
}

func TestManagerInputCandidatesBounded(t *testing.T) {
	input := ""
	for i := 0; i < 30; i++ {
		input += " @bot_" + string(rune('a'+i%26))
	}
	if got := managerInputCandidates(input); len(got) > 20 {
		t.Fatalf("candidate limit exceeded: %d", len(got))
	}
}

func TestManagerInputCandidatesTelegramLinks(t *testing.T) {
	got := managerInputCandidates("@foo_bot https://t.me/bar_bot t.me/baz_bot")
	if len(got) != 3 || got[0].Value != "foo_bot" || got[1].Value != "bar_bot" || got[2].Value != "baz_bot" {
		t.Fatalf("unexpected link candidates: %#v", got)
	}
}

func TestManagerAddExistingUsernameSkipsDuplicate(t *testing.T) {
	ctx, s, st := managerTestDB(t)
	first, exists, err := s.managerAddExisting(ctx, st, "@Example_Bot")
	if err != nil || exists || first == nil {
		t.Fatalf("first add failed: bot=%+v exists=%t err=%v", first, exists, err)
	}
	second, exists, err := s.managerAddExisting(ctx, st, "example_bot")
	if err != nil || !exists || second == nil || second.Id != first.Id {
		t.Fatalf("duplicate was not reused: first=%+v second=%+v exists=%t err=%v", first, second, exists, err)
	}
	count, err := dao.YoubanBotManaged.Ctx(ctx).Count()
	if err != nil || count != 1 {
		t.Fatalf("duplicate row created: count=%d err=%v", count, err)
	}
	third, exists, err := s.managerAddExisting(ctx, st, "@another_bot")
	if err != nil || exists || third == nil || third.TelegramBotId == first.TelegramBotId {
		t.Fatalf("second username was not independently staged: bot=%+v exists=%t err=%v", third, exists, err)
	}
}
