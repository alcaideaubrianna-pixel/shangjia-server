package sysin

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/internal/model/input/form"
)

type CollectManageChannelListInp struct {
	form.PageReq
	Keyword string `json:"keyword"`
}

type CollectManageChannelActionInp struct {
	RelationId int64  `json:"relationId"`
	RuleId     int64  `json:"ruleId"`
	Action     string `json:"action"`
	Suffix     string `json:"suffix"`
}

func (in *CollectManageChannelActionInp) Filter(_ context.Context) error {
	if in == nil || in.RelationId <= 0 {
		return gerror.New("请选择采集频道")
	}
	switch in.Action {
	case "approve", "reject", "disable":
		return nil
	case "suffix":
		if in.RuleId <= 0 {
			return gerror.New("请选择采集规则")
		}
		in.Suffix = strings.TrimSpace(in.Suffix)
		if utf8.RuneCountInString(in.Suffix) > 255 {
			return gerror.New("频道后缀不能超过255个字符")
		}
		return nil
	default:
		return gerror.New("不支持的频道操作")
	}
}

type CollectManageChannelModel struct {
	RelationId     int64  `json:"relationId"`
	RuleId         int64  `json:"ruleId"`
	RuleName       string `json:"ruleName"`
	ChannelId      string `json:"channelId"`
	ChannelTitle   string `json:"channelTitle"`
	ChannelLink    string `json:"channelLink"`
	Suffix         string `json:"suffix"`
	Status         string `json:"status"`
	ApprovalStatus string `json:"approvalStatus"`
}
