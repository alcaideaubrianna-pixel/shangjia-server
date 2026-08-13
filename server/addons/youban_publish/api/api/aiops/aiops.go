package aiops

import "github.com/gogf/gf/v2/frame/g"

type ProfileMediaReq struct {
	g.Meta     `path:"/profile/media" method:"post" tags:"AI运维" summary:"诊断或恢复资料媒体"`
	ProfileIds []int64 `json:"profileIds" v:"required|max-length:20#资料ID不能为空|单次最多处理20条"`
	Apply      bool    `json:"apply" dc:"是否执行恢复"`
}

type ProfileMediaRes struct {
	Candidates  int     `json:"candidates"`
	Recoverable int     `json:"recoverable"`
	Requeued    int     `json:"requeued"`
	ProfileIds  []int64 `json:"profileIds"`
}

type ProfileRepublishReq struct {
	g.Meta     `path:"/profile/republish" method:"post" tags:"AI运维" summary:"重新上架媒体完整的资料"`
	ProfileIds []int64 `json:"profileIds" v:"required|max-length:20#资料ID不能为空|单次最多处理20条"`
}

type ProfileRepublishRes struct {
	Message string `json:"message"`
}
