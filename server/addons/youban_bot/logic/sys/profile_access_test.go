package sys

import (
	"strings"
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
	account := &botProfileAccount{TenantId: 7, AccountId: 3, AccountType: "uploader"}
	owned := &publishsysin.NoteModel{ProfileModel: publishsysin.ProfileModel{TenantId: 7, AccountId: 3}}
	followed := &publishsysin.NoteModel{ProfileModel: publishsysin.ProfileModel{TenantId: 9, AccountId: 401}}

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

func TestProfileEditSuccessMarkupReusesPublishCallback(t *testing.T) {
	markup := profileEditSuccessMarkup("a10005")
	if markup == nil || len(markup.InlineKeyboard) == 0 || len(markup.InlineKeyboard[0]) < 2 {
		t.Fatalf("edit success buttons missing: %#v", markup)
	}
	if got := markup.InlineKeyboard[0][0]; got.Text != "重新上架" || got.CallbackData != "pf:up:A10005" {
		t.Fatalf("republish button = %#v", got)
	}
	if got := markup.InlineKeyboard[0][1]; got.CallbackData != "pf:view:A10005" {
		t.Fatalf("view button = %#v", got)
	}
}

func TestFormatProfileChannelActionNotice(t *testing.T) {
	channels := []profileActionChannel{
		{Id: 7, Title: "频道甲", Username: "public_one", TargetChatId: "-100123", LatestMessageId: 10},
		{Id: 8, Title: "频道<乙>", TargetChatId: "-100456", LatestMessageId: 20},
		{Id: 7, Title: "频道甲", Username: "public_one"},
	}
	text := formatProfileChannelActionNotice("a10006", "标题<测试>", channels, "下架")
	for _, want := range []string{"<b>资料下架完成</b>", "编号：<code>A10006</code>", "标题：<code>标题&lt;测试&gt;</code>", `[频道甲] <a href="https://t.me/public_one/10"><b>已下架</b></a>`, `[频道&lt;乙&gt;] <a href="https://t.me/c/456/20"><b>已下架</b></a>`, "频道历史消息已进入后台清理队列"} {
		if !strings.Contains(text, want) {
			t.Fatalf("notice missing %q: %s", want, text)
		}
	}
	if strings.Count(text, "[频道甲]") != 1 {
		t.Fatalf("duplicate channel result: %s", text)
	}

	deleted := formatProfileChannelActionNotice("A10006", "删除标题", []profileActionChannel{{Id: 9}}, "删除")
	if !strings.Contains(deleted, "[频道 9] <b>已删除</b>") {
		t.Fatalf("delete fallback channel missing: %s", deleted)
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

func TestProfileCardTenantAdminPermissions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		account *botProfileAccount
		note    *publishsysin.NoteModel
		want    string
	}{
		{"TDURJ8IM", &botProfileAccount{TenantId: 7, AccountId: 8, AccountType: "admin"}, &publishsysin.NoteModel{ProfileModel: publishsysin.ProfileModel{Id: 643999, TenantId: 7, AccountId: 139, ProfileNo: "FNYA5991185", Status: 1}}, "view"},
		{"cross tenant admin", &botProfileAccount{TenantId: 7, AccountId: 8, AccountType: "admin"}, &publishsysin.NoteModel{ProfileModel: publishsysin.ProfileModel{TenantId: 9, AccountId: 139}}, "readonly"},
		{"uploader other owner", &botProfileAccount{TenantId: 7, AccountId: 8, AccountType: "uploader"}, &publishsysin.NoteModel{ProfileModel: publishsysin.ProfileModel{TenantId: 7, AccountId: 139}}, "readonly"},
		{"shared uploader", &botProfileAccount{TenantId: 7, AccountId: 8, AccountType: "uploader", SharedResourceEnabled: true}, &publishsysin.NoteModel{ProfileModel: publishsysin.ProfileModel{TenantId: 7, AccountId: 139}}, "view"},
		{"missing tenant", &botProfileAccount{AccountId: 8, AccountType: "admin"}, &publishsysin.NoteModel{ProfileModel: publishsysin.ProfileModel{AccountId: 8}}, "readonly"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := profileCardPurpose(tc.account, tc.note, "view")
			if got != tc.want {
				t.Fatalf("purpose=%s want=%s", got, tc.want)
			}
			buttons := profileCardMarkupForNote(tc.note, got)
			writes := false
			for _, row := range buttons.InlineKeyboard {
				for _, b := range row {
					if b.Text == "删除" || b.Text == "上架" || b.Text == "下架" {
						writes = true
					}
				}
			}
			if writes != (tc.want == "view") {
				t.Fatalf("write buttons=%v want=%s", writes, tc.want)
			}
		})
	}
}
