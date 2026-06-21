package contentimport

import (
	"hotgo/internal/model/input/form"
	"hotgo/internal/model/input/sysin"

	"github.com/gogf/gf/v2/frame/g"
)

// RunFeiNiuReq 手动执行 FeiNiu 导入。
type RunFeiNiuReq struct {
	g.Meta `path:"/contentImport/runFeiNiu" method:"post" tags:"内容导入" summary:"手动执行 FeiNiu 导入"`
	sysin.ContentImportFeiNiuInp
}

type RunFeiNiuRes struct {
	*sysin.ContentImportFeiNiuModel
}

// OverviewReq 获取 FeiNiu 导入概览。
type OverviewReq struct {
	g.Meta `path:"/contentImport/overview" method:"get" tags:"内容导入" summary:"获取内容导入概览"`
	sysin.ContentImportOverviewInp
}

type OverviewRes struct {
	*sysin.ContentImportOverviewModel
}

// RunListReq 获取 FeiNiu 导入运行记录。
type RunListReq struct {
	g.Meta `path:"/contentImport/runList" method:"get" tags:"内容导入" summary:"获取内容导入运行记录"`
	sysin.ContentImportRunListInp
}

type RunListRes struct {
	form.PageRes
	List []*sysin.ContentImportRunListModel `json:"list" dc:"运行记录"`
}

// AutoSyncReq 设置 FeiNiu 自动同步状态。
type AutoSyncReq struct {
	g.Meta `path:"/contentImport/autoSync" method:"post" tags:"内容导入" summary:"设置 FeiNiu 自动同步状态"`
	sysin.ContentImportAutoSyncInp
}

type AutoSyncRes struct {
	*sysin.ContentImportAutoSyncModel
}

// ReviewConfigReq 获取内容审核配置。
type ReviewConfigReq struct {
	g.Meta `path:"/contentImport/reviewConfig" method:"get" tags:"内容导入" summary:"获取内容审核配置"`
	sysin.ContentImportReviewConfigInp
}

type ReviewConfigRes struct {
	*sysin.ContentImportReviewConfigModel
}

// SaveReviewConfigReq 保存内容审核配置。
type SaveReviewConfigReq struct {
	g.Meta `path:"/contentImport/saveReviewConfig" method:"post" tags:"内容导入" summary:"保存内容审核配置"`
	sysin.ContentImportReviewConfigEditInp
}

type SaveReviewConfigRes struct {
	*sysin.ContentImportReviewConfigModel
}
