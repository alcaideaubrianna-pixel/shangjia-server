package publish

import (
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/frame/g"
)

type VipConfigViewReq struct {
	g.Meta `path:"/publish/vip/config/view" method:"get" tags:"上架插件后台" summary:"会员配置详情"`
}

type VipConfigViewRes struct {
	*sysin.TenantVipConfigModel
}

type VipConfigSaveReq struct {
	g.Meta `path:"/publish/vip/config/save" method:"post" tags:"上架插件后台" summary:"保存会员配置"`
	sysin.TenantVipConfigSaveInp
}

type VipConfigSaveRes struct{}

type VipTenantSaveReq struct {
	g.Meta `path:"/publish/vip/tenant/save" method:"post" tags:"上架插件后台" summary:"调整账号归属会员"`
	sysin.TenantVipTenantSaveInp
}

type VipTenantSaveRes struct{}

type VipOrderListReq struct {
	g.Meta `path:"/publish/vip/order/list" method:"get" tags:"上架插件后台" summary:"会员订单列表"`
	sysin.TenantVipOrderListInp
}

type VipOrderListRes struct {
	form.PageRes
	List []*sysin.TenantVipOrderModel `json:"list" dc:"订单列表"`
}

type VipLogListReq struct {
	g.Meta `path:"/publish/vip/log/list" method:"get" tags:"上架插件后台" summary:"会员变更记录"`
	sysin.TenantVipLogListInp
}

type VipLogListRes struct {
	form.PageRes
	List []*sysin.TenantVipLogModel `json:"list" dc:"会员记录列表"`
}

type VipCouponListReq struct {
	g.Meta `path:"/publish/vip/coupon/list" method:"get" tags:"上架插件后台" summary:"优惠码列表"`
	sysin.TenantVipCouponListInp
}

type VipCouponListRes struct {
	form.PageRes
	List []*sysin.TenantVipCouponModel `json:"list" dc:"优惠码列表"`
}

type VipCouponSaveReq struct {
	g.Meta `path:"/publish/vip/coupon/save" method:"post" tags:"上架插件后台" summary:"保存优惠码"`
	sysin.TenantVipCouponSaveInp
}

type VipCouponSaveRes struct{}

type VipCouponStatusReq struct {
	g.Meta `path:"/publish/vip/coupon/status" method:"post" tags:"上架插件后台" summary:"修改优惠码状态"`
	sysin.TenantVipCouponStatusInp
}

type VipCouponStatusRes struct{}
