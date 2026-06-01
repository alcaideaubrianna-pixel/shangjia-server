package consts

import (
	"hotgo/internal/library/dict"
	"hotgo/internal/model"
)

const (
	ContentVisibilityPrivate    = "private"
	ContentVisibilityPublic     = "public"
	ContentVisibilityMemberOnly = "member_only"
)

const (
	ContentReviewPending  = "pending"
	ContentReviewApproved = "approved"
	ContentReviewRejected = "rejected"
)

const (
	ContentMediaTypeImage = "image"
	ContentMediaTypeVideo = "video"
)

var ContentVisibilityOptions = []*model.Option{
	dict.GenWarningOption(ContentVisibilityPrivate, "私有"),
	dict.GenSuccessOption(ContentVisibilityPublic, "公开"),
	dict.GenPrimaryOption(ContentVisibilityMemberOnly, "会员可见"),
}

var ContentReviewStatusOptions = []*model.Option{
	dict.GenWarningOption(ContentReviewPending, "待审核"),
	dict.GenSuccessOption(ContentReviewApproved, "已通过"),
	dict.GenErrorOption(ContentReviewRejected, "已拒绝"),
}

var ContentMediaTypeOptions = []*model.Option{
	dict.GenSuccessOption(ContentMediaTypeImage, "图片"),
	dict.GenPrimaryOption(ContentMediaTypeVideo, "视频"),
}

func init() {
	dict.RegisterEnums("ContentVisibilityOptions", "内容可见性选项", ContentVisibilityOptions)
	dict.RegisterEnums("ContentReviewStatusOptions", "内容审核状态选项", ContentReviewStatusOptions)
	dict.RegisterEnums("ContentMediaTypeOptions", "内容媒体类型选项", ContentMediaTypeOptions)
}
