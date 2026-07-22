package sys

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/cache"
	"hotgo/internal/model"
	baseentity "hotgo/internal/model/entity"
	basesysin "hotgo/internal/model/input/sysin"
	"hotgo/internal/service"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const tenantVipCouponTable = "hg_youban_publish_tenant_vip_coupon"

func (s *sSysPublish) AdminTenantVipConfigView(ctx context.Context) (*sysin.TenantVipConfigModel, error) {
	cfg, err := service.SysConfig().GetYoubanPublishVip(ctx)
	if err != nil {
		return nil, err
	}
	return tenantVipConfigModel(cfg), nil
}

func (s *sSysPublish) AdminTenantVipConfigSave(ctx context.Context, in *sysin.TenantVipConfigSaveInp) error {
	list := g.Map{
		"youbanPublishVipActivityText":     in.ActivityText,
		"youbanPublishVipActivityTitle":    in.ActivityTitle,
		"youbanPublishVipDiscountText":     in.DiscountText,
		"youbanPublishVipEnabled":          in.Enabled,
		"youbanPublishVipInviteRewardDays": in.InviteRewardDays,
		"youbanPublishVipMonthlyPrice":     in.MonthlyPrice,
		"youbanPublishVipOriginalPrice":    in.OriginalPrice,
	}
	return service.SysConfig().UpdateConfigByGroup(ctx, &basesysin.UpdateConfigInp{Group: "youban_publish_vip", List: list})
}

func (s *sSysPublish) AdminTenantVipTenantSave(ctx context.Context, in *sysin.TenantVipTenantSaveInp) error {
	if in.TenantId <= 0 {
		return gerror.New("账号归属不能为空")
	}
	if in.Level < 0 || in.Level > 2 {
		return gerror.New("会员计划不合法")
	}
	var expiredAt *gtime.Time
	if in.Level > 0 {
		expiredAt = gtime.NewFromTimeStamp(in.ExpiredAt / 1000)
		if expiredAt == nil || expiredAt.Before(gtime.Now()) {
			return gerror.New("会员到期时间必须大于当前时间")
		}
	}
	return s.saveTenantVipUntil(ctx, in.TenantId, in.Level, expiredAt, strings.TrimSpace(in.Remark), "admin_adjust")
}

func (s *sSysPublish) AdminTenantVipOrderList(ctx context.Context, in *sysin.TenantVipOrderListInp) ([]*sysin.TenantVipOrderModel, int, error) {
	if in.Page <= 0 {
		in.Page = 1
	}
	if in.PerPage <= 0 || in.PerPage > 100 {
		in.PerPage = 20
	}
	cols := dao.AdminOrder.Columns()
	mod := dao.AdminOrder.Ctx(ctx).Where(cols.OrderType, tenantVipOrderType)
	if in.TenantId > 0 {
		mod = mod.Where(cols.ProductId, in.TenantId)
	}
	total, err := mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "读取会员订单数量失败")
	}
	var orders []*baseentity.AdminOrder
	if err = mod.Page(in.Page, in.PerPage).OrderDesc(cols.Id).Scan(&orders); err != nil {
		return nil, 0, gerror.Wrap(err, "读取会员订单列表失败")
	}
	return s.adminTenantVipOrderModels(ctx, orders), total, nil
}

func (s *sSysPublish) AdminTenantVipCouponList(ctx context.Context, in *sysin.TenantVipCouponListInp) ([]*sysin.TenantVipCouponModel, int, error) {
	if err := ensureTenantVipTables(ctx); err != nil {
		return nil, 0, err
	}
	if in.Page <= 0 {
		in.Page = 1
	}
	if in.PerPage <= 0 || in.PerPage > 100 {
		in.PerPage = 20
	}
	mod := g.DB().Model(tenantVipCouponTable).Safe().Ctx(ctx).WhereNull("deleted_at")
	if in.Status > 0 {
		mod = mod.Where("status", in.Status)
	}
	if strings.TrimSpace(in.Keyword) != "" {
		mod = mod.WhereLike("code", "%"+strings.TrimSpace(in.Keyword)+"%")
	}
	total, err := mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "读取优惠码数量失败")
	}
	var list []*sysin.TenantVipCouponModel
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "读取优惠码列表失败")
	}
	return list, total, nil
}

func (s *sSysPublish) AdminTenantVipCouponSave(ctx context.Context, in *sysin.TenantVipCouponSaveInp) error {
	if err := ensureTenantVipTables(ctx); err != nil {
		return err
	}
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if code == "" {
		code = randomTenantVipCouponCode()
	}
	if in.UseType != "multi" {
		in.UseType = "single"
		in.TotalCount = 1
	}
	if in.TotalCount <= 0 {
		in.TotalCount = 1
	}
	if in.Amount <= 0 {
		return gerror.New("优惠金额必须大于0")
	}
	data := g.Map{"amount": in.Amount, "code": code, "expired_at": timeFromMillis(in.ExpiredAt), "remark": strings.TrimSpace(in.Remark), "total_count": in.TotalCount, "use_type": in.UseType, "updated_at": gtime.Now()}
	if in.Id > 0 {
		_, err := g.DB().Model(tenantVipCouponTable).Safe().Ctx(ctx).Where("id", in.Id).Data(data).Update()
		return gerror.Wrap(err, "更新优惠码失败")
	}
	data["status"] = consts.StatusEnabled
	data["used_count"] = 0
	data["created_at"] = gtime.Now()
	_, err := g.DB().Model(tenantVipCouponTable).Safe().Ctx(ctx).Data(data).Insert()
	return gerror.Wrap(err, "保存优惠码失败")
}

func (s *sSysPublish) AdminTenantVipCouponStatus(ctx context.Context, in *sysin.TenantVipCouponStatusInp) error {
	if in.Id <= 0 {
		return gerror.New("优惠码ID不能为空")
	}
	_, err := g.DB().Model(tenantVipCouponTable).Safe().Ctx(ctx).Where("id", in.Id).Data(g.Map{"status": in.Status, "updated_at": gtime.Now()}).Update()
	return gerror.Wrap(err, "修改优惠码状态失败")
}

func (s *sSysPublish) tenantVipOrderAmountWithCoupon(ctx context.Context, price float64, couponCode string, cfg *model.YoubanPublishVipConfig) (amount float64, code string, discount float64, err error) {
	amount = price
	couponCode = strings.ToUpper(strings.TrimSpace(couponCode))
	if couponCode != "" {
		coupon, couponErr := s.getTenantVipCouponByCode(ctx, couponCode)
		if couponErr != nil {
			return 0, "", 0, couponErr
		}
		if coupon != nil {
			discount = coupon.Amount
			amount -= discount
			code = coupon.Code
		}
	}
	if code == "" && cfg != nil && cfg.CouponEnabled && cfg.CouponAmount > 0 && strings.TrimSpace(cfg.CouponCode) != "" && strings.EqualFold(strings.TrimSpace(cfg.CouponCode), couponCode) {
		code = strings.TrimSpace(cfg.CouponCode)
		discount = cfg.CouponAmount
		amount -= discount
	}
	if amount < 0 {
		amount = 0
	}
	return
}

func (s *sSysPublish) getTenantVipCouponByCode(ctx context.Context, code string) (*sysin.TenantVipCouponModel, error) {
	if err := ensureTenantVipTables(ctx); err != nil {
		return nil, err
	}
	var coupon sysin.TenantVipCouponModel
	if err := g.DB().Model(tenantVipCouponTable).Safe().Ctx(ctx).Where("code", strings.ToUpper(strings.TrimSpace(code))).WhereNull("deleted_at").Scan(&coupon); err != nil {
		return nil, gerror.Wrap(err, "读取优惠码失败")
	}
	if coupon.Id <= 0 {
		return nil, nil
	}
	if coupon.Status != consts.StatusEnabled {
		return nil, gerror.New("优惠码已停用")
	}
	if coupon.ExpiredAt != nil && coupon.ExpiredAt.Before(gtime.Now()) {
		return nil, gerror.New("优惠码已过期")
	}
	if coupon.UseType == "single" && coupon.UsedCount > 0 {
		return nil, gerror.New("优惠码已使用")
	}
	if coupon.UseType == "multi" && coupon.UsedCount >= coupon.TotalCount {
		return nil, gerror.New("优惠码次数已用完")
	}
	return &coupon, nil
}

func (s *sSysPublish) markTenantVipCouponUsed(ctx context.Context, detail *gjson.Json) error {
	if detail == nil {
		return nil
	}
	code := strings.TrimSpace(detail.Get("couponCode").String())
	if code == "" {
		return nil
	}
	_, err := g.DB().Model(tenantVipCouponTable).Safe().Ctx(ctx).
		Where("code", strings.ToUpper(code)).
		WhereNull("deleted_at").
		Data(g.Map{"used_count": gdb.Raw("used_count+1"), "updated_at": gtime.Now()}).
		Update()
	return gerror.Wrap(err, "更新优惠码使用次数失败")
}

func (s *sSysPublish) saveTenantVipUntil(ctx context.Context, tenantId int64, level int, expiredAt *gtime.Time, remark string, source string) error {
	before, err := s.tenantVipStatus(ctx, tenantId)
	if err != nil {
		return err
	}
	now := gtime.Now()
	status := consts.StatusDisable
	if level > 0 && expiredAt != nil && expiredAt.After(now) {
		status = consts.StatusEnabled
	}
	data := g.Map{"expired_at": expiredAt, "level": level, "opened_at": now, "remark": remark, "status": status, "updated_at": now}
	cols := pdao.YoubanPublishTenantVip.Columns()
	count, err := pdao.YoubanPublishTenantVip.Ctx(ctx).Where(cols.TenantId, tenantId).Count()
	if err != nil {
		return gerror.Wrap(err, "检查租户会员失败")
	}
	if count > 0 {
		_, err = pdao.YoubanPublishTenantVip.Ctx(ctx).Where(cols.TenantId, tenantId).Data(data).Update()
	} else {
		data[cols.TenantId] = tenantId
		data[cols.CreatedAt] = now
		_, err = pdao.YoubanPublishTenantVip.Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存租户会员失败")
	}
	_, _ = cache.Instance().Remove(ctx, tenantVipCacheKey(tenantId))
	return s.writeTenantVipLog(ctx, before, tenantId, level, expiredAt, source, remark)
}

func (s *sSysPublish) adminTenantVipOrderModels(ctx context.Context, orders []*baseentity.AdminOrder) []*sysin.TenantVipOrderModel {
	tenantNames := tenantNameMap(ctx, orders)
	res := make([]*sysin.TenantVipOrderModel, 0, len(orders))
	for _, item := range orders {
		model := tenantVipOrderModel(item, nil, &sysin.TenantVipPlanModel{Code: tenantVipPlanMonth, Currency: "U", Name: "VIP会员"})
		model.TenantId = item.ProductId
		model.TenantName = tenantNames[item.ProductId]
		res = append(res, model)
	}
	return res
}

func tenantVipConfigModel(cfg *model.YoubanPublishVipConfig) *sysin.TenantVipConfigModel {
	if cfg == nil {
		cfg = tenantVipDefaultConfig()
	}
	return &sysin.TenantVipConfigModel{ActivityText: cfg.ActivityText, ActivityTitle: cfg.ActivityTitle, DiscountText: cfg.DiscountText, Enabled: cfg.Enabled, InviteRewardDays: cfg.InviteRewardDays, MonthlyPrice: cfg.MonthlyPrice, OriginalPrice: cfg.OriginalPrice}
}

func tenantNameMap(ctx context.Context, orders []*baseentity.AdminOrder) map[int64]string {
	ids := make([]int64, 0, len(orders))
	for _, item := range orders {
		ids = append(ids, item.ProductId)
	}
	rows, _ := pdao.YoubanPublishTenant.Ctx(ctx).WhereIn("id", ids).Fields("id,name,remark").All()
	res := map[int64]string{}
	for _, row := range rows {
		res[row["id"].Int64()] = row["name"].String()
		if res[row["id"].Int64()] == "" {
			res[row["id"].Int64()] = row["remark"].String()
		}
	}
	return res
}

func timeFromMillis(value int64) *gtime.Time {
	if value <= 0 {
		return nil
	}
	return gtime.NewFromTimeStamp(value / 1000)
}

func randomTenantVipCouponCode() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("VIP%d", gtime.Now().Timestamp())
	}
	return fmt.Sprintf("VIP%X", buf)
}
