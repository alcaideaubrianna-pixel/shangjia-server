package sys

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"hotgo/addons/youban_bot/model/input/sysin"

	"github.com/gogf/gf/v2/errors/gerror"
)

type relayCommand struct {
	Name string
	Args string
}

func parseRelayCommand(text string) relayCommand {
	text = strings.TrimSpace(text)
	for _, name := range []string{"添加管理员", "管理员列表", "确认解绑", "解除屏蔽用户", "解除屏蔽群", "解除屏蔽@", "删除屏蔽", "屏蔽跳转", "屏蔽用户", "屏蔽群", "屏蔽@", "绑定列表", "屏蔽列表", "当前设置", "黑名单", "解绑", "绑定", "屏蔽"} {
		if !strings.HasPrefix(text, name) {
			continue
		}
		tail := strings.TrimPrefix(text, name)
		if tail != "" && !strings.HasSuffix(name, "@") && !strings.HasPrefix(tail, "@") {
			first, _ := utf8.DecodeRuneInString(tail)
			if !unicode.IsSpace(first) {
				continue
			}
		}
		args := strings.TrimSpace(tail)
		if strings.HasSuffix(name, "@") {
			name = strings.TrimSuffix(name, "@")
			args = "@" + args
		}
		return relayCommand{Name: name, Args: args}
	}
	command, args := botCommandAndArgs(text)
	if command == "help" || command == "id" {
		return relayCommand{Name: "/" + command, Args: args}
	}
	return relayCommand{}
}
func relayBindArguments(text string) (string, string, error) {
	fields := strings.Fields(text)
	if len(fields) == 1 && strings.HasPrefix(text, "@") {
		fields = strings.Split(text, "-")
	}
	if len(fields) != 2 || !sysin.RelayIdentityValid(fields[0]) || !sysin.RelayIdentityValid(fields[1]) {
		return "", "", gerror.New("格式：绑定 @alice @bob；群ID请用空格分隔，例如：绑定 -1001234567890 @alice")
	}
	return fields[0], fields[1], nil
}
