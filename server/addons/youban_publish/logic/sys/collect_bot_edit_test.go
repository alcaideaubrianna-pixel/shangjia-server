package sys

import (
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
)

func TestCollectorDeliveryIsEdit(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      bool
	}{
		{"historical album edit", `{"edited_channel_post":{"message_id":11268,"date":1791554577}}`, true},
		{"ordinary channel post", `{"channel_post":{"message_id":11272}}`, false},
		{"null edited field", `{"edited_channel_post":null,"channel_post":{"message_id":1}}`, false},
		{"private edit", `{"edited_message":{"message_id":1}}`, true},
		{"invalid", `{`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := collectorDeliveryIsEdit([]byte(tc.raw)); got != tc.want {
				t.Fatalf("isEdit=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestNormalizeCollectedBotEditTextKeepsIntroFeePolicy(t *testing.T) {
	raw := "省份: 北京\n城市:北京\n年龄:18\n身高:167\n体重:92\n罩杯:C\n职业:学生\n自我介绍加分项:活波可爱 爱画画 天然无整 脾气好 配合度高\n介绍费:6888 迪士尼C312\u2063\u200b\u200c"
	rule := gdb.Record{
		"truncate_intro_fee_enabled": gvar.New(true),
		"intro_fee_suffix":           gvar.New("土豆"),
		"delete_lines":               gvar.New([]string{}),
		"delete_texts":               gvar.New([]string{}),
		"replace_from":               gvar.New([]string{"6888"}),
		"replace_to":                 gvar.New([]string{"7888"}),
	}
	if got, want := normalizeCollectedBotEditText(raw, rule), "省份: 北京\n城市:北京\n年龄:18\n身高:167\n体重:92\n罩杯:C\n职业:学生\n自我介绍加分项:活波可爱 爱画画 天然无整 脾气好 配合度高\n介绍费 7888 土豆"; got != want {
		t.Fatalf("edited text = %q, want %q", got, want)
	}
}
