// Package epay 彩虹易支付
package epay

type notifyRequest struct {
	Pid         string `json:"pid"`
	TradeNo     string `json:"trade_no"`
	OutTradeNo  string `json:"out_trade_no"`
	ApiTradeNo  string `json:"api_trade_no"`
	Type        string `json:"type"`
	TradeStatus string `json:"trade_status"`
	Addtime     string `json:"addtime"`
	Endtime     string `json:"endtime"`
	Name        string `json:"name"`
	Money       string `json:"money"`
	Param       string `json:"param"`
	Buyer       string `json:"buyer"`
	Timestamp   string `json:"timestamp"`
	Sign        string `json:"sign"`
	SignType    string `json:"sign_type"`
}
