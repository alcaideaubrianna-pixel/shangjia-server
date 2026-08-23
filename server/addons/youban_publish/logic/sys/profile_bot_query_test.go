package sys

import (
	"context"
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestBotProfileSearchAccountIdsOnlyCurrentAccount(t *testing.T) {
	ids, err := (&sSysPublish{}).botProfileSearchAccountIds(context.Background(), &sysin.BotProfileSearchInp{
		TenantId: 3, AccountId: 3, AccountType: sysin.PublishAccountTypeAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != 3 {
		t.Fatalf("search account ids = %v, want [3]", ids)
	}
}
