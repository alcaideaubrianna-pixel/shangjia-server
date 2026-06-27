package sys

import (
	"context"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
	"hotgo/internal/library/hgorm/handler"
	"hotgo/internal/model/input/form"
	"hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
	"hotgo/utility/validate"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"
)

type sSysAppAnnouncement struct{}

func NewSysAppAnnouncement() *sSysAppAnnouncement {
	return &sSysAppAnnouncement{}
}

func init() {
	service.RegisterSysAppAnnouncement(NewSysAppAnnouncement())
}

func (s *sSysAppAnnouncement) Model(ctx context.Context, option ...*handler.Option) *gdb.Model {
	return handler.Model(dao.AppAnnouncement.Ctx(ctx), option...)
}

func (s *sSysAppAnnouncement) List(ctx context.Context, in *sysin.AppAnnouncementListInp) (list []*sysin.AppAnnouncementListModel, totalCount int, err error) {
	mod := s.Model(ctx)
	cols := dao.AppAnnouncement.Columns()
	if in.Title != "" {
		mod = mod.WhereLike(cols.Title, "%"+in.Title+"%")
	}
	if in.CategoryCode != "" {
		mod = mod.Where(cols.CategoryCode, in.CategoryCode)
	}
	if in.IsBanner > 0 {
		mod = mod.Where(cols.IsBanner, in.IsBanner)
	}
	if in.Status > 0 {
		mod = mod.Where(cols.Status, in.Status)
	}
	if len(in.CreatedAt) == 2 {
		mod = mod.WhereBetween(cols.CreatedAt, in.CreatedAt[0], in.CreatedAt[1])
	}
	mod = mod.Page(in.Page, in.PerPage).OrderDesc(cols.Sort).OrderDesc(cols.Id)
	if err = mod.ScanAndCount(&list, &totalCount, false); err != nil {
		err = gerror.Wrap(err, "获取APP公告列表失败，请稍后重试！")
	}
	return
}

func (s *sSysAppAnnouncement) PublicList(ctx context.Context, in *sysin.AppAnnouncementPublicListInp) (list []*sysin.AppAnnouncementPublicListModel, totalCount int, err error) {
	cols := dao.AppAnnouncement.Columns()
	now := gtime.Now()
	mod := s.Model(ctx, &handler.Option{FilterAuth: false}).
		Where(cols.Status, consts.StatusEnabled).
		Where("(publish_at IS NULL OR publish_at<=?)", now).
		Where("(expire_at IS NULL OR expire_at>?)", now)
	if in.IsBanner >= 0 {
		mod = mod.Where(cols.IsBanner, in.IsBanner)
	}
	if in.CategoryCode != "" {
		mod = mod.Where(cols.CategoryCode, in.CategoryCode)
	}
	mod = mod.Page(in.Page, in.PerPage).OrderDesc(cols.Sort).OrderDesc(cols.Id)
	if err = mod.ScanAndCount(&list, &totalCount, false); err != nil {
		err = gerror.Wrap(err, "获取APP公告列表失败，请稍后重试！")
	}
	return
}

func (s *sSysAppAnnouncement) PublicCategories(ctx context.Context) (list []*sysin.AppAnnouncementCategoryModel, err error) {
	list = sysin.ArticleCategories()
	cols := dao.AppAnnouncement.Columns()
	now := gtime.Now()
	for _, item := range list {
		count, countErr := s.Model(ctx, &handler.Option{FilterAuth: false}).
			Where(cols.Status, consts.StatusEnabled).
			Where(cols.IsBanner, 0).
			Where(cols.CategoryCode, item.Code).
			Where("(publish_at IS NULL OR publish_at<=?)", now).
			Where("(expire_at IS NULL OR expire_at>?)", now).
			Count()
		if countErr != nil {
			return nil, gerror.Wrap(countErr, "获取文章分类数量失败")
		}
		item.TotalCount = count
	}
	return
}

func (s *sSysAppAnnouncement) PublicView(ctx context.Context, in *sysin.AppAnnouncementPublicViewInp) (res *sysin.AppAnnouncementPublicListModel, err error) {
	cols := dao.AppAnnouncement.Columns()
	now := gtime.Now()
	err = s.Model(ctx, &handler.Option{FilterAuth: false}).
		Where(cols.Id, in.Id).
		Where(cols.Status, consts.StatusEnabled).
		Where("(publish_at IS NULL OR publish_at<=?)", now).
		Where("(expire_at IS NULL OR expire_at>?)", now).
		Scan(&res)
	if err != nil {
		return nil, gerror.Wrap(err, "获取APP公告详情失败，请稍后重试！")
	}
	if res == nil {
		return nil, gerror.New("公告不存在或未发布")
	}
	return
}

func (s *sSysAppAnnouncement) SeoFooter(ctx context.Context) (res *sysin.SeoFooterModel, err error) {
	config, err := service.SysConfig().GetConfigByGroup(ctx, &sysin.GetConfigInp{Group: "seo"})
	if err != nil {
		return nil, err
	}
	res = new(sysin.SeoFooterModel)
	raw := config.List["seoFooter"]
	if raw == nil {
		return
	}
	if err = gjson.New(raw).Scan(res); err != nil {
		err = gerror.Wrap(err, "解析SEO页脚配置失败")
	}
	return
}

func (s *sSysAppAnnouncement) View(ctx context.Context, in *sysin.AppAnnouncementViewInp) (res *sysin.AppAnnouncementViewModel, err error) {
	err = s.Model(ctx).WherePri(in.Id).Scan(&res)
	if err != nil {
		err = gerror.Wrap(err, "获取APP公告详情失败，请稍后重试！")
	}
	return
}

func (s *sSysAppAnnouncement) Edit(ctx context.Context, in *sysin.AppAnnouncementEditInp) (err error) {
	memberId := contexts.GetUserId(ctx)
	if in.Id > 0 {
		in.UpdatedBy = memberId
		_, err = s.Model(ctx).Fields(sysin.AppAnnouncementUpdateFields{}).WherePri(in.Id).Data(in).Update()
	} else {
		in.CreatedBy = memberId
		_, err = dao.AppAnnouncement.Ctx(ctx).Fields(sysin.AppAnnouncementInsertFields{}).Data(in).Insert()
	}
	if err != nil {
		err = gerror.Wrap(err, "保存APP公告失败，请稍后重试！")
	}
	return
}

func (s *sSysAppAnnouncement) Delete(ctx context.Context, in *sysin.AppAnnouncementDeleteInp) (err error) {
	_, err = s.Model(ctx).WherePri(in.Id).Delete()
	if err != nil {
		err = gerror.Wrap(err, "删除APP公告失败，请稍后重试！")
	}
	return
}

func (s *sSysAppAnnouncement) Status(ctx context.Context, in *sysin.AppAnnouncementStatusInp) (err error) {
	if !validate.InSlice(consts.StatusSlice, in.Status) {
		return gerror.New("状态不正确")
	}
	_, err = s.Model(ctx).WherePri(in.Id).Data(dao.AppAnnouncement.Columns().Status, in.Status).Update()
	if err != nil {
		err = gerror.Wrap(err, "更新APP公告状态失败，请稍后重试！")
	}
	return
}

func (s *sSysAppAnnouncement) MaxSort(ctx context.Context, in *sysin.AppAnnouncementMaxSortInp) (res *sysin.AppAnnouncementMaxSortModel, err error) {
	cols := dao.AppAnnouncement.Columns()
	if err = dao.AppAnnouncement.Ctx(ctx).Fields(cols.Sort).OrderDesc(cols.Sort).Scan(&res); err != nil {
		err = gerror.Wrap(err, "获取APP公告最大排序失败，请稍后重试！")
		return
	}
	if res == nil {
		res = new(sysin.AppAnnouncementMaxSortModel)
	}
	res.Sort = form.DefaultMaxSort(res.Sort)
	return
}
