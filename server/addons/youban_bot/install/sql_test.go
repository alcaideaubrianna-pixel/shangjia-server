package install

import (
	"strings"
	"testing"
)

func TestInviteUsageTableSql(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		upgrade bool
	}{
		{name: "mysql install", path: "addons/youban_bot/resource/sql/install.sql"},
		{name: "mysql upgrade", path: "addons/youban_bot/resource/sql/upgrade.sql", upgrade: true},
		{name: "pgsql install", path: "addons/youban_bot/resource/sql/install.pgsql.sql"},
		{name: "pgsql upgrade", path: "addons/youban_bot/resource/sql/upgrade.pgsql.sql", upgrade: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content := readSqlFile(test.path)
			normalized := strings.ReplaceAll(content, "`", "")
			if !strings.Contains(content, "hg_youban_bot_invite_usage") {
				t.Fatalf("%s does not create invite usage table", test.path)
			}
			if !strings.Contains(content, "uk_ybbiu_used_tenant") {
				t.Fatalf("%s does not create invite usage unique index", test.path)
			}
			if test.upgrade && !strings.Contains(normalized, "source IN ('web','bot')") {
				t.Fatalf("%s does not migrate reusable invite records", test.path)
			}
		})
	}
}
