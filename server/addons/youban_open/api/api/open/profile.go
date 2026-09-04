package open

import (
	addonsSysin "hotgo/addons/youban_open/model/input/sysin"
	"hotgo/internal/model/input/form"
	"hotgo/internal/model/input/sysin"

	"github.com/gogf/gf/v2/frame/g"
)

type ListReq struct {
	g.Meta `path:"/open/v1/profiles" method:"get" tags:"开放资料" summary:"获取开放资料列表"`
	form.PageReq
	Feed            string `json:"feed" dc:"排序方式：latest/hot/recommended" v:"in:latest,hot,recommended#feed 仅支持 latest、hot、recommended"`
	ActorId         string `json:"actorId" dc:"平台侧不可逆用户标识，用于个性化推荐"`
	ProvinceCode    string `json:"provinceCode" dc:"省份行政区划编码（6位数字）" v:"regex:^[0-9]{6}$#provinceCode 必须为6位行政区划编码"`
	ProvinceCodes   string `json:"provinceCodes" dc:"多个省份行政区划编码，逗号分隔"`
	CityCode        string `json:"cityCode" dc:"城市行政区划编码（6位数字）" v:"regex:^[0-9]{6}$#cityCode 必须为6位行政区划编码"`
	AgeMin          int    `json:"ageMin" dc:"最小年龄"`
	AgeMax          int    `json:"ageMax" dc:"最大年龄"`
	HeightMin       int    `json:"heightMin" dc:"最小身高"`
	HeightMax       int    `json:"heightMax" dc:"最大身高"`
	WeightMin       int    `json:"weightMin" dc:"最小体重"`
	WeightMax       int    `json:"weightMax" dc:"最大体重"`
	Cups            string `json:"cups" dc:"资料标签，多个值用逗号分隔"`
	HasVideo        int    `json:"hasVideo" dc:"是否有视频：1是"`
	HasVerification int    `json:"hasVerification" dc:"是否有验证视频：1是"`
	IsVirgin        int    `json:"isVirgin" dc:"是否处：1是"`
}

type ListRes struct {
	form.PageRes
	List []*sysin.ContentProfileListModel `json:"list"`
}

type ViewReq struct {
	g.Meta `path:"/open/v1/profiles/{id}" method:"get" tags:"开放资料" summary:"获取开放资料详情"`
	Id     int64 `json:"id" v:"required|min:1"`
}

type ViewRes struct {
	*sysin.ContentProfileViewModel
}

type BatchReq struct {
	g.Meta `path:"/open/v1/profiles/batch" method:"post" tags:"开放资料" summary:"批量获取开放资料"`
	Ids    []int64 `json:"ids" v:"required|length:1,100#请选择资料|单次最多获取100条资料"`
}

type BatchRes struct {
	List []*sysin.ContentProfileListModel `json:"list"`
}

type RegionsReq struct {
	g.Meta `path:"/open/v1/regions" method:"get" tags:"开放资料" summary:"获取开放地区"`
}

type RegionsRes struct {
	*sysin.ContentProfileRegionsModel
}

type BindingCodeReq struct {
	g.Meta `path:"/open/v1/binding-code" method:"post" tags:"开放绑定" summary:"登记或刷新CMS绑定码"`
	addonsSysin.CmsBindingCodeSaveInp
}

type BindingCodeRes struct {
	*addonsSysin.CmsBindingCodeModel
}

type BindingListReq struct {
	g.Meta `path:"/open/v1/bindings" method:"get" tags:"开放绑定" summary:"获取CMS租户绑定"`
	addonsSysin.CmsBindingListInp
}

type BindingListRes struct {
	List []*addonsSysin.CmsBindingModel `json:"list"`
}

type BindingStatusReq struct {
	g.Meta `path:"/open/v1/bindings/status" method:"post" tags:"开放绑定" summary:"审核CMS租户绑定"`
	addonsSysin.CmsBindingStatusInp
}

type BindingStatusRes struct{ *addonsSysin.CmsBindingModel }

type SettingsReq struct {
	g.Meta `path:"/open/v1/settings" method:"post" tags:"开放配置" summary:"同步CMS开放配置"`
	addonsSysin.CmsAppSettingsInp
}
type SettingsRes struct {
	*addonsSysin.CmsAppSettingsModel
}

type InteractionReq struct {
	g.Meta     `path:"/open/v1/profile-interactions" method:"post" tags:"开放资料" summary:"上报资料互动事件"`
	EventId    string `json:"eventId" v:"required|length:8,128#事件ID不能为空|事件ID长度应为8到128位"`
	ActorId    string `json:"actorId" v:"required|length:16,128#用户标识不能为空|用户标识长度应为16到128位"`
	ProfileId  int64  `json:"profileId" v:"required|min:1#资料ID不能为空|资料ID无效"`
	Type       string `json:"type" v:"required|in:view,favorite,unfavorite#事件类型不能为空|事件类型无效"`
	OccurredAt int64  `json:"occurredAt" dc:"客户端事件时间（毫秒）"`
}

type InteractionRes struct {
	Accepted  bool `json:"accepted"`
	Duplicate bool `json:"duplicate"`
}
