// Package gmpay GMPay 推荐接入
package gmpay

type createTransactionRequest struct {
	Pid         string  `json:"pid"`
	OrderID     string  `json:"order_id"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	NotifyURL   string  `json:"notify_url"`
	RedirectURL string  `json:"redirect_url,omitempty"`
	Name        string  `json:"name,omitempty"`
	Token       string  `json:"token,omitempty"`
	Network     string  `json:"network,omitempty"`
	PaymentType string  `json:"payment_type,omitempty"`
	Signature   string  `json:"signature"`
}

type createTransactionResponse struct {
	StatusCode int    `json:"status_code"`
	Code       int    `json:"code"`
	Message    string `json:"message"`
	Msg        string `json:"msg"`
	RequestID  string `json:"request_id"`
	Data       struct {
		PaymentURL     string  `json:"payment_url"`
		TradeID        string  `json:"trade_id"`
		OrderID        string  `json:"order_id"`
		Amount         float64 `json:"amount"`
		ActualAmount   float64 `json:"actual_amount"`
		Currency       string  `json:"currency"`
		Token          string  `json:"token"`
		Network        string  `json:"network"`
		ReceiveAddress string  `json:"receive_address"`
	} `json:"data"`
	PaymentURL string `json:"payment_url"`
	TradeID    string `json:"trade_id"`
	OrderID    string `json:"order_id"`
}

type notifyRequest struct {
	Pid                string  `json:"pid"`
	Status             int     `json:"status"`
	Code               int     `json:"code"`
	OrderID            string  `json:"order_id"`
	TradeID            string  `json:"trade_id"`
	BlockTransactionID string  `json:"block_transaction_id"`
	ReceiveAddress     string  `json:"receive_address"`
	Token              string  `json:"token"`
	Network            string  `json:"network"`
	PaymentType        string  `json:"payment_type"`
	StatusCode         int     `json:"status_code"`
	Message            string  `json:"message"`
	Amount             float64 `json:"amount"`
	ActualAmount       float64 `json:"actual_amount"`
	Signature          string  `json:"signature"`
}
