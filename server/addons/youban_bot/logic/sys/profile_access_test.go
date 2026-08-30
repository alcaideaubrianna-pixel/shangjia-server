package sys

import (
	"testing"

	"github.com/go-telegram/bot/models"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
)

func TestExactProfileNo(t *testing.T) {
	tests := map[string]string{
		"FNUR8829266":       "FNUR8829266",
		"编号：fnur8829266":    "FNUR8829266",
		"资料编号: FNUR8829266": "FNUR8829266",
		"搜索 FNUR8829266":    "",
		"跳舞":                "",
	}
	for input, want := range tests {
		if got := exactProfileNo(input); got != want {
			t.Fatalf("exactProfileNo(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestProfileCardPurposeByOwnership(t *testing.T) {
	account := &botProfileAccount{AccountId: 3, AccountType: "admin"}
	owned := &publishsysin.NoteModel{ProfileModel: publishsysin.ProfileModel{AccountId: 3}}
	followed := &publishsysin.NoteModel{ProfileModel: publishsysin.ProfileModel{AccountId: 401}}

	if got := profileCardPurpose(account, owned, "view"); got != "view" {
		t.Fatalf("owned profile purpose = %q, want view", got)
	}
	if got := profileCardPurpose(account, followed, "view"); got != "readonly" {
		t.Fatalf("followed profile purpose = %q, want readonly", got)
	}
	if got := profileCardPurpose(account, followed, "up"); got != "readonly" {
		t.Fatalf("followed profile write purpose = %q, want readonly", got)
	}
	if got := botProfileScopeAccountId(account); got != 3 {
		t.Fatalf("admin write account id = %d, want 3", got)
	}
}

func TestProfileCardMarkupForNoteShowsSourceByURL(t *testing.T) {
	tests := []struct {
		name    string
		note    *publishsysin.NoteModel
		purpose string
	}{
		{
			name: "publishing account owned profile",
			note: &publishsysin.NoteModel{ProfileModel: publishsysin.ProfileModel{
				ProfileNo: "A10001", IsCollected: true, CollectSourceUrl: "https://t.me/source/101",
			}},
			purpose: "view",
		},
		{
			name: "shared readonly profile",
			note: &publishsysin.NoteModel{ProfileModel: publishsysin.ProfileModel{
				ProfileNo: "A10002", IsCollected: true, CollectSourceUrl: "https://t.me/source/102",
			}},
			purpose: "readonly",
		},
		{
			name: "legacy profile without collected flag",
			note: &publishsysin.NoteModel{ProfileModel: publishsysin.ProfileModel{
				ProfileNo: "A10003", IsCollected: false, CollectSourceUrl: "https://t.me/source/103",
			}},
			purpose: "view",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			markup := profileCardMarkupForNote(test.note, test.purpose)
			if !hasProfileSourceButton(markup, test.note.CollectSourceUrl) {
				t.Fatalf("source button missing: %#v", markup)
			}
		})
	}
}

func TestProfileCardMarkupForNoteHidesEmptySourceURL(t *testing.T) {
	note := &publishsysin.NoteModel{ProfileModel: publishsysin.ProfileModel{ProfileNo: "A10004", IsCollected: true}}
	if hasProfileSourceButton(profileCardMarkupForNote(note, "view"), "") {
		t.Fatal("unexpected source button for profile without source URL")
	}
}

func hasProfileSourceButton(markup *models.InlineKeyboardMarkup, url string) bool {
	if markup == nil {
		return false
	}
	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			if button.Text == "来源频道 >" && button.URL == url {
				return true
			}
		}
	}
	return false
}
