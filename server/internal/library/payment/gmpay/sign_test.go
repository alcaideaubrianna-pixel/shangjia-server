package gmpay

import (
	"testing"

	"hotgo/internal/model"
)

func TestSignParamsUsesGMPayHMACSHA256(t *testing.T) {
	params := map[string]string{
		"amount":       "30",
		"currency":     "usdt",
		"name":         "VIP",
		"network":      "tron",
		"notify_url":   "https://merchant.test/notify",
		"order_id":     "ORD1",
		"pid":          "1000",
		"redirect_url": "https://merchant.test/return",
		"token":        "usdt",
	}

	const expected = "d387adaab4c11291ab7f02e3dbc97c01cc8ddc9ffd4ffa2147d5962864e14937"
	if actual := signParams(params, "secret"); actual != expected {
		t.Fatalf("unexpected GMPay signature: got %q, want %q", actual, expected)
	}
}

func TestIsPaymentSuccessSupportsCurrentGMPayStatus(t *testing.T) {
	if !isPaymentSuccess(&notifyRequest{Status: 2}) {
		t.Fatal("status 2 should be treated as a successful payment")
	}
	if isPaymentSuccess(&notifyRequest{Status: 1}) {
		t.Fatal("status 1 should not be treated as a successful payment")
	}
}

func TestIsPaymentSuccessSupportsLegacyStatusFields(t *testing.T) {
	if !isPaymentSuccess(&notifyRequest{StatusCode: 200}) {
		t.Fatal("legacy status_code 200 should be treated as a successful payment")
	}
	if !isPaymentSuccess(&notifyRequest{Code: 200}) {
		t.Fatal("legacy code 200 should be treated as a successful payment")
	}
}

func TestNotifyUsesActualAmountAndOrderID(t *testing.T) {
	notify := &notifyRequest{
		Status:             2,
		OrderID:            "ORD-2",
		Amount:             30,
		ActualAmount:       30.01,
		BlockTransactionID: "tx-2",
	}

	if got := firstPositive(notify.ActualAmount, notify.Amount); got != 30.01 {
		t.Fatalf("actual amount = %v, want 30.01", got)
	}
	if notify.OrderID != "ORD-2" {
		t.Fatalf("order id = %q, want %q", notify.OrderID, "ORD-2")
	}
}

func TestParseCreateResponseReadsActualAmount(t *testing.T) {
	client := New(&model.PayConfig{GMPayKey: "secret", GMPayPid: "1000"})
	rsp, err := client.parseCreateResponse(`{"status_code":200,"data":{"payment_url":"https://pay.test/checkout/1","trade_id":"trade-1","order_id":"ORD-1","amount":30,"actual_amount":30.01,"currency":"USDT","token":"USDT","network":"tron","receive_address":"TAddress"}}`)
	if err != nil {
		t.Fatalf("parse create response: %v", err)
	}
	if rsp.Data.ActualAmount != 30.01 {
		t.Fatalf("actual amount = %v, want 30.01", rsp.Data.ActualAmount)
	}
	if rsp.Data.ReceiveAddress != "TAddress" {
		t.Fatalf("receive address = %q, want %q", rsp.Data.ReceiveAddress, "TAddress")
	}
}
