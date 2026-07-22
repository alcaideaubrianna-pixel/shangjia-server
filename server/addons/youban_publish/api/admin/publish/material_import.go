package publish

import (
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/frame/g"
)

type MaterialImportTaskListReq struct {
	g.Meta `path:"/publish/materialImport/list" method:"get" tags:"上架插件后台" summary:"资料导入任务列表"`
	sysin.MaterialImportListInp
}

type MaterialImportTaskListRes struct {
	form.PageRes
	List []*sysin.MaterialImportTaskModel `json:"list" dc:"任务列表"`
}

type MaterialImportTaskCreateReq struct {
	g.Meta `path:"/publish/materialImport/create" method:"post" tags:"上架插件后台" summary:"创建资料导入任务"`
	sysin.MaterialImportTaskSaveInp
}

type MaterialImportTaskCreateRes struct {
	Id int64 `json:"id" dc:"任务ID"`
}

type MaterialImportTaskViewReq struct {
	g.Meta `path:"/publish/materialImport/view" method:"get" tags:"上架插件后台" summary:"资料导入任务详情"`
	sysin.MaterialImportTaskViewInp
}

type MaterialImportTaskViewRes struct {
	*sysin.MaterialImportTaskModel
}

type MaterialImportTaskStartReq struct {
	g.Meta `path:"/publish/materialImport/start" method:"post" tags:"上架插件后台" summary:"启动资料导入任务"`
	sysin.MaterialImportTaskActionInp
}

type MaterialImportTaskStartRes struct{}

type MaterialImportTaskCancelReq struct {
	g.Meta `path:"/publish/materialImport/cancel" method:"post" tags:"上架插件后台" summary:"取消资料导入任务"`
	sysin.MaterialImportTaskActionInp
}

type MaterialImportTaskCancelRes struct{}

type MaterialImportTaskRetryReq struct {
	g.Meta `path:"/publish/materialImport/retry" method:"post" tags:"上架插件后台" summary:"重试资料导入任务"`
	sysin.MaterialImportTaskActionInp
}

type MaterialImportTaskRetryRes struct{}
