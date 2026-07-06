package publish

import (
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"
	basesysin "hotgo/internal/model/input/sysin"

	"github.com/gogf/gf/v2/frame/g"
)

type AccountProfileViewReq struct {
	g.Meta `path:"/publish/account/profile/view" method:"get" tags:"上架插件" summary:"账号主页"`
	sysin.AccountProfileViewInp
}

type AccountProfileViewRes struct {
	*sysin.AccountProfileModel
}

type AccountProfileSaveReq struct {
	g.Meta `path:"/publish/account/profile/save" method:"post" tags:"上架插件" summary:"保存账号资料"`
	sysin.AccountProfileSaveInp
}

type AccountProfileSaveRes struct {
	*sysin.AccountProfileModel
}

type AccountUploadReq struct {
	g.Meta `path:"/publish/account/upload" method:"post" mime:"multipart/form-data" tags:"上架插件" summary:"上传账号图片"`
}

type AccountUploadRes struct {
	*basesysin.AttachmentListModel
}

type AccountFollowListReq struct {
	g.Meta `path:"/publish/account/follow/list" method:"get" tags:"上架插件" summary:"账号关注列表"`
	sysin.AccountFollowListInp
}

type AccountFollowListRes struct {
	form.PageRes
	List []*sysin.AccountFollowModel `json:"list" dc:"关注列表"`
}

type AccountFollowApplyReq struct {
	g.Meta `path:"/publish/account/follow/apply" method:"post" tags:"上架插件" summary:"关注账号"`
	sysin.AccountFollowApplyInp
}

type AccountFollowApplyRes struct{}

type AccountFollowActionReq struct {
	g.Meta `path:"/publish/account/follow/action" method:"post" tags:"上架插件" summary:"关注审批/拉黑"`
	sysin.AccountFollowActionInp
}

type AccountFollowActionRes struct{}

type FollowNoteListReq struct {
	g.Meta `path:"/publish/follow/note/list" method:"get" tags:"上架插件" summary:"关注账号笔记列表"`
	sysin.FollowNoteListInp
}

type FollowNoteListRes struct {
	form.PageRes
	List []*sysin.NoteModel `json:"list" dc:"笔记列表"`
}

type FollowNoteImageSearchReq struct {
	g.Meta `path:"/publish/follow/note/image-search" method:"post" mime:"multipart/form-data" tags:"上架插件" summary:"关注账号笔记图片搜索"`
	sysin.FollowNoteListInp
}

type FollowNoteImageSearchRes struct {
	form.PageRes
	List []*sysin.NoteModel `json:"list" dc:"笔记列表"`
}
