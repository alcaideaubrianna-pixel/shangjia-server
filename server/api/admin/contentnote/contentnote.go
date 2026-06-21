package contentnote

import (
	"hotgo/internal/model/input/form"
	"hotgo/internal/model/input/sysin"

	"github.com/gogf/gf/v2/frame/g"
)

// ListReq 查询内容笔记列表。
type ListReq struct {
	g.Meta `path:"/contentNote/list" method:"get" tags:"内容笔记" summary:"获取内容笔记列表"`
	sysin.ContentNoteListInp
}

type ListRes struct {
	form.PageRes
	List []*sysin.ContentNoteListModel `json:"list" dc:"数据列表"`
}

// ViewReq 获取内容笔记详情。
type ViewReq struct {
	g.Meta `path:"/contentNote/view" method:"get" tags:"内容笔记" summary:"获取内容笔记详情"`
	sysin.ContentNoteViewInp
}

type ViewRes struct {
	*sysin.ContentNoteViewModel
}

// EditReq 修改内容笔记。
type EditReq struct {
	g.Meta `path:"/contentNote/edit" method:"post" tags:"内容笔记" summary:"修改内容笔记"`
	sysin.ContentNoteEditInp
}

type EditRes struct{}

// MediaEditReq 修改内容笔记媒体。
type MediaEditReq struct {
	g.Meta `path:"/contentNote/mediaEdit" method:"post" tags:"内容笔记" summary:"修改内容笔记媒体"`
	sysin.ContentNoteMediaEditInp
}

type MediaEditRes struct{}

// BatchDeleteReq 批量删除内容笔记。
type BatchDeleteReq struct {
	g.Meta `path:"/contentNote/batchDelete" method:"post" tags:"内容笔记" summary:"批量删除内容笔记"`
	sysin.ContentNoteBatchDeleteInp
}

type BatchDeleteRes struct{}

// BatchReviewReq 批量审核内容笔记。
type BatchReviewReq struct {
	g.Meta `path:"/contentNote/batchReview" method:"post" tags:"内容笔记" summary:"批量审核内容笔记"`
	sysin.ContentNoteBatchReviewInp
}

type BatchReviewRes struct{}

// BatchStatusReq 批量更新内容笔记状态。
type BatchStatusReq struct {
	g.Meta `path:"/contentNote/batchStatus" method:"post" tags:"内容笔记" summary:"批量更新内容笔记状态"`
	sysin.ContentNoteBatchStatusInp
}

type BatchStatusRes struct{}

// BatchRemarkReq 批量备注内容笔记。
type BatchRemarkReq struct {
	g.Meta `path:"/contentNote/batchRemark" method:"post" tags:"内容笔记" summary:"批量备注内容笔记"`
	sysin.ContentNoteBatchRemarkInp
}

type BatchRemarkRes struct{}
