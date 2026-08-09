package gmpay

import "testing"

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
