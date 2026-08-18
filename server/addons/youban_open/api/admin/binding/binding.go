package binding

import (
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_open/model/input/sysin"
)

type ListReq struct {
	g.Meta `path:"/binding/list" method:"get"`
	sysin.CmsBindingListInp
}
type ListRes struct {
	List []*sysin.CmsBindingModel `json:"list"`
}
type StatusReq struct {
	g.Meta `path:"/binding/status" method:"post"`
	sysin.CmsBindingStatusInp
	AppId string `json:"appId"`
}
type StatusRes struct{ *sysin.CmsBindingModel }
