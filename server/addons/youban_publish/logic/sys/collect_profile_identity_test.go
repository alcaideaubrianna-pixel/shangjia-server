package sys

import (
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
)

func TestCollectPublishClientRequestIDIgnoresCollectorAndRule(t *testing.T) {
	accountEvent := gdb.Record{
		"tenant_id": gvar.New(5), "account_id": gvar.New(6),
		"source_chat_id": gvar.New("3974614787"), "source_grouped_id": gvar.New("14288735558729401"),
		"source_unique_key": gvar.New("account:13:source:80:3974614787:group:14288735558729401"),
	}
	botEvent := gdb.Record{
		"tenant_id": gvar.New(5), "account_id": gvar.New(6),
		"source_chat_id": gvar.New("-1003974614787"), "source_grouped_id": gvar.New("14288735558729401"),
		"source_unique_key": gvar.New("bot:54:-1003974614787:group:14288735558729401"),
	}
	gotAccount := collectPublishClientRequestId(accountEvent, gdb.Record{"id": gvar.New(48)})
	gotBot := collectPublishClientRequestId(botEvent, gdb.Record{"id": gvar.New(22)})
	if gotAccount != gotBot {
		t.Fatalf("same account channel group must share profile key: account=%q bot=%q", gotAccount, gotBot)
	}
	if gotAccount != "collect:v2:5:6:-1003974614787:group:14288735558729401" {
		t.Fatalf("unexpected canonical profile key: %q", gotAccount)
	}
}

func TestCollectPublishClientRequestIDSeparatesAccounts(t *testing.T) {
	base := gdb.Record{
		"tenant_id": gvar.New(5), "account_id": gvar.New(6),
		"source_chat_id": gvar.New("3974614787"), "source_message_id": gvar.New(1005),
	}
	other := gdb.Record{
		"tenant_id": gvar.New(5), "account_id": gvar.New(7),
		"source_chat_id": gvar.New("3974614787"), "source_message_id": gvar.New(1005),
	}
	if collectPublishClientRequestId(base, nil) == collectPublishClientRequestId(other, nil) {
		t.Fatal("different accounts must not share profile key")
	}
}

func TestCollectProfileMaterialShouldNotDowngradeVerification(t *testing.T) {
	existing := gdb.Record{
		"has_verification_video": gvar.New(1), "image_count": gvar.New(4), "video_count": gvar.New(1),
	}
	if collectProfileMaterialShouldReplace(existing, 4, 0, 0) {
		t.Fatal("profile with verification video must not be replaced by incomplete material")
	}
	if !collectProfileMaterialShouldReplace(existing, 5, 2, 1) {
		t.Fatal("more complete verified material should replace existing material")
	}
}
