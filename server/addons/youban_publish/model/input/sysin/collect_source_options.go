package sysin

type CollectSourceOptionModel struct {
	Id       int64  `json:"id" dc:"采集源ID"`
	Label    string `json:"label" dc:"采集源显示名称"`
	Username string `json:"username" dc:"采集源用户名"`
}
