package cms_app

import (
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_open/model/input/sysin"
)

type ListReq struct {
	g.Meta `path:"/cmsApp/list" method:"get"`
	sysin.CmsAppListInp
}
type ListRes struct {
	List []*sysin.CmsAppModel `json:"list"`
}
type SaveReq struct {
	g.Meta `path:"/cmsApp/save" method:"post"`
	sysin.CmsAppSaveInp
}
type SaveRes struct{ *sysin.CmsAppCredentialModel }
type ResetSecretReq struct {
	g.Meta `path:"/cmsApp/resetSecret" method:"post"`
	sysin.CmsAppResetSecretInp
}
type ResetSecretRes struct{ *sysin.CmsAppCredentialModel }
