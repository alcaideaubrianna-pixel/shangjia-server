package sys

import (
	"testing"

	"github.com/gotd/td/tg"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestUniqueStrings(t *testing.T) {
	values := uniqueStrings([]string{" 1 ", "1", "", "2"})
	if len(values) != 2 || values[0] != "1" || values[1] != "2" {
		t.Fatalf("unexpected values: %+v", values)
	}
}

func TestValidateAccountCollectTgAccount(t *testing.T) {
	tests := []struct {
		name    string
		account *accountCollectTgAccount
		wantErr bool
	}{
		{name: "missing", wantErr: true},
		{name: "expired", account: &accountCollectTgAccount{Id: 2, Status: "expired", SessionKey: "session.json"}, wantErr: true},
		{name: "missing session", account: &accountCollectTgAccount{Id: 2, Status: sysin.PublishTgAccountStatusAuthorized}, wantErr: true},
		{name: "authorized", account: &accountCollectTgAccount{Id: 2, Status: sysin.PublishTgAccountStatusAuthorized, SessionKey: "session.json"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateAccountCollectTgAccount(test.account)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateAccountCollectTgAccount() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestGotdCollectMessageUsesMessageIdentityForMediaGroups(t *testing.T) {
	source := accountCollectSourceRuntime{Id: 7, TenantId: 1, AccountId: 2}
	first := &tg.Message{ID: 101, Message: "资料"}
	first.SetGroupedID(9001)
	second := &tg.Message{ID: 102}
	second.SetGroupedID(9001)

	firstMessage := gotdCollectMessage(3, source, first, "-100123")
	secondMessage := gotdCollectMessage(3, source, second, "-100123")
	if firstMessage.SourceGroupedId == "" || firstMessage.SourceGroupedId != secondMessage.SourceGroupedId {
		t.Fatalf("media group identity mismatch: first=%q second=%q", firstMessage.SourceGroupedId, secondMessage.SourceGroupedId)
	}
	if firstMessage.SourceUniqueKey == secondMessage.SourceUniqueKey {
		t.Fatalf("media group messages must have distinct event keys: %q", firstMessage.SourceUniqueKey)
	}
}

func TestAccountCollectMaterialGroupKeyMergesRawMediaEvents(t *testing.T) {
	first := &collectorin.CollectorDelivery{
		TgAccountID:     3,
		SourceID:        7,
		SourceChatID:    "-100123",
		SourceMessageID: 101,
		SourceGroupedID: "9001",
		SourceUniqueKey: "account:3:source:7:-100123:message:101",
	}
	second := *first
	second.SourceMessageID = 102
	second.SourceUniqueKey = "account:3:source:7:-100123:message:102"

	if got, want := accountCollectMaterialGroupKey(first, first.SourceGroupedID), "account:3:source:7:-100123:group:9001"; got != want {
		t.Fatalf("unexpected group key: got %q want %q", got, want)
	}
	if got := accountCollectMaterialGroupKey(&second, second.SourceGroupedID); got != accountCollectMaterialGroupKey(first, first.SourceGroupedID) {
		t.Fatalf("same media group must share material key: first=%q second=%q", accountCollectMaterialGroupKey(first, first.SourceGroupedID), got)
	}
}
