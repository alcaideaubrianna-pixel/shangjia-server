package publish

import (
	"hotgo/addons/youban_open/model/input/sysin"

	"github.com/gogf/gf/v2/frame/g"
)

type CmsBindingClaimReq struct {
	g.Meta `path:"/publish/admin/cms-binding/claim" method:"post" tags:"CMS绑定" summary:"申请绑定XC-CMS"`
	sysin.CmsBindingClaimInp
}

type CmsBindingClaimRes struct{ *sysin.CmsBindingModel }

type CmsBindingMineReq struct {
	g.Meta `path:"/publish/admin/cms-binding/mine" method:"get" tags:"CMS绑定" summary:"获取当前租户CMS绑定"`
}

type CmsBindingMineRes struct {
	List []*sysin.CmsBindingModel `json:"list"`
}
type CmsBindingLookupReq struct {
	g.Meta `path:"/publish/admin/cms-binding/lookup" method:"get"`
	Code   string `json:"code" v:"required|length:8,64"`
}
type CmsBindingLookupRes struct{ *sysin.CmsBindingLookupModel }

type CmsBindingRevokeReq struct {
	g.Meta `path:"/publish/admin/cms-binding/revoke" method:"post" tags:"CMS绑定" summary:"解除XC-CMS绑定"`
	sysin.CmsBindingRevokeInp
}
type CmsBindingRevokeRes struct{ *sysin.CmsBindingModel }
