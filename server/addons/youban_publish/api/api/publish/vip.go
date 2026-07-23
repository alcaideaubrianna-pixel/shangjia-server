package publish

import (
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/frame/g"
)

type TenantVipStatusReq struct {
	g.Meta `path:"/publish/vip/status" method:"get" tags:"上架插件" summary:"租户会员状态"`
}

type TenantVipStatusRes struct {
	*sysin.TenantVipStatusModel
}

type TenantVipPlansReq struct {
	g.Meta `path:"/publish/vip/plans" method:"get" tags:"上架插件" summary:"租户会员套餐"`
}

type TenantVipPlansRes struct {
	List []*sysin.TenantVipPlanModel `json:"list" dc:"套餐列表"`
}

type TenantVipOrderCreateReq struct {
	g.Meta `path:"/publish/vip/order/create" method:"post" tags:"上架插件" summary:"创建租户会员订单"`
	sysin.TenantVipOrderCreateInp
}

type TenantVipOrderCreateRes struct {
	*sysin.TenantVipOrderModel
}

type TenantVipOrderListReq struct {
	g.Meta `path:"/publish/vip/order/list" method:"get" tags:"上架插件" summary:"租户会员订单列表"`
	sysin.TenantVipOrderListInp
}

type TenantVipOrderListRes struct {
	form.PageRes
	List []*sysin.TenantVipOrderModel `json:"list" dc:"订单列表"`
}

type TenantVipOrderPayReq struct {
	g.Meta `path:"/publish/vip/order/pay" method:"post" tags:"上架插件" summary:"支付租户会员订单"`
	sysin.TenantVipOrderPayInp
}

type TenantVipOrderPayRes struct {
	*sysin.TenantVipOrderModel
}

type TenantVipCouponCheckReq struct {
	g.Meta `path:"/publish/vip/coupon/check" method:"post" tags:"上架插件" summary:"检查租户会员优惠码"`
	sysin.TenantVipCouponCheckInp
}

type TenantVipCouponCheckRes struct {
	*sysin.TenantVipCouponCheckModel
}

type MediaSimilarCountReq struct {
	g.Meta `path:"/publish/media/similar/count" method:"post" tags:"上架插件" summary:"媒体相似数量"`
	sysin.MediaSimilarCountInp
}

type MediaSimilarCountRes struct {
	List []*sysin.MediaSimilarCountModel `json:"list" dc:"相似数量列表"`
}

type MediaSimilarListReq struct {
	g.Meta `path:"/publish/media/similar/list" method:"get" tags:"上架插件" summary:"媒体相似列表"`
	sysin.MediaSimilarListInp
}

type MediaSimilarListRes struct {
	form.PageRes
	MediaId int64              `json:"mediaId" dc:"媒体ID"`
	List    []*sysin.NoteModel `json:"list" dc:"相似资料"`
}
