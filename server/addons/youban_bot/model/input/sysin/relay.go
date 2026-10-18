package sysin

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/model/entity"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/errors/gerror"
)

type RelayBotInp struct {
	BotId int64 `json:"botId" v:"required|min:1#请选择Bot|请选择Bot"`
}

func (in *RelayBotInp) Filter() error {
	if in == nil || in.BotId <= 0 {
		return gerror.New("请选择Bot")
	}
	return nil
}

type RelaySettingsInp struct {
	RelayBotInp
	Enabled       int      `json:"enabled"`
	BlockLinks    int      `json:"blockLinks"`
	AuditDatabase int      `json:"auditDatabase"`
	AuditGroupId  string   `json:"auditGroupId"`
	Admins        []string `json:"admins"`
}

func (in *RelaySettingsInp) Filter() error {
	if err := in.RelayBotInp.Filter(); err != nil {
		return err
	}
	for _, v := range []int{in.Enabled, in.BlockLinks, in.AuditDatabase} {
		if v != 0 && v != 1 {
			return gerror.New("开关参数不合法")
		}
	}
	in.AuditGroupId = strings.TrimSpace(in.AuditGroupId)
	if in.AuditGroupId != "" {
		id, err := strconv.ParseInt(in.AuditGroupId, 10, 64)
		if err != nil || id >= 0 {
			return gerror.New("审计转发目标必须是群聊ID")
		}
	}
	if len(in.Admins) > 100 {
		return gerror.New("最多配置100个管理员")
	}
	for i, v := range in.Admins {
		v = strings.TrimSpace(v)
		if !RelayIdentityValid(v) {
			return gerror.New("管理员必须是用户ID或@用户名")
		}
		in.Admins[i] = v
	}
	return nil
}

type RelaySettingsModel struct {
	*entity.YoubanBotRelayConfig
	Admins []*entity.YoubanBotRelayAdmin `json:"admins"`
}
type RelayListInp struct {
	form.PageReq
	BotId   int64  `json:"botId"`
	Kind    string `json:"kind"`
	Keyword string `json:"keyword"`
}

func (in *RelayListInp) Filter() error {
	if in.BotId <= 0 {
		return gerror.New("请选择Bot")
	}
	in.Page, in.PerPage, _ = form.CalPage(in.Page, in.PerPage)
	if in.PerPage > 100 {
		in.PerPage = 100
	}
	in.Keyword = strings.TrimSpace(in.Keyword)
	return nil
}

type RelayBindingInp struct {
	RelayBotInp
	Left  string `json:"left"`
	Right string `json:"right"`
}

func (in *RelayBindingInp) Filter() error {
	if err := in.RelayBotInp.Filter(); err != nil {
		return err
	}
	in.Left = strings.TrimSpace(in.Left)
	in.Right = strings.TrimSpace(in.Right)
	if !RelayIdentityValid(in.Left) || !RelayIdentityValid(in.Right) {
		return gerror.New("绑定对象必须是ID或@用户名")
	}
	return nil
}

type RelayDeleteInp struct {
	RelayBotInp
	Id int64 `json:"id" v:"required|min:1#请选择记录|请选择记录"`
}
type RelayRuleInp struct {
	RelayBotInp
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

func (in *RelayRuleInp) Filter() error {
	if err := in.RelayBotInp.Filter(); err != nil {
		return err
	}
	in.Value = strings.TrimSpace(in.Value)
	if in.Value == "" || utf8.RuneCountInString(in.Value) > 255 {
		return gerror.New("屏蔽内容需为1至255个字符")
	}
	if in.Kind != consts.RelayRuleKeyword && in.Kind != consts.RelayRuleUser && in.Kind != consts.RelayRuleGroup {
		return gerror.New("屏蔽类型不合法")
	}
	if in.Kind != consts.RelayRuleKeyword && !RelayIdentityValid(in.Value) {
		return gerror.New("屏蔽对象必须是ID或@用户名")
	}
	return nil
}
func RelayIdentityValid(value string) bool {
	if strings.HasPrefix(value, "@") {
		v := strings.TrimPrefix(value, "@")
		if len(v) < 1 || len(v) > 32 {
			return false
		}
		for _, r := range v {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
				return false
			}
		}
		return true
	}
	id, err := strconv.ParseInt(value, 10, 64)
	return err == nil && id != 0 && strconv.FormatInt(id, 10) == value
}

type RelayBotModel struct {
	Id            int64  `json:"id"`
	BotName       string `json:"botName"`
	BotUsername   string `json:"botUsername"`
	Status        int    `json:"status"`
	Enabled       int    `json:"enabled"`
	BlockLinks    int    `json:"blockLinks"`
	AuditDatabase int    `json:"auditDatabase"`
	AuditGroupId  string `json:"auditGroupId"`
}

type RelaySwitchInp struct {
	RelayBotInp
	Enabled int `json:"enabled"`
}

func (in *RelaySwitchInp) Filter() error {
	if err := in.RelayBotInp.Filter(); err != nil {
		return err
	}
	if in.Enabled != 0 && in.Enabled != 1 {
		return gerror.New("开关参数不合法")
	}
	return nil
}
