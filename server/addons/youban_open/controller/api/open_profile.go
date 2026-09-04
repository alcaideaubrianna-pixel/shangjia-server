package api

import (
	"context"
	"github.com/gogf/gf/v2/errors/gerror"
	"regexp"
	"strings"

	"hotgo/addons/youban_open/api/api/open"
	"hotgo/addons/youban_open/internal/opencontext"
	addonsSysin "hotgo/addons/youban_open/model/input/sysin"
	addonService "hotgo/addons/youban_open/service"
	"hotgo/internal/dao"
	"hotgo/internal/library/profilescope"
	"hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
)

var OpenProfile = cOpenProfile{}

type cOpenProfile struct{}

func (c *cOpenProfile) List(ctx context.Context, req *open.ListReq) (res *open.ListRes, err error) {
	in := sysin.ContentProfileListInp{PageReq: req.PageReq, Feed: "latest", Province: strings.TrimSpace(req.ProvinceCode), City: strings.TrimSpace(req.CityCode), AgeMin: req.AgeMin, AgeMax: req.AgeMax, HeightMin: req.HeightMin, HeightMax: req.HeightMax, WeightMin: req.WeightMin, WeightMax: req.WeightMax, Cups: req.Cups, HasVideo: req.HasVideo, HasVerification: req.HasVerification, IsVirgin: req.IsVirgin}
	provinceCodes, err := normalizeProvinceCodes(req.ProvinceCode, req.ProvinceCodes)
	if err != nil {
		return nil, err
	}
	if len(provinceCodes) > 0 {
		in.Province = ""
		in.Provinces = strings.Join(provinceCodes, ",")
	}
	if code := strings.TrimSpace(req.ProvinceCode); code != "" {
		if !openRegionCodePattern.MatchString(code) {
			return nil, gerror.New("provinceCode 必须为6位行政区划编码")
		}
		in.Province = code
	}
	if code := strings.TrimSpace(req.CityCode); code != "" {
		if !openRegionCodePattern.MatchString(code) {
			return nil, gerror.New("cityCode 必须为6位行政区划编码")
		}
		in.City = code
	}
	if err := validateOpenRegionCodes(ctx, strings.Join(provinceCodes, ","), req.CityCode); err != nil {
		return nil, err
	}
	scopedCtx, err := openProfileContext(ctx)
	if err != nil {
		return nil, err
	}
	feed := strings.ToLower(strings.TrimSpace(req.Feed))
	if feed != "" && feed != "latest" {
		return nil, gerror.New("Open 资料列表仅支持 latest 排序")
	}
	// Open list deliberately avoids ranking and COUNT queries. Ranking has a
	// separate interaction pipeline and withTotal is not part of the contract.
	in.Feed, in.WithTotal = "latest", 0
	list, total, err := service.SysContent().ListProfiles(scopedCtx, &in)
	if err != nil {
		return nil, err
	}
	res = &open.ListRes{List: list}
	res.PageRes.Pack(req, total)
	return res, nil
}

func normalizeProvinceCodes(single, multiple string) ([]string, error) {
	values := make([]string, 0, 4)
	if strings.TrimSpace(single) != "" {
		values = append(values, strings.TrimSpace(single))
	}
	for _, value := range strings.Split(multiple, ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !openRegionCodePattern.MatchString(value) {
			return nil, gerror.New("provinceCodes 必须为6位行政区划编码")
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result, nil
}

var openRegionCodePattern = regexp.MustCompile(`^[0-9]{6}$`)

func validateOpenRegionCodes(ctx context.Context, provinceCode, cityCode string) error {
	columns := dao.SysProvinces.Columns()
	provinceCodes := strings.Split(provinceCode, ",")
	for _, code := range provinceCodes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		count, err := dao.SysProvinces.Ctx(ctx).Where(columns.Id, code).Where(columns.Pid, 0).Where(columns.Status, 1).Count()
		if err != nil {
			return gerror.Wrap(err, "校验省份行政区划编码失败")
		}
		if count == 0 {
			return gerror.New("provinceCode 不是有效的省份行政区划编码")
		}
	}
	if cityCode != "" {
		model := dao.SysProvinces.Ctx(ctx).Where(columns.Id, cityCode).Where(columns.Status, 1)
		normalizedProvinces := make([]string, 0, len(provinceCodes))
		for _, code := range provinceCodes {
			if code = strings.TrimSpace(code); code != "" {
				normalizedProvinces = append(normalizedProvinces, code)
			}
		}
		if len(normalizedProvinces) == 1 {
			model = model.Where(columns.Pid, normalizedProvinces[0])
		} else if len(normalizedProvinces) > 1 {
			model = model.WhereIn(columns.Pid, normalizedProvinces)
		} else {
			model = model.WhereGT(columns.Pid, 0)
		}
		count, err := model.Count()
		if err != nil {
			return gerror.Wrap(err, "校验城市行政区划编码失败")
		}
		if count == 0 {
			return gerror.New("cityCode 不是有效城市编码或不属于 provinceCode")
		}
	}
	return nil
}

func (c *cOpenProfile) Interaction(ctx context.Context, req *open.InteractionReq) (*open.InteractionRes, error) {
	accepted, err := addonService.OpenAccess().RecordInteraction(ctx, opencontext.AppId(ctx), &addonsSysin.ProfileInteractionInp{
		EventId: req.EventId, ActorId: req.ActorId, ProfileId: req.ProfileId, Type: req.Type, OccurredAt: req.OccurredAt,
	})
	if err != nil {
		return nil, err
	}
	return &open.InteractionRes{Accepted: true, Duplicate: !accepted}, nil
}

func (c *cOpenProfile) View(ctx context.Context, req *open.ViewReq) (res *open.ViewRes, err error) {
	scopedCtx, err := openProfileContext(ctx)
	if err != nil {
		return nil, err
	}
	data, err := service.SysContent().ViewProfile(scopedCtx, &sysin.ContentProfileViewInp{Id: req.Id})
	if err != nil {
		return nil, err
	}
	return &open.ViewRes{ContentProfileViewModel: data}, nil
}

func (c *cOpenProfile) Regions(ctx context.Context, req *open.RegionsReq) (res *open.RegionsRes, err error) {
	scopedCtx, err := openProfileContext(ctx)
	if err != nil {
		return nil, err
	}
	data, err := service.SysContent().Regions(scopedCtx)
	if err != nil {
		return nil, err
	}
	return &open.RegionsRes{ContentProfileRegionsModel: data}, nil
}

func (c *cOpenProfile) Batch(ctx context.Context, req *open.BatchReq) (res *open.BatchRes, err error) {
	scopedCtx, err := openProfileContext(ctx)
	if err != nil {
		return nil, err
	}
	in := req.ContentProfileListInp
	in.ProfileIds, in.WithTotal = req.Ids, 0
	in.Page, in.PerPage = 1, len(req.Ids)
	list, _, err := service.SysContent().ListProfiles(scopedCtx, &in)
	if err != nil {
		return nil, err
	}
	byId := make(map[int64]*sysin.ContentProfileListModel, len(list))
	for _, item := range list {
		byId[item.Id] = item
	}
	ordered := make([]*sysin.ContentProfileListModel, 0, len(list))
	for _, id := range req.Ids {
		if item := byId[id]; item != nil {
			ordered = append(ordered, item)
		}
	}
	return &open.BatchRes{List: ordered}, nil
}

func (c *cOpenProfile) BindingCode(ctx context.Context, req *open.BindingCodeReq) (res *open.BindingCodeRes, err error) {
	data, err := addonService.OpenAccess().SaveBindingCode(ctx, opencontext.AppId(ctx), &req.CmsBindingCodeSaveInp)
	if err != nil {
		return nil, err
	}
	return &open.BindingCodeRes{CmsBindingCodeModel: data}, nil
}

func (c *cOpenProfile) BindingList(ctx context.Context, req *open.BindingListReq) (res *open.BindingListRes, err error) {
	list, err := addonService.OpenAccess().Bindings(ctx, opencontext.AppId(ctx), &req.CmsBindingListInp)
	if err != nil {
		return nil, err
	}
	return &open.BindingListRes{List: list}, nil
}

func (c *cOpenProfile) BindingStatus(ctx context.Context, req *open.BindingStatusReq) (res *open.BindingStatusRes, err error) {
	data, err := addonService.OpenAccess().UpdateBinding(ctx, opencontext.AppId(ctx), &req.CmsBindingStatusInp)
	if err != nil {
		return nil, err
	}
	return &open.BindingStatusRes{CmsBindingModel: data}, nil
}

func (c *cOpenProfile) Settings(ctx context.Context, req *open.SettingsReq) (*open.SettingsRes, error) {
	data, err := addonService.OpenAccess().SaveAppSettings(ctx, opencontext.AppId(ctx), &req.CmsAppSettingsInp)
	if err != nil {
		return nil, err
	}
	return &open.SettingsRes{CmsAppSettingsModel: data}, nil
}

func openProfileContext(ctx context.Context) (context.Context, error) {
	tenantIds, err := addonService.OpenAccess().AllowedTenantIds(ctx, opencontext.AppId(ctx))
	if err != nil {
		return nil, err
	}
	return profilescope.WithTrustedTenantIds(ctx, tenantIds), nil
}
