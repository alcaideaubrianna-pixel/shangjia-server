package sysin

type MediaAssetsInp struct {
	MediaType string `json:"mediaType" dc:"媒体类型"`
	LocalPath string `json:"localPath" dc:"本地文件路径"`
	FileURL   string `json:"fileUrl" dc:"媒体远程地址"`
	PosterURL string `json:"posterUrl" dc:"视频预览图地址"`
	FileName  string `json:"fileName" dc:"文件名"`
}

type MediaAssetsModel struct {
	Processed         bool   `json:"processed" dc:"是否已处理"`
	PerceptualHash    string `json:"perceptualHash" dc:"感知哈希"`
	PosterUrl         string `json:"posterUrl" dc:"视频封面地址"`
	PosterStoragePath string `json:"posterStoragePath" dc:"视频封面存储路径"`
}
