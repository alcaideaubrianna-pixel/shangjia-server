package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestMatchAccountCollectSources(t *testing.T) {
	sources := []accountCollectSourceRuntime{
		{Id: 1, SourceChatId: "12345"},
		{Id: 2, SourceChatId: "-10012345"},
		{Id: 3, SourceChatId: "-678"},
		{Id: 4, SourceChatId: "999"},
	}
	matches := matchAccountCollectSources(sources, []string{"12345", "-10012345"})
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].Id != 1 || matches[1].Id != 2 {
		t.Fatalf("unexpected matches: %+v", matches)
	}
}

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
