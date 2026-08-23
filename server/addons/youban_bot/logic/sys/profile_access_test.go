package sys

import (
	"testing"

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
