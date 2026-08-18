package open

import (
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_open/model/input/sysin"
)

type InstanceRegisterReq struct {
	g.Meta `path:"/open/v1/cms-instances/register" method:"post" tags:"CMS实例" summary:"注册XC-CMS实例"`
	sysin.CmsInstanceRegisterInp
}
type InstanceRegisterRes struct {
	*sysin.CmsInstanceRegisterModel
}

type InstanceHeartbeatReq struct {
	g.Meta `path:"/open/v1/cms-instances/heartbeat" method:"post" tags:"CMS实例" summary:"XC-CMS实例心跳"`
	sysin.CmsInstanceHeartbeatInp
}
type InstanceHeartbeatRes struct {
	*sysin.CmsInstanceRegisterModel
}
