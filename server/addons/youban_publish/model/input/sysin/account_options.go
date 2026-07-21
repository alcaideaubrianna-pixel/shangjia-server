package sysin

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
)

const (
	AccountOptionsScopeAll       = "all"
	AccountOptionsScopeFollowing = "following"
)

type AccountOptionsInp struct {
	Scope string `json:"scope" dc:"筛选范围：all/following"`
}

func (in *AccountOptionsInp) Filter(ctx context.Context) error {
	in.Scope = strings.TrimSpace(in.Scope)
	if in.Scope == "" {
		in.Scope = AccountOptionsScopeAll
	}
	if in.Scope != AccountOptionsScopeAll && in.Scope != AccountOptionsScopeFollowing {
		return gerror.New("筛选范围不合法")
	}
	return nil
}

type AccountOptionModel struct {
	Label string `json:"label" dc:"显示名称"`
	Value int64  `json:"value" dc:"账号ID"`
}
