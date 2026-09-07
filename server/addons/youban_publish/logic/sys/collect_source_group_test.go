package sys

import (
	"reflect"
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
)

func TestCollectSourceGroupKeyNormalizesTelegramChatID(t *testing.T) {
	grouped := gdb.Record{
		"source_chat_id":    gvar.New("-1003982091537"),
		"source_grouped_id": gvar.New("14309746313903389"),
		"source_message_id": gvar.New(int64(3179)),
	}
	if got, want := collectSourceGroupKey(grouped), "3982091537:group:14309746313903389"; got != want {
		t.Fatalf("collectSourceGroupKey() = %q, want %q", got, want)
	}
	grouped["source_chat_id"] = gvar.New("3982091537")
	if got, want := collectSourceGroupKey(grouped), "3982091537:group:14309746313903389"; got != want {
		t.Fatalf("collectSourceGroupKey() without prefix = %q, want %q", got, want)
	}
}

func TestMergeGlobalCollectTextPolicyDoesNotCreateDispatchRule(t *testing.T) {
	bound := gdb.Record{
		"delete_lines": gvar.New([]string{"local-line"}),
		"delete_texts": gvar.New([]string{"local-text"}),
		"replace_from": gvar.New([]string{"local-from"}),
		"replace_to":   gvar.New([]string{"local-to"}),
	}
	global := gdb.Record{
		"delete_lines": gvar.New([]string{"global-line"}),
		"delete_texts": gvar.New([]string{"global-text"}),
		"replace_from": gvar.New([]string{"global-from"}),
		"replace_to":   gvar.New([]string{"global-to"}),
	}
	mergeGlobalCollectTextPolicy([]gdb.Record{bound}, []gdb.Record{global})
	if got, want := collectRuleStrings(bound, "delete_lines"), []string{"global-line", "local-line"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("delete lines = %#v, want %#v", got, want)
	}
	if got, want := collectRuleStrings(bound, "delete_texts"), []string{"global-text", "local-text"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("delete texts = %#v, want %#v", got, want)
	}
	replacements := collectRuleReplacements(bound)
	if len(replacements) != 2 || replacements[0].From != "global-from" || replacements[1].From != "local-from" {
		t.Fatalf("replacements = %#v", replacements)
	}
}
