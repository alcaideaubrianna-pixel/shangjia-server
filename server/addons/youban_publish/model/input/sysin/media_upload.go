package sysin

// MediaMultipartAttachInp 将已完成的对象存储附件绑定到资料媒体。
type MediaMultipartAttachInp struct {
	MediaUploadInp
	AttachmentId int64 `json:"attachmentId" v:"required|min:1#附件不能为空|附件不能为空" dc:"附件ID"`
}
