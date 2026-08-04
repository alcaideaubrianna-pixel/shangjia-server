package sysin

import (
	"hotgo/internal/model/input/form"
	"hotgo/internal/model/input/payin"

	"github.com/gogf/gf/v2/os/gtime"
)

const (
	TenantVipFeatureSimilarMedia      = "similar_media"
	TenantVipFeatureAntiScan          = "anti_scan"
	TenantVipFeatureTextObfuscation   = "text_obfuscation"
	TenantVipFeatureCollectSource     = "collect_source"
	TenantVipFeatureBackgroundReplace = "background_replace"
	TenantVipFeatureRandomMedia       = "random_media"
)

type TenantVipStatusModel struct {
	TenantId            int64                     `json:"tenantId" dc:"租户ID"`
	IsVip               bool                      `json:"isVip" dc:"是否会员"`
	Level               int                       `json:"level" dc:"会员等级"`
	Status              int                       `json:"status" dc:"会员状态"`
	ExpiredAt           *gtime.Time               `json:"expiredAt" dc:"到期时间"`
	Features            []string                  `json:"features" dc:"会员权益"`
	AvailableFeatures   []string                  `json:"availableFeatures" dc:"用户端可见功能"`
	ActivityBannerTitle string                    `json:"activityBannerTitle" dc:"活动标题"`
	ActivityBannerText  string                    `json:"activityBannerText" dc:"活动说明"`
	Activities          []*TenantVipActivityModel `json:"activities" dc:"会员活动"`
}

type TenantVipActivityModel struct {
	Code            string `json:"code" dc:"活动编码"`
	Title           string `json:"title" dc:"活动标题"`
	Description     string `json:"description" dc:"活动说明"`
	Action          string `json:"action" dc:"操作类型"`
	RewardDays      int    `json:"rewardDays" dc:"单次奖励天数"`
	RewardCount     int    `json:"rewardCount" dc:"累计奖励次数"`
	RewardDaysTotal int    `json:"rewardDaysTotal" dc:"累计奖励天数"`
	Completed       bool   `json:"completed" dc:"是否已完成"`
	StatusText      string `json:"statusText" dc:"状态说明"`
}

type TenantFeaturePermissionViewInp struct {
	TenantId int64 `json:"tenantId" dc:"租户ID"`
}

type TenantFeaturePermissionItem struct {
	Code        string `json:"code" dc:"功能编码"`
	Name        string `json:"name" dc:"功能名称"`
	Description string `json:"description" dc:"功能说明"`
	Enabled     bool   `json:"enabled" dc:"是否开放"`
}

type TenantFeaturePermissionViewModel struct {
	TenantId int64                          `json:"tenantId" dc:"租户ID"`
	List     []*TenantFeaturePermissionItem `json:"list" dc:"功能列表"`
}

type TenantFeaturePermissionSaveInp struct {
	TenantId int64    `json:"tenantId" dc:"租户ID"`
	Features []string `json:"features" dc:"开放功能编码"`
}

type TenantVipPlanModel struct {
	Code          string                   `json:"code" dc:"套餐编码"`
	CouponAmount  float64                  `json:"couponAmount" dc:"优惠券金额"`
	CouponEnabled bool                     `json:"couponEnabled" dc:"是否启用优惠券"`
	Currency      string                   `json:"currency" dc:"币种"`
	Days          int                      `json:"days" dc:"开通天数"`
	Description   string                   `json:"description" dc:"说明"`
	DiscountText  string                   `json:"discountText" dc:"折扣文案"`
	Features      []string                 `json:"features" dc:"权益列表"`
	PayItems      []*TenantVipPayItemModel `json:"payItems" dc:"支付渠道"`
	Level         int                      `json:"level" dc:"会员等级"`
	Name          string                   `json:"name" dc:"套餐名称"`
	OriginalPrice float64                  `json:"originalPrice" dc:"原价"`
	Price         float64                  `json:"price" dc:"价格"`
}

type TenantVipPayItemModel struct {
	Enabled   bool    `json:"enabled" dc:"是否启用"`
	Label     string  `json:"label" dc:"名称"`
	PayType   string  `json:"payType" dc:"支付方式"`
	TradeType string  `json:"tradeType" dc:"交易类型"`
	Money     float64 `json:"money" dc:"金额"`
}

type TenantVipOrderCreateInp struct {
	CouponCode string `json:"couponCode" dc:"优惠券码"`
	PayType    string `json:"payType" dc:"支付方式"`
	PlanCode   string `json:"planCode" dc:"套餐编码"`
	ReturnUrl  string `json:"returnUrl" dc:"买家付款成功跳转地址"`
	TradeType  string `json:"tradeType" dc:"交易类型"`
}

type TenantVipOrderModel struct {
	Id         int64                   `json:"id" dc:"订单ID"`
	TenantId   int64                   `json:"tenantId" dc:"租户ID"`
	TenantName string                  `json:"tenantName" dc:"租户名称"`
	OrderNo    string                  `json:"orderNo" dc:"订单号"`
	PlanCode   string                  `json:"planCode" dc:"套餐编码"`
	PlanName   string                  `json:"planName" dc:"套餐名称"`
	Amount     float64                 `json:"amount" dc:"金额"`
	Currency   string                  `json:"currency" dc:"币种"`
	Status     int                     `json:"status" dc:"订单状态"`
	StatusTxt  string                  `json:"statusText" dc:"订单状态文本"`
	PayUrl     string                  `json:"payUrl" dc:"支付地址"`
	TradeType  string                  `json:"tradeType" dc:"交易类型"`
	CreatedAt  *gtime.Time             `json:"createdAt" dc:"创建时间"`
	PaidAt     *gtime.Time             `json:"paidAt" dc:"支付时间"`
	Order      *payin.CreateOrderModel `json:"order" dc:"支付订单"`
}

type TenantVipOrderListInp struct {
	form.PageReq
	TenantId int64 `json:"tenantId" dc:"租户ID"`
}

type TenantVipLogListInp struct {
	form.PageReq
	TenantId int64 `json:"tenantId" dc:"租户ID"`
}

type TenantVipLogModel struct {
	Action          string      `json:"action" dc:"操作"`
	AfterExpiredAt  *gtime.Time `json:"afterExpiredAt" dc:"变更后到期时间"`
	AfterLevel      int         `json:"afterLevel" dc:"变更后等级"`
	AfterStatus     int         `json:"afterStatus" dc:"变更后状态"`
	BeforeExpiredAt *gtime.Time `json:"beforeExpiredAt" dc:"变更前到期时间"`
	BeforeLevel     int         `json:"beforeLevel" dc:"变更前等级"`
	BeforeStatus    int         `json:"beforeStatus" dc:"变更前状态"`
	CreatedAt       *gtime.Time `json:"createdAt" dc:"创建时间"`
	Id              int64       `json:"id" dc:"ID"`
	OperatorId      int64       `json:"operatorId" dc:"操作人ID"`
	Remark          string      `json:"remark" dc:"备注"`
	Source          string      `json:"source" dc:"来源"`
	TenantId        int64       `json:"tenantId" dc:"租户ID"`
	TenantName      string      `json:"tenantName" dc:"租户名称"`
}

type TenantVipOrderPayInp struct {
	Id        int64  `json:"id" dc:"订单ID"`
	ReturnUrl string `json:"returnUrl" dc:"买家付款成功跳转地址"`
}

type TenantVipCouponCheckInp struct {
	Code string `json:"code" dc:"优惠码"`
}

type TenantVipCouponCheckModel struct {
	Amount   float64 `json:"amount" dc:"抵扣金额"`
	Code     string  `json:"code" dc:"优惠码"`
	Discount float64 `json:"discount" dc:"优惠金额"`
	Message  string  `json:"message" dc:"提示信息"`
	Valid    bool    `json:"valid" dc:"是否有效"`
}

type TenantVipConfigModel struct {
	Currency       string  `json:"currency" dc:"币种"`
	DiscountText   string  `json:"discountText" dc:"折扣文案"`
	Enabled        bool    `json:"enabled" dc:"是否启用"`
	MonthlyPrice   float64 `json:"monthlyPrice" dc:"会员月价"`
	OriginalPrice  float64 `json:"originalPrice" dc:"展示原价"`
	PaymentGateway string  `json:"paymentGateway" dc:"支付网关"`
}

type TenantVipConfigSaveInp = TenantVipConfigModel

type TenantVipTenantSaveInp struct {
	ExpiredAt int64  `json:"expiredAt" dc:"到期时间毫秒时间戳"`
	Level     int    `json:"level" dc:"会员等级"`
	Remark    string `json:"remark" dc:"备注"`
	TenantId  int64  `json:"tenantId" dc:"租户ID"`
}

type TenantVipCouponListInp struct {
	form.PageReq
	Status  int    `json:"status" dc:"状态"`
	Keyword string `json:"keyword" dc:"关键词"`
}

type TenantVipCouponSaveInp struct {
	Amount     float64 `json:"amount" dc:"优惠金额"`
	Code       string  `json:"code" dc:"优惠码"`
	ExpiredAt  int64   `json:"expiredAt" dc:"到期时间毫秒时间戳"`
	Id         int64   `json:"id" dc:"ID"`
	Remark     string  `json:"remark" dc:"备注"`
	TotalCount int     `json:"totalCount" dc:"可用次数"`
	UseType    string  `json:"useType" dc:"使用类型"`
}

type TenantVipCouponStatusInp struct {
	Id     int64 `json:"id" dc:"ID"`
	Status int   `json:"status" dc:"状态"`
}

type TenantVipCouponModel struct {
	Amount     float64     `json:"amount" dc:"优惠金额"`
	Code       string      `json:"code" dc:"优惠码"`
	CreatedAt  *gtime.Time `json:"createdAt" dc:"创建时间"`
	ExpiredAt  *gtime.Time `json:"expiredAt" dc:"到期时间"`
	Id         int64       `json:"id" dc:"ID"`
	Remark     string      `json:"remark" dc:"备注"`
	Status     int         `json:"status" dc:"状态"`
	TotalCount int         `json:"totalCount" dc:"可用次数"`
	UseType    string      `json:"useType" dc:"使用类型"`
	UsedCount  int         `json:"usedCount" dc:"已用次数"`
}

type MediaSimilarCountInp struct {
	MediaId int64 `json:"mediaId" dc:"媒体ID"`
}

type MediaSimilarCountModel struct {
	MediaId int64 `json:"mediaId" dc:"媒体ID"`
	Count   int   `json:"count" dc:"相似数量"`
}

type MediaSimilarListInp struct {
	form.PageReq
	MediaId   int64 `json:"mediaId" dc:"媒体ID"`
	Threshold int   `json:"threshold" dc:"最大汉明距离"`
}

type MediaSimilarListModel struct {
	MediaId int64        `json:"mediaId" dc:"媒体ID"`
	List    []*NoteModel `json:"list" dc:"相似资料"`
}
