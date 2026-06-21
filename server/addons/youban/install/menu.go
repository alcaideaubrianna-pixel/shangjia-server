package install

import (
	"context"
	"hotgo/internal/dao"
	"hotgo/internal/model/do"
	"hotgo/internal/model/entity"
	"hotgo/utility/tree"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

type menuSpec struct {
	ParentName  string
	Title       string
	Name        string
	Path        string
	Icon        string
	Type        int
	Redirect    string
	Permissions string
	Component   string
	AlwaysShow  int
	ActiveMenu  string
	IsRoot      int
	IsFrame     int
	FrameSrc    string
	KeepAlive   int
	Hidden      int
	Affix       int
	Sort        int
	Remark      string
	Status      int
}

var youbanMenuSpecs = []menuSpec{
	{Title: "内容管理", Name: "Content", Path: "/content", Icon: "AppstoreOutlined", Type: 1, Redirect: "/content/importMonitor", Component: "LAYOUT", AlwaysShow: 1, Sort: 90, Status: 1},
	{ParentName: "Content", Title: "同步监控", Name: "ContentImportMonitor", Path: "importMonitor", Type: 2, Component: "/content/importMonitor/index", AlwaysShow: 1, Sort: 10, Status: 1},
	{ParentName: "Content", Title: "笔记管理", Name: "ContentNote", Path: "note", Type: 2, Component: "/content/note/index", AlwaysShow: 1, Sort: 20, Status: 1},
	{ParentName: "Content", Title: "公告展示", Name: "ContentAnnouncement", Path: "announcement", Type: 2, Component: "/content/announcement/index", AlwaysShow: 1, Sort: 30, Status: 1},
	{ParentName: "ContentAnnouncement", Title: "APP公告列表", Name: "AppAnnouncementList", Type: 3, Permissions: "/appAnnouncement/list", AlwaysShow: 1, Hidden: 1, Sort: 10, Status: 1},
	{ParentName: "ContentAnnouncement", Title: "编辑APP公告", Name: "AppAnnouncementEdit", Type: 3, Permissions: "/appAnnouncement/edit", AlwaysShow: 1, Hidden: 1, Sort: 20, Status: 1},
	{ParentName: "ContentAnnouncement", Title: "更新APP公告状态", Name: "AppAnnouncementStatus", Type: 3, Permissions: "/appAnnouncement/status", AlwaysShow: 1, Hidden: 1, Sort: 30, Status: 1},
	{ParentName: "ContentAnnouncement", Title: "删除APP公告", Name: "AppAnnouncementDelete", Type: 3, Permissions: "/appAnnouncement/delete", AlwaysShow: 1, Hidden: 1, Sort: 40, Status: 1},
	{ParentName: "ContentAnnouncement", Title: "APP公告最大排序", Name: "AppAnnouncementMaxSort", Type: 3, Permissions: "/appAnnouncement/maxSort", AlwaysShow: 1, Hidden: 1, Sort: 50, Status: 1},
	{ParentName: "ContentNote", Title: "笔记列表", Name: "ContentNoteList", Type: 3, Permissions: "/contentNote/list", AlwaysShow: 1, Hidden: 1, Sort: 10, Status: 1},
	{ParentName: "ContentNote", Title: "笔记详情", Name: "ContentNoteView", Type: 3, Permissions: "/contentNote/view", AlwaysShow: 1, Hidden: 1, Sort: 20, Status: 1},
	{ParentName: "ContentNote", Title: "修改笔记", Name: "ContentNoteEdit", Type: 3, Permissions: "/contentNote/edit", AlwaysShow: 1, Hidden: 1, Sort: 30, Status: 1},
	{ParentName: "ContentNote", Title: "修改媒体", Name: "ContentNoteMediaEdit", Type: 3, Permissions: "/contentNote/mediaEdit", AlwaysShow: 1, Hidden: 1, Sort: 40, Status: 1},
	{ParentName: "ContentNote", Title: "批量删除笔记", Name: "ContentNoteBatchDelete", Type: 3, Permissions: "/contentNote/batchDelete", AlwaysShow: 1, Hidden: 1, Sort: 50, Status: 1},
	{ParentName: "ContentNote", Title: "批量审核笔记", Name: "ContentNoteBatchReview", Type: 3, Permissions: "/contentNote/batchReview", AlwaysShow: 1, Hidden: 1, Sort: 60, Status: 1},
	{ParentName: "ContentNote", Title: "批量状态笔记", Name: "ContentNoteBatchStatus", Type: 3, Permissions: "/contentNote/batchStatus", AlwaysShow: 1, Hidden: 1, Sort: 70, Status: 1},
	{ParentName: "ContentNote", Title: "批量备注笔记", Name: "ContentNoteBatchRemark", Type: 3, Permissions: "/contentNote/batchRemark", AlwaysShow: 1, Hidden: 1, Sort: 80, Status: 1},
	{ParentName: "ContentImportMonitor", Title: "同步概览", Name: "ContentImportOverview", Type: 3, Permissions: "/contentImport/overview", AlwaysShow: 1, Hidden: 1, Sort: 10, Status: 1},
	{ParentName: "ContentImportMonitor", Title: "同步记录", Name: "ContentImportRunList", Type: 3, Permissions: "/contentImport/runList", AlwaysShow: 1, Hidden: 1, Sort: 20, Status: 1},
	{ParentName: "ContentImportMonitor", Title: "手动同步", Name: "ContentImportRunFeiNiu", Type: 3, Permissions: "/contentImport/runFeiNiu", AlwaysShow: 1, Hidden: 1, Sort: 30, Status: 1},
	{ParentName: "ContentImportMonitor", Title: "自动同步开关", Name: "ContentImportAutoSync", Type: 3, Permissions: "/contentImport/autoSync", AlwaysShow: 1, Hidden: 1, Sort: 40, Status: 1},
	{ParentName: "ContentImportMonitor", Title: "审核配置", Name: "ContentImportReviewConfig", Type: 3, Permissions: "/contentImport/reviewConfig", AlwaysShow: 1, Hidden: 1, Sort: 50, Status: 1},
	{ParentName: "ContentImportMonitor", Title: "保存审核配置", Name: "ContentImportSaveReviewConfig", Type: 3, Permissions: "/contentImport/saveReviewConfig", AlwaysShow: 1, Hidden: 1, Sort: 60, Status: 1},
	{ParentName: "Org", Title: "会员日志", Name: "OrgVipLog", Path: "vip-log", Type: 2, Permissions: "/member/vipLogList", Component: "/org/vipLog/index", AlwaysShow: 1, Sort: 15, Status: 1},
}

func installMenus(ctx context.Context) error {
	return dao.AdminMenu.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, spec := range youbanMenuSpecs {
			menu, err := upsertMenu(ctx, spec)
			if err != nil {
				return err
			}
			if err = grantMenuToDefaultRoles(ctx, menu.Id); err != nil {
				return err
			}
		}
		return nil
	})
}

func upsertMenu(ctx context.Context, spec menuSpec) (menu *entity.AdminMenu, err error) {
	parent, err := getParentMenu(ctx, spec.ParentName)
	if err != nil {
		return nil, err
	}
	data := menuData(spec, parent)
	if err = dao.AdminMenu.Ctx(ctx).Where(dao.AdminMenu.Columns().Name, spec.Name).Scan(&menu); err != nil {
		return nil, gerror.Wrapf(err, "读取菜单失败：%s", spec.Name)
	}
	if menu != nil && menu.Id > 0 {
		if _, err = dao.AdminMenu.Ctx(ctx).WherePri(menu.Id).Data(data).Update(); err != nil {
			return nil, gerror.Wrapf(err, "更新菜单失败：%s", spec.Name)
		}
		if err = dao.AdminMenu.Ctx(ctx).WherePri(menu.Id).Scan(&menu); err != nil {
			return nil, gerror.Wrapf(err, "读取菜单失败：%s", spec.Name)
		}
		return menu, nil
	}
	id, err := dao.AdminMenu.Ctx(ctx).Data(data).InsertAndGetId()
	if err != nil {
		return nil, gerror.Wrapf(err, "创建菜单失败：%s", spec.Name)
	}
	if err = dao.AdminMenu.Ctx(ctx).WherePri(id).Scan(&menu); err != nil {
		return nil, gerror.Wrapf(err, "读取菜单失败：%s", spec.Name)
	}
	return menu, nil
}

func getParentMenu(ctx context.Context, name string) (parent *entity.AdminMenu, err error) {
	if name == "" {
		return nil, nil
	}
	if err = dao.AdminMenu.Ctx(ctx).Where(dao.AdminMenu.Columns().Name, name).Scan(&parent); err != nil {
		return nil, gerror.Wrapf(err, "读取父级菜单失败：%s", name)
	}
	if parent == nil || parent.Id <= 0 {
		return nil, gerror.Newf("父级菜单不存在：%s", name)
	}
	return parent, nil
}

func menuData(spec menuSpec, parent *entity.AdminMenu) *do.AdminMenu {
	now := gtime.Now()
	pid := int64(0)
	level := 1
	treeValue := ""
	if parent != nil {
		pid = parent.Id
		level = parent.Level + 1
		treeValue = tree.GenLabel(parent.Tree, parent.Id)
	}
	return &do.AdminMenu{
		Pid:         pid,
		Level:       level,
		Tree:        treeValue,
		Title:       spec.Title,
		Name:        spec.Name,
		Path:        spec.Path,
		Icon:        spec.Icon,
		Type:        spec.Type,
		Redirect:    spec.Redirect,
		Permissions: spec.Permissions,
		Component:   spec.Component,
		AlwaysShow:  spec.AlwaysShow,
		ActiveMenu:  spec.ActiveMenu,
		IsRoot:      spec.IsRoot,
		IsFrame:     spec.IsFrame,
		FrameSrc:    spec.FrameSrc,
		KeepAlive:   spec.KeepAlive,
		Hidden:      spec.Hidden,
		Affix:       spec.Affix,
		Sort:        spec.Sort,
		Remark:      spec.Remark,
		Status:      spec.Status,
		UpdatedAt:   now,
		CreatedAt:   now,
	}
}

func grantMenuToDefaultRoles(ctx context.Context, menuId int64) (err error) {
	if menuId <= 0 {
		return nil
	}
	roleIds, err := dao.AdminRole.Ctx(ctx).
		Fields(dao.AdminRole.Columns().Id).
		WhereIn(dao.AdminRole.Columns().Id, []int64{1, 2}).
		Array()
	if err != nil {
		return gerror.Wrap(err, "读取默认授权角色失败")
	}
	for _, roleId := range roleIds {
		roleMenu := &do.AdminRoleMenu{RoleId: roleId.Int64(), MenuId: menuId}
		exists, err := dao.AdminRoleMenu.Ctx(ctx).Where(roleMenu).Exist()
		if err != nil {
			return gerror.Wrap(err, "检查角色菜单授权失败")
		}
		if exists {
			continue
		}
		if _, err = dao.AdminRoleMenu.Ctx(ctx).Data(roleMenu).Insert(); err != nil {
			return gerror.Wrap(err, "创建角色菜单授权失败")
		}
	}
	return nil
}

func refreshAdminMenuTree(ctx context.Context) error {
	var list []*entity.AdminMenu
	if err := dao.AdminMenu.Ctx(ctx).OrderAsc(dao.AdminMenu.Columns().Level).Scan(&list); err != nil {
		return gerror.Wrap(err, "读取菜单关系树失败")
	}
	menuMap := make(map[int64]*entity.AdminMenu, len(list))
	for _, item := range list {
		menuMap[item.Id] = item
	}
	for _, item := range list {
		level, treeValue := 1, ""
		if item.Pid > 0 {
			parent := menuMap[item.Pid]
			if parent == nil {
				continue
			}
			level = parent.Level + 1
			treeValue = tree.GenLabel(parent.Tree, parent.Id)
		}
		if item.Level == level && item.Tree == treeValue {
			continue
		}
		if _, err := dao.AdminMenu.Ctx(ctx).WherePri(item.Id).Data(g.Map{
			dao.AdminMenu.Columns().Level:     level,
			dao.AdminMenu.Columns().Tree:      treeValue,
			dao.AdminMenu.Columns().UpdatedAt: gtime.Now(),
		}).Update(); err != nil {
			return gerror.Wrapf(err, "更新菜单关系树失败：%s", item.Name)
		}
		item.Level = level
		item.Tree = treeValue
	}
	return nil
}
