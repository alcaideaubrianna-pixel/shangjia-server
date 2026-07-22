package publish

import (
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/frame/g"
)

type AdminMaterialImportTaskListReq struct {
	g.Meta `path:"/publish/admin/materialImport/list" method:"get" tags:"上架插件管理端" summary:"资料导入任务列表"`
	sysin.MaterialImportListInp
}

type AdminMaterialImportTaskListRes struct {
	form.PageRes
	List []*sysin.MaterialImportTaskModel `json:"list" dc:"任务列表"`
}

type AdminMaterialImportTaskCreateReq struct {
	g.Meta `path:"/publish/admin/materialImport/create" method:"post" tags:"上架插件管理端" summary:"创建资料导入任务"`
	sysin.MaterialImportTaskSaveInp
}

type AdminMaterialImportTaskCreateRes struct {
	Id int64 `json:"id" dc:"任务ID"`
}

type AdminMaterialImportTaskViewReq struct {
	g.Meta `path:"/publish/admin/materialImport/view" method:"get" tags:"上架插件管理端" summary:"资料导入任务详情"`
	sysin.MaterialImportTaskViewInp
}

type AdminMaterialImportTaskViewRes struct {
	*sysin.MaterialImportTaskModel
}

type AdminMaterialImportTaskStartReq struct {
	g.Meta `path:"/publish/admin/materialImport/start" method:"post" tags:"上架插件管理端" summary:"启动资料导入任务"`
	sysin.MaterialImportTaskActionInp
}

type AdminMaterialImportTaskStartRes struct{}

type AdminMaterialImportTaskCancelReq struct {
	g.Meta `path:"/publish/admin/materialImport/cancel" method:"post" tags:"上架插件管理端" summary:"取消资料导入任务"`
	sysin.MaterialImportTaskActionInp
}

type AdminMaterialImportTaskCancelRes struct{}

type AdminMaterialImportTaskRetryReq struct {
	g.Meta `path:"/publish/admin/materialImport/retry" method:"post" tags:"上架插件管理端" summary:"重试资料导入任务"`
	sysin.MaterialImportTaskActionInp
}

type AdminMaterialImportTaskRetryRes struct{}
