package api

import (
	"context"
	"strings"

	"hotgo/addons/youban_open/api/api/open"
	"hotgo/addons/youban_open/internal/opencontext"
	addonsSysin "hotgo/addons/youban_open/model/input/sysin"
	addonService "hotgo/addons/youban_open/service"
	"hotgo/internal/library/profilescope"
	"hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
)

var OpenProfile = cOpenProfile{}

type cOpenProfile struct{}

func (c *cOpenProfile) List(ctx context.Context, req *open.ListReq) (res *open.ListRes, err error) {
	scopedCtx, err := openProfileContext(ctx)
	if err != nil {
		return nil, err
	}
	in := req.ContentProfileListInp
	feed := strings.ToLower(strings.TrimSpace(in.Feed))
	if feed == "region" {
		in.Feed = "latest"
	}
	if (feed == "hot" || feed == "recommended") && !hasAdvancedProfileFilters(in) {
		ids, rankErr := addonService.OpenAccess().RankedProfileIds(ctx, opencontext.AppId(ctx), in.ActorId, feed, 500)
		if rankErr != nil {
			return nil, rankErr
		}
		if len(ids) > 0 {
			base, rankedInput := in, in
			originalPage, originalSize := base.Page, base.PerPage
			rankedInput.ProfileIds, rankedInput.Page, rankedInput.PerPage = ids, 1, len(ids)
			rankedInput.WithTotal, rankedInput.Feed = 1, "latest"
			candidates, _, listErr := service.SysContent().ListProfiles(scopedCtx, &rankedInput)
			if listErr != nil {
				return nil, listErr
			}
			fallbackInput := base
			fallbackInput.ProfileIds, fallbackInput.Page, fallbackInput.PerPage = nil, 1, 500
			fallbackInput.WithTotal, fallbackInput.Feed = 1, "latest"
			fallback, fallbackTotal, fallbackErr := service.SysContent().ListProfiles(scopedCtx, &fallbackInput)
			if fallbackErr != nil {
				return nil, fallbackErr
			}
			ordered := appendUniqueProfiles(orderRankedProfiles(ids, candidates), fallback)
			list, _ := paginateRankedProfiles(ordered, originalPage, originalSize)
			res = &open.ListRes{List: list}
			res.PageRes.Page, res.PageRes.PerPage = originalPage, originalSize
			if res.PageRes.Page <= 0 {
				res.PageRes.Page = 1
			}
			if res.PageRes.PerPage <= 0 {
				res.PageRes.PerPage = 20
			}
			res.PageRes.TotalCount = fallbackTotal
			res.PageRes.PageCount = (fallbackTotal + res.PageRes.PerPage - 1) / res.PageRes.PerPage
			return res, nil
		}
		in.Feed = "latest"
	}
	list, total, err := service.SysContent().ListProfiles(scopedCtx, &in)
	if err != nil {
		return nil, err
	}
	res = &open.ListRes{List: list}
	res.PageRes.Pack(req, total)
	return res, nil
}

func hasAdvancedProfileFilters(in sysin.ContentProfileListInp) bool {
	return strings.TrimSpace(in.Keyword) != "" || in.AgeMin > 0 || in.AgeMax > 0 ||
		in.HeightMin > 0 || in.HeightMax > 0 || strings.TrimSpace(in.Cups) != "" ||
		strings.TrimSpace(in.Cup) != ""
}

func appendUniqueProfiles(ranked, fallback []*sysin.ContentProfileListModel) []*sysin.ContentProfileListModel {
	seen := make(map[int64]bool, len(ranked)+len(fallback))
	merged := make([]*sysin.ContentProfileListModel, 0, len(ranked)+len(fallback))
	for _, rows := range [][]*sysin.ContentProfileListModel{ranked, fallback} {
		for _, row := range rows {
			if row == nil || seen[row.Id] {
				continue
			}
			seen[row.Id] = true
			merged = append(merged, row)
		}
	}
	return merged
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

func orderRankedProfiles(ids []int64, rows []*sysin.ContentProfileListModel) []*sysin.ContentProfileListModel {
	byId := make(map[int64]*sysin.ContentProfileListModel, len(rows))
	for _, row := range rows {
		byId[row.Id] = row
	}
	ordered := make([]*sysin.ContentProfileListModel, 0, len(rows))
	for _, id := range ids {
		if row := byId[id]; row != nil {
			ordered = append(ordered, row)
		}
	}
	return ordered
}

func paginateRankedProfiles(rows []*sysin.ContentProfileListModel, page, size int) ([]*sysin.ContentProfileListModel, int) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	start := (page - 1) * size
	if start >= len(rows) {
		return []*sysin.ContentProfileListModel{}, len(rows)
	}
	end := start + size
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end], len(rows)
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
