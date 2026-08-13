package sysin

// MediaMultipartAttachInp 将已完成的对象存储附件绑定到资料媒体。
type MediaMultipartAttachInp struct {
	MediaUploadInp
	AttachmentId int64 `json:"attachmentId" v:"required|min:1#附件不能为空|附件不能为空" dc:"附件ID"`
}

type MediaDirectUploadCreateInp struct {
	MediaUploadInp
	FileName    string `json:"fileName" v:"required#文件名不能为空"`
	FileSize    int64  `json:"fileSize" v:"required|min:1#文件大小不能为空|文件大小不合法"`
	ContentType string `json:"contentType"`
}

type MediaDirectUploadCreateModel struct {
	SessionId string `json:"sessionId"`
	Bucket    string `json:"bucket"`
	Region    string `json:"region"`
	Key       string `json:"key"`
}

type MediaDirectUploadSignInp struct {
	SessionId string              `json:"sessionId" v:"required#上传会话不能为空"`
	Method    string              `json:"method" v:"required#签名方法不能为空"`
	Key       string              `json:"key" dc:"对象路径，Bucket级分片查询时为空"`
	Query     map[string][]string `json:"query"`
	Headers   map[string][]string `json:"headers"`
}

type MediaDirectUploadSignModel struct {
	Authorization string `json:"authorization"`
}

type MediaDirectUploadCompleteInp struct {
	SessionId string `json:"sessionId" v:"required#上传会话不能为空"`
}
