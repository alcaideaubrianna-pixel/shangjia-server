package aiops

import "github.com/gogf/gf/v2/frame/g"

type ProfileMediaReq struct {
	g.Meta     `path:"/profile/media" method:"post" tags:"AI运维" summary:"诊断或恢复资料媒体"`
	ProfileIds []int64 `json:"profileIds" v:"required#资料ID不能为空"`
	Apply      bool    `json:"apply" dc:"是否执行恢复"`
}

type ProfileMediaRes struct {
	Candidates  int     `json:"candidates"`
	Recoverable int     `json:"recoverable"`
	Requeued    int     `json:"requeued"`
	ProfileIds  []int64 `json:"profileIds"`
}

type ProfileMediaQueueReq struct {
	g.Meta     `path:"/profile/media/queue" method:"post" tags:"运维" summary:"扫描并入队恢复Bot历史媒体"`
	ProfileIds []int64 `json:"profileIds" dc:"指定资料ID；留空扫描全部"`
	Limit      int     `json:"limit" d:"1000" v:"max:1000#单次最多入队1000条资料"`
}

type ProfileMediaQueueRes struct {
	Queued     int     `json:"queued"`
	ProfileIds []int64 `json:"profileIds"`
}

type ProfileRepublishReq struct {
	g.Meta     `path:"/profile/republish" method:"post" tags:"AI运维" summary:"重新上架媒体完整的资料"`
	ProfileIds []int64 `json:"profileIds" v:"required#资料ID不能为空"`
}

type ProfileRepublishRes struct {
	Message string `json:"message"`
}

type ProfileDeleteReq struct {
	g.Meta     `path:"/profile/delete" method:"post" tags:"AI运维" summary:"批量下架并删除指定账号的可管理资料"`
	TenantId   int64   `json:"tenantId" v:"min:1#租户ID不能为空"`
	AccountId  int64   `json:"accountId" v:"min:1#账号ID不能为空"`
	ProfileIds []int64 `json:"profileIds" v:"required#资料ID不能为空"`
	Apply      bool    `json:"apply" dc:"是否执行删除；关闭时仅校验"`
}

type ProfileDeleteRes struct {
	Candidates int     `json:"candidates"`
	Deleted    int     `json:"deleted"`
	ProfileIds []int64 `json:"profileIds"`
}
